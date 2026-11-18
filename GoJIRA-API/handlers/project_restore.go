package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"gojira-api/db"
)

// POST /api/v{version}/Project/{jiraCode}/restore
//
// 과제 가져오기의 Mongo 쪽. Platform-API 가 zip 을 풀고 Gitea 저장소를 복원한 뒤
// 이 엔드포인트로 프로젝트·이슈·카운터·유즈케이스 산출물을 넣는다.
//
// 이슈 키는 과제 코드에 묶여 있다. 사용자가 다른 코드로 가져오면 키를
// OLD-12 → NEW-12 로 다시 써야 하고, 그 범위는 key 하나가 아니다:
//   - issues[].key
//   - issues[].issueLinks[].outwardIssue.key / inwardIssue.key
//   - issues[].issueLinks[].id          ("tmp_<ts>_OLD-20" 형태로 키가 박혀 있다)
//   - project.feature                   ("OLD-1" 처럼 대표 이슈를 가리킨다)
//   - usecase_artifacts (vosKeys, userStories[].vosKey 등)
//
// 멱등성: 같은 코드로 다시 부르면 같은 결과가 된다(키 기준 upsert).
// 다만 이 엔드포인트는 "새 과제 가져오기" 전용이라 호출자가 중복을 먼저 막는다.
func ProjectRestoreHandler(w http.ResponseWriter, r *http.Request) {
	targetCode := chi.URLParam(r, "jiraCode")
	if targetCode == "" {
		respondError(w, http.StatusBadRequest, "jiraCode path param required")
		return
	}
	user, ok := resolveUser(r, "")
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	var req struct {
		SourceCode       string                   `json:"sourceCode"`
		Name             string                   `json:"name"`
		Project          map[string]interface{}   `json:"project"`
		ArchSettings     interface{}              `json:"archSettings"`
		Issues           []map[string]interface{} `json:"issues"`
		Documents        map[string]interface{}   `json:"documents"`
		UsecaseArtifacts json.RawMessage          `json:"usecaseArtifacts"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Documents == nil {
		respondError(w, http.StatusBadRequest, "documents required (저장소 정보 없이는 복원해도 쓸 수 없다)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	rekey := newRekeyer(req.SourceCode, targetCode)

	// 1) 프로젝트 문서 — 가져온 필드 + 이 시스템 기준의 코드/이름/저장소.
	doc := bson.M{}
	for k, v := range req.Project {
		doc[k] = rekey.value(v)
	}
	doc["name"] = req.Name
	doc["jiraCodes"] = bson.M{"main": bson.M{"jiraCode": targetCode}}
	doc["documents"] = req.Documents
	doc["createdAt"] = time.Now()
	doc["importedFrom"] = bson.M{
		"code":       req.SourceCode,
		"importedAt": time.Now(),
		"importedBy": user,
	}
	if req.ArchSettings != nil {
		doc["archSettings"] = rekey.value(req.ArchSettings)
	}
	if err := db.UpsertProject(ctx, targetCode, doc); err != nil {
		respondError(w, http.StatusInternalServerError, "프로젝트 저장 실패: "+err.Error())
		return
	}
	prj := db.FindProjectByCode(ctx, targetCode)
	if prj == nil {
		respondError(w, http.StatusInternalServerError, "프로젝트 저장 직후 조회 실패: "+targetCode)
		return
	}
	projectID := prj["_id"]

	// 2) 이슈 — 키 재작성 + projectId 재지정. 원본 _id 는 버린다(이 DB 가 새로 발급).
	restored := 0
	var failed []map[string]string
	for _, src := range req.Issues {
		idoc := bson.M{}
		for k, v := range src {
			if k == "_id" || k == "projectId" || k == "_dirty" || k == "_syncedAt" {
				continue
			}
			idoc[k] = rekey.value(v)
		}
		key, _ := idoc["key"].(string)
		if key == "" {
			failed = append(failed, map[string]string{"error": "key 없는 이슈 — 건너뜀"})
			continue
		}
		idoc["projectId"] = projectID
		idoc["_dirty"] = false
		idoc["_syncedAt"] = time.Now()
		if err := db.UpsertIssue(ctx, key, idoc); err != nil {
			failed = append(failed, map[string]string{"key": key, "error": err.Error()})
			continue
		}
		restored++
	}

	// 3) 카운터 — 복원한 이슈의 최대값으로 맞춘다.
	//
	// counters 의 seq 는 "마지막으로 발급된 번호"다(다음 발급은 $inc 후 값). 그래서
	// 최대 키 번호를 그대로 넣으면 다음 이슈가 max+1 로 나간다. 비워 두면 첫 발급
	// 시점에 issues 스캔으로 자동 seed 되지만, 실패한 가져오기가 남긴 낡은 카운터가
	// 있으면 번호가 어긋난다 — 그래서 명시적으로 덮어쓴다.
	counters, cerr := reseedCounters(ctx, targetCode, req.Issues, rekey)
	if cerr != nil {
		log.Printf("restore: 카운터 재설정 실패 (%s): %v", targetCode, cerr)
	}

	// 4) 유즈케이스 산출물 — 없는 과제도 많다.
	usecaseRestored := false
	if len(req.UsecaseArtifacts) > 0 {
		if err := restoreUsecaseArtifacts(ctx, projectID, req.UsecaseArtifacts, rekey, user); err != nil {
			log.Printf("restore: 유즈케이스 산출물 복원 실패 (%s): %v", targetCode, err)
		} else {
			usecaseRestored = true
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"code":            targetCode,
		"sourceCode":      req.SourceCode,
		"keyRewritten":    rekey.active,
		"issuesRestored":  restored,
		"issuesFailed":    failed,
		"counters":        counters,
		"usecaseRestored": usecaseRestored,
	})
}

// rekeyer 과제 코드가 바뀔 때 문자열 안의 이슈 키를 바꿔주는 변환기.
// 코드가 같으면 active=false 로 아무것도 하지 않는다 (불필요한 전체 순회 회피).
type rekeyer struct {
	active bool
	re     *regexp.Regexp
	repl   string
}

func newRekeyer(sourceCode, targetCode string) *rekeyer {
	if sourceCode == "" || sourceCode == targetCode {
		return &rekeyer{active: false}
	}
	return &rekeyer{
		active: true,
		// 키만 바꾼다 — 숫자가 뒤따르는 형태만. 산문에 쓰인 코드는 건드리지 않는다.
		re:   regexp.MustCompile(regexp.QuoteMeta(sourceCode) + `-(\d)`),
		repl: targetCode + "-$1",
	}
}

func (k *rekeyer) str(s string) string {
	if !k.active {
		return s
	}
	return k.re.ReplaceAllString(s, k.repl)
}

// value 임의 JSON 값을 재귀적으로 훑어 문자열만 바꾼다.
func (k *rekeyer) value(v interface{}) interface{} {
	if !k.active {
		return v
	}
	switch t := v.(type) {
	case string:
		return k.str(t)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, e := range t {
			out[i] = k.value(e)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for key, e := range t {
			out[key] = k.value(e)
		}
		return out
	case bson.M:
		out := bson.M{}
		for key, e := range t {
			out[key] = k.value(e)
		}
		return out
	default:
		return v
	}
}

// reseedCounters issuenum / subkey 카운터를 복원된 이슈의 최대값으로 설정한다.
func reseedCounters(ctx context.Context, code string, issues []map[string]interface{}, rekey *rekeyer) (map[string]int, error) {
	maxIssue := 0
	maxSub := map[string]int{}
	prefix := code + "-"
	for _, src := range issues {
		key, _ := src["key"].(string)
		key = rekey.str(key)
		if strings.HasPrefix(key, prefix) {
			if n, err := strconv.Atoi(key[len(prefix):]); err == nil && n > maxIssue {
				maxIssue = n
			}
		}
		sk, _ := src["subkey"].(string)
		if i := strings.LastIndex(sk, "_"); i > 0 {
			p := sk[:i]
			if n, err := strconv.Atoi(sk[i+1:]); err == nil && n > maxSub[p] {
				maxSub[p] = n
			}
		}
	}

	out := map[string]int{}
	set := func(id string, seq int) error {
		if seq <= 0 {
			return nil
		}
		_, err := db.Counters().UpdateOne(ctx,
			bson.M{"_id": id},
			bson.M{"$set": bson.M{"seq": seq}},
			options.UpdateOne().SetUpsert(true),
		)
		if err == nil {
			out[id] = seq
		}
		return err
	}
	if err := set("issuenum:"+code, maxIssue); err != nil {
		return out, err
	}
	for p, n := range maxSub {
		if err := set(fmt.Sprintf("subkey:%s:%s", code, p), n); err != nil {
			return out, err
		}
	}
	return out, nil
}

// restoreUsecaseArtifacts 내보내기에 담긴 유즈케이스 산출물을 새 projectId 로 넣는다.
func restoreUsecaseArtifacts(ctx context.Context, projectID interface{}, raw json.RawMessage, rekey *rekeyer, user string) error {
	var art map[string]interface{}
	if err := json.Unmarshal(raw, &art); err != nil {
		return fmt.Errorf("파싱 실패: %w", err)
	}
	if len(art) == 0 {
		return nil
	}
	fields := bson.M{}
	for k, v := range art {
		if k == "_id" || k == "projectId" || k == "updatedAt" || k == "updatedBy" {
			continue
		}
		fields[k] = rekey.value(v)
	}
	if len(fields) == 0 {
		return nil
	}
	return db.UpsertUsecaseArtifactPartial(ctx, projectID, fields, user)
}

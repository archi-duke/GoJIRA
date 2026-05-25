package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"gopkg.in/yaml.v3"

	"gojira-api/db"
	"gojira-api/models"
)

// data/issues/<subkey>.yaml 경로 (Python issuestore._yaml_path 와 동일 규칙).
var _subkeySafe = regexp.MustCompile(`[^A-Za-z0-9_\-]+`)

func issueYamlPath(subkey string) string {
	safe := _subkeySafe.ReplaceAllString(subkey, "_")
	return "data/issues/" + safe + ".yaml"
}

// localWrite — 로컬 쓰기 모드 응답에 동봉하는 파일 조작 지시.
// CLI(로컬 clone 보유자)가 이 목록을 받아 repo root 기준으로 적용한다.
//   - Content 있음 → 해당 경로에 본문 기록(생성/갱신)
//   - Delete=true  → 해당 경로 삭제
type localWrite struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

// 이슈 쓰기는 항상 로컬 모델(트랙 E): 서버는 fs 에 손대지 않고 YAML 본문을 localWrites 로
// 돌려주며, 호출자(CLI/스킬/에이전트)가 로컬 clone 에 기록한다. 웹은 조회 전용.

// POST /api/v{version}/Issue — 이슈 생성.
//
// 동작 변경 (YAML 진실 원천):
//  1. key / subkey 자체 할당 (JIRA 호출 제거)
//  2. YAML 직렬화 → docs/{user} 브랜치 커밋
//  3. Mongo 캐시 upsert + project.issues 배열 갱신
//
// JIRA 푸시는 별도 기능(/Issue/push-jira, 추후)에서 처리.
func IssueCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, ok := resolveUser(r, req.User)
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Summary == "" {
		respondError(w, http.StatusBadRequest, "summary is required")
		return
	}
	if req.IssueType == "" {
		respondError(w, http.StatusBadRequest, "issueType is required")
		return
	}
	prjKey := req.ProjectKey
	if prjKey == "" {
		prjKey = req.PrjID
	}
	if prjKey == "" {
		respondError(w, http.StatusBadRequest, "prjID/projectKey required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// 0) project 존재 확인 + _id 확보 (이슈는 project._id 로 단일 소속)
	prj := db.FindProjectByCode(ctx, prjKey)
	if prj == nil {
		respondError(w, http.StatusNotFound, "project not found: "+prjKey)
		return
	}
	projectID := prj["_id"]

	// 1) key 할당
	num, err := db.NextIssueNum(ctx, prjKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "key 할당 실패: "+err.Error())
		return
	}
	key := fmt.Sprintf("%s-%d", prjKey, num)

	// 2) subkey 할당 (미전달 시 issueType 매핑)
	subkey := req.SubKey
	if subkey == "" {
		prefix := db.SubkeyPrefix(req.IssueType, req.ReqType)
		if prefix == "" {
			respondError(w, http.StatusBadRequest, "subkey required for issueType "+req.IssueType)
			return
		}
		subkey, err = db.NextSubkey(ctx, prjKey, prefix)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "subkey 할당 실패: "+err.Error())
			return
		}
	}

	// 3) YAML doc build
	now := time.Now().Format("2006-01-02T15:04:05.000+0900")
	doc := map[string]interface{}{
		"key":         key,
		"subkey":      subkey,
		"issueType":   req.IssueType,
		"summary":     req.Summary,
		"description": req.Desc,
		"status":      "To Do",
		"priority":    "Medium",
		"labels":      req.Labels,
		"created":     now,
		"updated":     now,
	}
	if req.ReqType != "" && req.ReqType != "null" {
		doc["reqtype"] = req.ReqType
	}

	yamlBytes, err := yaml.Marshal(doc)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "yaml marshal: "+err.Error())
		return
	}

	// 4) YAML 영속화 — 로컬 clone 에 기록 (호출자가 응답 localWrites 적용). 서버는 fs 미접촉.
	resp := map[string]interface{}{
		"key":         key,
		"subkey":      subkey,
		"localWrites": []localWrite{{Path: issueYamlPath(subkey), Content: string(yamlBytes)}},
	}

	// 5) Mongo 캐시 upsert (실패해도 YAML 이 진실이므로 경고만)
	if err := upsertIssueCacheFromDoc(ctx, key, projectID, bson.M(doc)); err != nil {
		log.Printf("cache upsert warn key=%s: %v", key, err)
	}

	log.Printf("created issue %s subkey=%s user=%s (YAML)", key, subkey, user)
	respondJSON(w, http.StatusOK, resp)
}

// GET /api/v{version}/Issue — Mongo 캐시 read.
func IssueGetHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueGetRequest
	if err := parseXBody(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid XBody header")
		return
	}
	if _, ok := resolveUser(r, req.User); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var filter bson.M
	switch req.InfoType {
	case "designs":
		filter = bson.M{"issueType": bson.M{
			"$in": []string{"Architectural Design", "Detailed Design"},
		}}
	case "nonTestIssues":
		filter = bson.M{"issueType": bson.M{"$ne": "Test"}}
	case "isBusy":
		respondJSON(w, http.StatusOK, false)
		return
	default:
		filter = nil
	}

	if req.PrjID != "" {
		prjDoc := db.FindProjectByCode(ctx, req.PrjID)
		if prjDoc == nil {
			respondJSON(w, http.StatusOK, []bson.M{})
			return
		}
		// 이슈는 projectId 로 단일 소속
		prjFilter := bson.M{"projectId": prjDoc["_id"]}
		if filter != nil {
			filter = bson.M{"$and": []bson.M{filter, prjFilter}}
		} else {
			filter = prjFilter
		}
	}

	issues, err := db.FindAllIssues(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cleaned := make([]bson.M, 0, len(issues))
	for _, issue := range issues {
		delete(issue, "_id")
		delete(issue, "_dirty")
		delete(issue, "_syncedAt")
		cleaned = append(cleaned, issue)
	}

	respondJSON(w, http.StatusOK, cleaned)
}

// PUT /api/v{version}/Issue — 필드 업데이트.
//
// 동작: Mongo 캐시 문서 로드 → Data 병합 → YAML 재생성 → localWrites 로 반환(호출자가 로컬
// clone 에 기록) → 캐시 업데이트. 서버는 fs 미접촉. JIRA 동기화는 별도 push 플로우.
func IssueUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueUpdateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, ok := resolveUser(r, req.User); !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	existing, err := db.FindIssueByKey(ctx, req.Key)
	if err != nil || existing == nil {
		respondError(w, http.StatusNotFound, "issue not found: "+req.Key)
		return
	}

	subkey, _ := existing["subkey"].(string)
	if subkey == "" {
		respondError(w, http.StatusConflict, "issue has no subkey — cannot locate YAML")
		return
	}
	code := keyToProjectCode(req.Key)
	if code == "" {
		respondError(w, http.StatusBadRequest, "cannot derive project code from key")
		return
	}

	// 병합 — 내부 필드 제외
	merged := bson.M{}
	for k, v := range existing {
		if k == "_id" || k == "_dirty" || k == "_syncedAt" {
			continue
		}
		merged[k] = v
	}
	for k, v := range req.Data {
		merged[k] = v
	}
	merged["updated"] = time.Now().Format("2006-01-02T15:04:05.000+0900")

	yamlBytes, err := yaml.Marshal(merged)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "yaml marshal: "+err.Error())
		return
	}

	resp := map[string]interface{}{
		"key":         req.Key,
		"localWrites": []localWrite{{Path: issueYamlPath(subkey), Content: string(yamlBytes)}},
	}

	if err := upsertIssueCacheFromDoc(ctx, req.Key, merged["projectId"], merged); err != nil {
		log.Printf("cache upsert warn key=%s: %v", req.Key, err)
	}

	respondJSON(w, http.StatusOK, resp)
}

// DELETE /api/v{version}/Issue — 이슈 삭제.
//
// 자기 과제 이슈: YAML 삭제 → 캐시 삭제 → 모든 프로젝트 issues 배열에서 제거.
// 타 과제 이슈(링크만 걸려있는 경우): 현재 프로젝트 issues 배열에서만 제거.
// JIRA 삭제 호출은 제거.
func IssueDeleteHandler(w http.ResponseWriter, r *http.Request) {
	var req models.IssueDeleteRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, ok := resolveUser(r, req.User)
	if !ok {
		respondError(w, http.StatusForbidden, "user required")
		return
	}
	if req.Key == "" {
		respondError(w, http.StatusBadRequest, "key is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	writes, err := deleteIssueInternal(ctx, req.PrjID, req.Key, user)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := map[string]interface{}{"key": req.Key}
	if writes != nil {
		resp["localWrites"] = writes
	}
	respondJSON(w, http.StatusOK, resp)
}

// deleteIssueInternal encapsulates the full delete flow: Gitea YAML + Mongo issues.
// Soft delete (2026-04-29 사고 후): mongo issue → issues_trash 컬렉션으로 이동, 3일 후
// 자동 영구 삭제 (purgeOldTrashWorker). YAML 은 Gitea 에서 즉시 삭제 (git history 가
// 자체 휴지통 역할). 복원 시 trash mongo doc 으로부터 yaml 재생성 가능.
func deleteIssueInternal(ctx context.Context, code, key, user string) ([]localWrite, error) {
	isOwnIssue := code != "" && strings.HasPrefix(key, code+"-")

	var writes []localWrite
	if isOwnIssue {
		existing, _ := db.FindIssueByKey(ctx, key)
		subkey, _ := existing["subkey"].(string)
		if subkey != "" {
			// 로컬 clone 의 YAML 삭제 지시 — 호출자가 적용. 서버는 fs 미접촉.
			writes = append(writes, localWrite{Path: issueYamlPath(subkey), Delete: true})
		}
	}
	// Mongo: issues → issues_trash (soft delete). 3일 후 자동 영구 삭제.
	if err := db.MoveIssueToTrash(ctx, key, user, ""); err != nil {
		return nil, err
	}
	return writes, nil
}

// ── 유틸 ──

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func keyToProjectCode(key string) string {
	idx := strings.LastIndex(key, "-")
	if idx <= 0 {
		return ""
	}
	return key[:idx]
}

// upsertIssueCacheFromDoc — 이슈 YAML doc(map) 를 Mongo 캐시 형태로 변환해 upsert.
// "YAML = 진실, Mongo = 캐시" 변환을 한 곳에 모은다 (create/update/ingest 공용).
// 내부 필드(_id/_dirty/_syncedAt)는 제외하고, projectId·동기화 메타를 다시 채운다.
func upsertIssueCacheFromDoc(ctx context.Context, key string, projectID interface{}, doc bson.M) error {
	cacheDoc := bson.M{}
	for k, v := range doc {
		if k == "_id" || k == "_dirty" || k == "_syncedAt" {
			continue
		}
		cacheDoc[k] = v
	}
	// issueLinks 정규화 — 과거 어느 시점에 ordered-map 이 array-of-{key,value}
	// 로 잘못 직렬화돼 캐시에 들어간 적이 있고, fetch→yaml→ingest 사이클을 돌
	// 면서 self-perpetuate 됐다. 진입점(이 함수) 에서 한 번 normalize 해 차단.
	if links, ok := cacheDoc["issueLinks"]; ok {
		cacheDoc["issueLinks"] = normalizeValue(links)
	}
	cacheDoc["projectId"] = projectID
	cacheDoc["_dirty"] = false
	cacheDoc["_syncedAt"] = time.Now()
	return db.UpsertIssue(ctx, key, cacheDoc)
}

// normalizeValue — 임의의 BSON/YAML 디코드 결과 안에 섞여있는
// "[{key:K, value:V}, ...]" 형태의 ordered-map 인공물을 정상 {K:V, ...} 로 되돌린다.
// 정상 데이터에는 no-op (모든 분기가 같은 값을 반환).
func normalizeValue(v interface{}) interface{} {
	switch x := v.(type) {
	case bson.M:
		out := bson.M{}
		for k, vv := range x {
			out[k] = normalizeValue(vv)
		}
		return out
	case map[string]interface{}:
		out := bson.M{}
		for k, vv := range x {
			out[k] = normalizeValue(vv)
		}
		return out
	case bson.A:
		if obj, ok := pairsToObject([]interface{}(x)); ok {
			return obj
		}
		out := make(bson.A, len(x))
		for i, e := range x {
			out[i] = normalizeValue(e)
		}
		return out
	case []interface{}:
		if obj, ok := pairsToObject(x); ok {
			return obj
		}
		out := make([]interface{}, len(x))
		for i, e := range x {
			out[i] = normalizeValue(e)
		}
		return out
	default:
		return x
	}
}

// pairsToObject — slice 의 모든 요소가 정확히 {key, value} 두 필드 map 이면
// 합쳐서 bson.M 반환. 하나라도 다르면 (nil, false). 빈 slice 도 false.
func pairsToObject(arr []interface{}) (bson.M, bool) {
	if len(arr) == 0 {
		return nil, false
	}
	obj := bson.M{}
	for _, e := range arr {
		var m map[string]interface{}
		switch t := e.(type) {
		case bson.M:
			m = map[string]interface{}(t)
		case map[string]interface{}:
			m = t
		default:
			return nil, false
		}
		if len(m) != 2 {
			return nil, false
		}
		kk, hasK := m["key"]
		vv, hasV := m["value"]
		if !hasK || !hasV {
			return nil, false
		}
		ks, ok := kk.(string)
		if !ok {
			return nil, false
		}
		obj[ks] = normalizeValue(vv)
	}
	return obj, true
}

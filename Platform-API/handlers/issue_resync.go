package handlers

// issue_resync.go — Phase 2 서버측 reconcile 트리거 (web-local-view-model.md §Phase 2).
//
// 공식본(Mongo, GoJIRA-API 캐시)은 main push 시점에만 갱신한다. main 의 data/issues/*.yaml
// 이 진실이므로, main push 후 이 파일 전체를 읽어 GoJIRA-API 벌크 ingest(reconcile=true)
// 로 보내면 ingest 집합에 없는 캐시 이슈는 GoJIRA-API 측에서 제거된다 — 즉 삭제도
// "reconcile" 만으로 반영된다(별도 delete 이벤트 불필요).
//
// 안전 규칙: data/issues/ 파일 중 단 하나라도 읽기 실패하면 절대 ingest 를 호출하지
// 않는다. 부분 집합으로 reconcile 하면 읽기 실패한 파일에 대응하는 정상 이슈가
// "ingest 집합에 없음" 으로 오인되어 삭제되는 사고로 이어진다.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"platform-api/github"
)

// ResyncProjectIssues — main 트리에서 data/issues/*.yaml(.yml) 전부를 읽어 GoJIRA-API
// 벌크 ingest 엔드포인트(POST /Project/{code}/issues/ingest, reconcile=true)로 전송한다.
func ResyncProjectIssues(owner, repo, code string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	tree, err := github.GetTree(owner, repo, "main")
	if err != nil {
		return nil, fmt.Errorf("get tree failed: %w", err)
	}
	// truncated 트리로 reconcile 하면 누락된 파일의 이슈가 삭제된다 — 부분 집합 금지.
	if tm, ok := tree.(map[string]interface{}); ok {
		if trunc, _ := tm["truncated"].(bool); trunc {
			return nil, fmt.Errorf("tree truncated for %s/%s — resync aborted (partial reconcile 방지)", owner, repo)
		}
	}
	paths := issueYamlPathsFromTree(tree)

	// 파일 0개여도 트리 조회가 성공했으면 빈 files 로 진행한다 — main 에 이슈가 전혀
	// 없다는 것도 유효한 상태이며, 전체 삭제가 main 의 진실이다.
	files := make([]string, 0, len(paths))
	for _, p := range paths {
		fc, err := github.GetFileContent(owner, repo, p, "main")
		if err != nil {
			// 하나라도 읽기 실패하면 ingest 를 호출하지 않는다 — 부분 집합 reconcile 방지.
			return nil, fmt.Errorf("read %s failed: %w", p, err)
		}
		content, err := decodeGitFileContent(fc)
		if err != nil {
			return nil, fmt.Errorf("decode %s failed: %w", p, err)
		}
		files = append(files, content)
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"files":     files,
		"reconcile": true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ingest request: %w", err)
	}

	data, err := callGoJira(ctx, "POST", fmt.Sprintf("/Project/%s/issues/ingest", code), reqBody, "platform")
	if err != nil {
		return nil, fmt.Errorf("ingest call failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse ingest response: %w", err)
	}
	return result, nil
}

// POST /DocRepo/{code}/issues/resync — 수동 트리거(웹훅 유실/디버깅용). 동기 실행.
func DocRepoIssuesResyncHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	result, err := ResyncProjectIssues(owner, name, code)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// issueYamlPathsFromTree — git tree(재귀) 응답에서 data/issues/ 하위 .yaml/.yml blob
// 경로만 추출한다. shape 은 webhook.go 의 extractTreeFiles 와 동일한 GetTree 응답을 따른다.
func issueYamlPathsFromTree(tree interface{}) []string {
	tm, ok := tree.(map[string]interface{})
	if !ok {
		return nil
	}
	items, _ := tm["tree"].([]interface{})
	var paths []string
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t == "tree" {
			continue
		}
		path, _ := m["path"].(string)
		if !strings.HasPrefix(path, "data/issues/") {
			continue
		}
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			paths = append(paths, path)
		}
	}
	return paths
}

// decodeGitFileContent — github.GetFileContent 응답({content, encoding:"base64", ...},
// Gitea/GitHub contents API 표준 shape — docrepo.go DocRepoFileHandler/gojira-launcher
// handlers.go 의 {path, content(base64), encoding:"base64"} 계약과 동일)에서 실제
// 파일 본문을 디코드한다.
func decodeGitFileContent(fc map[string]interface{}) (string, error) {
	raw, _ := fc["content"].(string)
	encoding, _ := fc["encoding"].(string)
	if encoding != "" && encoding != "base64" {
		return raw, nil
	}
	// Gitea/GitHub 는 base64 본문을 개행 포함해 반환할 수 있다.
	clean := strings.ReplaceAll(raw, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

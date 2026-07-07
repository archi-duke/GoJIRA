package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IngestResult — 콜드스타트 ingest 요약 (/open·/edit 응답 동봉).
type IngestResult struct {
	Ingested int  `json:"ingested"`
	Removed  int  `json:"removed"`
	Skipped  bool `json:"skipped,omitempty"` // 백엔드 미설정 등으로 건너뜀
}

// issueFile — 이슈 YAML 1개 (파일명 + raw 내용). readIssueFiles 의 결과 요소.
type issueFile struct {
	Name    string // 파일명 (예: VOS_001.yaml) — 디렉터리 prefix 없음.
	Content string // raw YAML 원문.
}

// readIssueFiles — repoPath/data/issues 하위의 *.yaml, *.yml 파일을 이름순으로 읽는다.
// IngestIssues(Mongo 캐시 동기화)와 handleIssues(GET /issues, 웹 로컬뷰) 가 공용으로 쓴다.
// 디렉터리가 없으면 (nil, nil) — 호출측이 "이슈 없음"으로 구분해 처리(전자는 reconcile
// 로 캐시 정리 계속 진행, 후자는 404).
func readIssueFiles(issuesDir string) ([]issueFile, error) {
	entries, err := os.ReadDir(issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("issues 디렉터리 읽기: %w", err)
	}
	var files []issueFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".yaml") && !strings.HasSuffix(lower, ".yml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(issuesDir, name))
		if err != nil {
			return nil, fmt.Errorf("%s 읽기: %w", name, err)
		}
		files = append(files, issueFile{Name: name, Content: string(b)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// IngestIssues — 로컬 clone 의 data/issues/*.yaml(.yml) 를 읽어 GoJIRA-API 벌크 ingest 로
// Mongo 캐시를 콜드스타트 동기화한다 (트랙 E §2.2). reconcile=true 이므로 "로컬 YAML
// 집합 = 진실"로 보고, 캐시에만 남은 이 과제 소속 이슈는 정리된다.
//
// clone 이 항상 YAML 을 동봉하므로 ingest 소스는 늘 로컬이다. YAML 이 진실이라
// 실패해도 치명적이지 않다 — 호출부는 경고만 하고 진행한다.
func (s *Server) IngestIssues(code, user string) (*IngestResult, error) {
	base := strings.TrimRight(s.cfg.Backend.GojiraAPI, "/")
	if base == "" {
		return &IngestResult{Skipped: true}, nil
	}

	issuesDir := filepath.Join(s.repoPath(code), "data", "issues")
	issueFiles, err := readIssueFiles(issuesDir)
	if err != nil {
		return nil, err
	}
	// data/issues 가 없으면 issueFiles=nil — ingest 할 YAML 은 없지만 reconcile 로 캐시 정리.

	files := make([]string, 0, len(issueFiles))
	for _, f := range issueFiles {
		files = append(files, f.Content)
	}

	body, _ := json.Marshal(map[string]any{"files": files, "reconcile": true})
	url := fmt.Sprintf("%s/Project/%s/issues/ingest", base, code)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", user)
	// SSO 모드: 사용자 세션토큰을 Bearer 로(백엔드 introspect 검증). dev 면 X-User-Id 폴백.
	if t := s.ssoToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ingest 요청: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ingest HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out IngestResult
	_ = json.Unmarshal(data, &out)
	return &out, nil
}

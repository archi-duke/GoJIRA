package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IngestResult — 콜드스타트 ingest 요약 (/open·/edit 응답 동봉).
type IngestResult struct {
	Ingested int  `json:"ingested"`
	Removed  int  `json:"removed"`
	Skipped  bool `json:"skipped,omitempty"` // 백엔드 미설정 등으로 건너뜀
}

// IngestIssues — 로컬 clone 의 data/issues/*.yaml 를 읽어 GoJIRA-API 벌크 ingest 로
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
	entries, err := os.ReadDir(issuesDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("issues 디렉터리 읽기: %w", err)
	}
	// data/issues 가 없으면 entries=nil — ingest 할 YAML 은 없지만 reconcile 로 캐시 정리.

	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(issuesDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s 읽기: %w", e.Name(), err)
		}
		files = append(files, string(b))
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

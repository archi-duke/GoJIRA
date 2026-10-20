package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandleIssuesLocal — 로컬 clone 에 data/issues 가 있으면 *.yaml/*.yml 을 모두
// 반환하고 그 외 확장자는 걸러야 한다 (Phase 1 웹 이슈 뷰 이원화 — GET /issues).
func TestHandleIssuesLocal(t *testing.T) {
	tmp := t.TempDir()
	code := "TESTPRJ"
	repoPath := filepath.Join(tmp, "duke.kimm", code)
	issuesDir := filepath.Join(repoPath, "data", "issues")
	if err := os.MkdirAll(issuesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// .git 디렉터리 존재만으로 "clone 있음" 판정(핸들러는 os.Stat(.git) 만 확인).
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "VOS_001.yaml"), []byte("key: T-1\nsubkey: VOS_001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "FR_001.yml"), []byte("key: T-2\nsubkey: FR_001\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{cfg: &Config{RepoRoot: tmp}}

	req := httptest.NewRequest(http.MethodGet, "/issues?code="+code+"&user=duke.kimm", nil)
	w := httptest.NewRecorder()
	s.handleIssues(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Issues []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		} `json:"issues"`
		Count  int    `json:"count"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("응답 파싱 실패: %v (body=%s)", err, w.Body.String())
	}
	if resp.Count != 2 {
		t.Fatalf("count=%d want 2", resp.Count)
	}
	if resp.Source != "local" {
		t.Fatalf("source=%q want local", resp.Source)
	}
	wantPaths := map[string]bool{"data/issues/VOS_001.yaml": true, "data/issues/FR_001.yml": true}
	for _, it := range resp.Issues {
		if !wantPaths[it.Path] {
			t.Errorf("예기치 않은 path: %q", it.Path)
		}
		if it.Content == "" {
			t.Errorf("content 비어있음: %q", it.Path)
		}
	}
}

// TestHandleIssuesNoClone — 로컬 clone 자체가 없으면 404 (웹은 공식본으로 폴백).
// EnsureRepo 를 호출하지 않으므로 clone 이 자동 생성되면 안 된다.
func TestHandleIssuesNoClone(t *testing.T) {
	tmp := t.TempDir()
	s := &Server{cfg: &Config{RepoRoot: tmp}}

	req := httptest.NewRequest(http.MethodGet, "/issues?code=NOPE&user=duke.kimm", nil)
	w := httptest.NewRecorder()
	s.handleIssues(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(tmp, "duke.kimm", "NOPE")); !os.IsNotExist(err) {
		t.Fatal("clone 이 자동 생성되면 안 됨(EnsureRepo 호출 금지)")
	}
}

// TestHandleIssuesNoIssuesDir — clone 은 있지만 data/issues 가 없으면 404.
func TestHandleIssuesNoIssuesDir(t *testing.T) {
	tmp := t.TempDir()
	code := "EMPTYPRJ"
	repoPath := filepath.Join(tmp, "duke.kimm", code)
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: &Config{RepoRoot: tmp}}

	req := httptest.NewRequest(http.MethodGet, "/issues?code="+code+"&user=duke.kimm", nil)
	w := httptest.NewRecorder()
	s.handleIssues(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", w.Code, w.Body.String())
	}
}

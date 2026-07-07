package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newJournalTestServer — 테스트 전용 Server. gojiraDir() 는 os.UserHomeDir() 를 쓰므로
// HOME/USERPROFILE 을 임시 디렉터리로 가리켜 실제 ~/.gojira 를 건드리지 않는다.
func newJournalTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // windows
	return &Server{cfg: &Config{RepoRoot: t.TempDir()}}, home
}

func postIssueJournal(s *Server, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/issue-journal", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleIssueJournal(w, req)
	return w
}

// TestIssueJournalAppendTwice — 파일 생성 + 2회 append 시 리스트 2항목, 한글 summary 보존.
func TestIssueJournalAppendTwice(t *testing.T) {
	s, home := newJournalTestServer(t)

	body1 := `{"code":"NBBALL","user":"duke.kimm","entry":{"op":"link","keys":["NBBALL-3","NBBALL-7"],"summary":"링크 연결: NBBALL-3 → NBBALL-7"}}`
	w1 := postIssueJournal(s, []byte(body1))
	if w1.Code != http.StatusOK {
		t.Fatalf("1st append status=%d body=%s", w1.Code, w1.Body.String())
	}
	var resp1 struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("응답 파싱 실패: %v", err)
	}
	wantPath := filepath.Join(home, ".gojira", "journal", "NBBALL.yaml")
	if resp1.Path != wantPath {
		t.Fatalf("path=%q want %q", resp1.Path, wantPath)
	}

	body2 := `{"code":"NBBALL","user":"duke.kimm","entry":{"op":"update","keys":["NBBALL-3"],"summary":"우선순위 변경","fields":["priority"]}}`
	w2 := postIssueJournal(s, []byte(body2))
	if w2.Code != http.StatusOK {
		t.Fatalf("2nd append status=%d body=%s", w2.Code, w2.Body.String())
	}

	raw, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("journal 파일 읽기 실패: %v", err)
	}
	content := string(raw)

	// 리스트 항목 2개(각 "- ts:" 로 시작).
	itemCount := strings.Count(content, "\n- ts:")
	if strings.HasPrefix(content, "- ts:") {
		itemCount++
	}
	if itemCount != 2 {
		t.Fatalf("항목 수=%d want 2, content=%s", itemCount, content)
	}

	// 한글 summary 가 원문 그대로(이스케이프 없이) 보존됐는지 확인.
	if !strings.Contains(content, "링크 연결: NBBALL-3 → NBBALL-7") {
		t.Fatalf("한글 summary 가 보존되지 않음: %s", content)
	}
	if !strings.Contains(content, "우선순위 변경") {
		t.Fatalf("두 번째 항목의 한글 summary 가 보존되지 않음: %s", content)
	}
	if !strings.Contains(content, "fields: [\"priority\"]") {
		t.Fatalf("fields 가 올바르게 직렬화되지 않음: %s", content)
	}
	// 첫 항목엔 fields 가 없어야 함(선택 필드 — 있을 때만).
	firstItemEnd := strings.Index(content, "\n- ts:")
	firstItem := content
	if firstItemEnd >= 0 {
		firstItem = content[:firstItemEnd]
	}
	if strings.Contains(firstItem, "fields:") {
		t.Fatalf("첫 항목에 fields 가 없어야 하는데 존재함: %s", firstItem)
	}
}

// TestIssueJournalCodeRejected — code 형식이 올바르지 않으면 400 (경로 traversal/인젝션 차단).
func TestIssueJournalCodeRejected(t *testing.T) {
	s, _ := newJournalTestServer(t)
	body := `{"code":"../evil","user":"duke.kimm","entry":{"op":"create","keys":["X-1"],"summary":"s"}}`
	w := postIssueJournal(s, []byte(body))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", w.Code, w.Body.String())
	}
}

// TestIssueJournalMissingFields — entry 필수필드(op/keys/summary) 누락 시 400.
func TestIssueJournalMissingFields(t *testing.T) {
	s, _ := newJournalTestServer(t)

	cases := []struct {
		name string
		body string
	}{
		{"op 누락", `{"code":"NBBALL","user":"duke.kimm","entry":{"keys":["X-1"],"summary":"s"}}`},
		{"op 화이트리스트 밖", `{"code":"NBBALL","user":"duke.kimm","entry":{"op":"drop","keys":["X-1"],"summary":"s"}}`},
		{"keys 누락", `{"code":"NBBALL","user":"duke.kimm","entry":{"op":"create","summary":"s"}}`},
		{"summary 누락", `{"code":"NBBALL","user":"duke.kimm","entry":{"op":"create","keys":["X-1"]}}`},
		{"code 누락", `{"user":"duke.kimm","entry":{"op":"create","keys":["X-1"],"summary":"s"}}`},
		{"user 누락", `{"code":"NBBALL","entry":{"op":"create","keys":["X-1"],"summary":"s"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := postIssueJournal(s, []byte(c.body))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 body=%s", w.Code, w.Body.String())
			}
		})
	}
}

// TestAgentEnvJournalPath — agentEnv 가 code 있을 때만 GOJIRA_ISSUE_JOURNAL 을 주입한다.
func TestAgentEnvJournalPath(t *testing.T) {
	s, home := newJournalTestServer(t)

	env := s.agentEnv(filepath.Join(s.cfg.RepoRoot, "NBBALL"), "duke.kimm", "NBBALL")
	want := "GOJIRA_ISSUE_JOURNAL=" + filepath.Join(home, ".gojira", "journal", "NBBALL.yaml")
	found := false
	for _, e := range env {
		if e == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("GOJIRA_ISSUE_JOURNAL 미주입 또는 값 불일치. env=%v want=%q", env, want)
	}

	envNoCode := s.agentEnv(s.cfg.RepoRoot, "duke.kimm", "")
	for _, e := range envNoCode {
		if strings.HasPrefix(e, "GOJIRA_ISSUE_JOURNAL=") {
			t.Fatalf("code 없는 일반 채팅인데 GOJIRA_ISSUE_JOURNAL 이 주입됨: %s", e)
		}
	}
}

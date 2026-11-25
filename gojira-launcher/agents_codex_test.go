package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

// codex 는 반드시 --no-daemon 으로 떠야 ATalk 가 세션 창을 찾는다.
func TestCodexSpawnsWithNoDaemon(t *testing.T) {
	spec, ok := agentBins["codex"]
	if !ok {
		t.Fatal("codex 가 에이전트 화이트리스트에 없다")
	}
	if spec.Bin != "codex" || len(spec.Args) != 1 || spec.Args[0] != "--no-daemon" {
		t.Fatalf("codex 실행 명령 = %q %v, want codex [--no-daemon]", spec.Bin, spec.Args)
	}
}

func TestResolveAgentAcceptsCodex(t *testing.T) {
	s := &Server{cfg: &Config{DefaultAgent: "codex"}}
	got, err := s.resolveAgent("")
	if err != nil || got != "codex" {
		t.Fatalf("resolveAgent(\"\") = %q, %v — defaultAgent=codex 가 통하지 않는다", got, err)
	}
	if _, err := s.resolveAgent("bash"); err == nil {
		t.Fatal("화이트리스트 밖 agent 가 통과했다")
	}
}

// chatAgent 는 설치된 에이전트일 때만 /health 에 실린다 — 없는 CLI 를 웹이 띄우려다 실패하지 않게.
func TestHealthChatAgentOnlyWhenInstalled(t *testing.T) {
	s := &Server{cfg: &Config{ChatAgent: "없는에이전트"}}
	rec := httptest.NewRecorder()
	s.handleHealth(rec, httptest.NewRequest("GET", "/health", nil))
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["chatAgent"]; ok {
		t.Fatalf("설치 안 된 chatAgent 가 노출됐다: %v", m["chatAgent"])
	}
}

func TestSyncRetiredModels(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/opencode.json"
	if err := writeFileString(src, `{"model":"zai-coding-plan/glm-5.3","small_model":"zai-coding-plan/glm-5.3-flash"}`); err != nil {
		t.Fatal(err)
	}
	m := map[string]any{"model": "zai-coding-plan/glm-5.1", "small_model": "my/own-model"}
	if !syncRetiredModels(m, src) {
		t.Fatal("단종 모델을 바꾸지 않았다")
	}
	if m["model"] != "zai-coding-plan/glm-5.3" {
		t.Errorf("model = %v", m["model"])
	}
	if m["small_model"] != "my/own-model" {
		t.Errorf("사용자가 고른 small_model 을 바꿨다: %v", m["small_model"])
	}
}

func writeFileString(p, s string) error { return os.WriteFile(p, []byte(s), 0o644) }

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "llm.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// 레지스트리 스키마 — selected 항목이 로드되고 생략 필드는 default 로 채워진다.
func TestLoadConfigRegistry(t *testing.T) {
	p := writeTemp(t, `{
	  "selected": "b",
	  "models": [
	    {"name": "a", "model": "model-a", "endpoint": "http://a"},
	    {"name": "b", "model": "model-b", "endpoint": "http://b", "authType": "header", "authHeader": "X-T"}
	  ]
	}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "model-b" || cfg.Endpoint != "http://b" || cfg.AuthHeader != "X-T" {
		t.Fatalf("selected 항목 미반영: %+v", cfg)
	}
	// 생략 필드 default 보존 (maxTokens 500, responseFormat 등)
	if cfg.MaxTokens != 500 {
		t.Fatalf("생략 필드 default 소실: maxTokens=%d (500 기대)", cfg.MaxTokens)
	}
	if cfg.ResponseFormat.ChoicesPath != "choices" {
		t.Fatalf("responseFormat default 소실: %+v", cfg.ResponseFormat)
	}
}

// selected 가 models 에 없으면 가용 이름을 나열하며 실패한다.
func TestLoadConfigRegistrySelectedNotFound(t *testing.T) {
	p := writeTemp(t, `{
	  "selected": "nope",
	  "models": [{"name": "a", "model": "m"}, {"name": "b", "model": "m"}]
	}`)
	_, err := LoadConfig(p)
	if err == nil {
		t.Fatal("오류 기대")
	}
	if !strings.Contains(err.Error(), "a, b") {
		t.Fatalf("가용 이름 나열 누락: %v", err)
	}
}

// selected 미설정도 fail-fast.
func TestLoadConfigRegistryNoSelected(t *testing.T) {
	p := writeTemp(t, `{"models": [{"name": "a", "model": "m"}]}`)
	if _, err := LoadConfig(p); err == nil || !strings.Contains(err.Error(), "selected 미설정") {
		t.Fatalf("selected 미설정 오류 기대: %v", err)
	}
}

// models 항목의 name 누락도 오류.
func TestLoadConfigRegistryNamelessModel(t *testing.T) {
	p := writeTemp(t, `{"selected": "a", "models": [{"model": "m"}]}`)
	if _, err := LoadConfig(p); err == nil || !strings.Contains(err.Error(), "name 누락") {
		t.Fatalf("name 누락 오류 기대: %v", err)
	}
}

// 구형 평면 스키마 하위호환 — 기존 배포본 llm.json 그대로 동작.
func TestLoadConfigLegacyFlat(t *testing.T) {
	p := writeTemp(t, `{
	  "endpoint": "http://legacy",
	  "model": "legacy-model",
	  "maxTokens": 123
	}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "legacy-model" || cfg.Endpoint != "http://legacy" || cfg.MaxTokens != 123 {
		t.Fatalf("평면 스키마 파싱 실패: %+v", cfg)
	}
}

// ${VAR} 환경변수 치환 — 레지스트리 경로에서도 동작.
func TestLoadConfigRegistryEnvExpand(t *testing.T) {
	t.Setenv("TEST_LLM_KEY", "sk-test")
	p := writeTemp(t, `{
	  "selected": "a",
	  "models": [{"name": "a", "model": "m", "apiKey": "${TEST_LLM_KEY}", "headers": {"X-K": "${TEST_LLM_KEY}"}}]
	}`)
	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "sk-test" || cfg.Headers["X-K"] != "sk-test" {
		t.Fatalf("env 치환 실패: apiKey=%q headers=%v", cfg.APIKey, cfg.Headers)
	}
}

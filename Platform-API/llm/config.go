// Package llm 은 GoJIRA 의 모든 서비스가 공유하는 LLM 호출 인프라다.
//
// llm.json 한 파일로 endpoint / model / auth / 응답 포맷을 관리한다.
// 사내 프록시 / 모델 교체 / 키 회전 시 코드 수정 없이 JSON 만 바꾸면 된다.
package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config 는 llm.json 의 스키마와 동일하다. 미설정 필드는 zero value (호출자가 default 처리).
type Config struct {
	Endpoint       string            `json:"endpoint"`
	Model          string            `json:"model"`
	APIKey         string            `json:"apiKey"`
	MaxTokens      int               `json:"maxTokens"`
	Temperature    float64           `json:"temperature"`
	Stream         bool              `json:"stream"`
	SystemPrompt   string             `json:"systemPrompt"`
	Headers        map[string]string `json:"headers"`
	AuthType       string            `json:"authType"`   // "bearer" | "header" | "none"
	AuthHeader     string            `json:"authHeader"` // authType 이 "header" 일 때
	ResponseFormat ResponseFormat    `json:"responseFormat"`
}

type ResponseFormat struct {
	ChoicesPath  string `json:"choicesPath"`
	ContentPath  string `json:"contentPath"`
	StreamPrefix string `json:"streamPrefix"`
	StreamDone   string `json:"streamDone"`
}

// DefaultConfig 는 OpenAI gpt-4o-mini 호환 기본값을 반환한다.
func DefaultConfig() *Config {
	c := &Config{
		Endpoint:    "https://api.openai.com/v1/chat/completions",
		Model:       "gpt-4o-mini",
		MaxTokens:   500,
		Temperature: 0.3,
		Headers:     map[string]string{"Content-Type": "application/json"},
		AuthType:    "bearer",
	}
	c.ResponseFormat.ChoicesPath = "choices"
	c.ResponseFormat.ContentPath = "message.content"
	c.ResponseFormat.StreamPrefix = "data: "
	c.ResponseFormat.StreamDone = "[DONE]"
	return c
}

// LoadConfig 는 path 의 JSON 파일을 읽어 Config 로 파싱한다.
// 누락 필드는 DefaultConfig 의 값으로 채운다 (Headers, ResponseFormat 등).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("llm.json 읽기 실패: %w", err)
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("llm.json 파싱 실패: %w", err)
	}
	// JSON 에서 누락된 ResponseFormat 필드 보강
	if cfg.ResponseFormat.ChoicesPath == "" {
		cfg.ResponseFormat.ChoicesPath = "choices"
	}
	if cfg.ResponseFormat.ContentPath == "" {
		cfg.ResponseFormat.ContentPath = "message.content"
	}
	if cfg.ResponseFormat.StreamPrefix == "" {
		cfg.ResponseFormat.StreamPrefix = "data: "
	}
	if cfg.ResponseFormat.StreamDone == "" {
		cfg.ResponseFormat.StreamDone = "[DONE]"
	}
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{"Content-Type": "application/json"}
	}
	// llm.json 에 박힌 ${VAR} 패턴을 환경변수 값으로 치환.
	// 예: "X-Dep-Ticket": "${PLATFORM_LLM_TICKET}" → 실제 토큰 값.
	// 시크릿(인증 토큰 등) 을 git tracked 파일에서 분리하기 위함.
	cfg.APIKey = expandEnv(cfg.APIKey)
	for k, v := range cfg.Headers {
		cfg.Headers[k] = expandEnv(v)
	}
	cfg.SystemPrompt = expandEnv(cfg.SystemPrompt)
	return cfg, nil
}

// expandEnv 는 "${VAR}" 패턴을 환경변수로 치환한다 (os.Expand 와 동일하지만 $VAR 형식은 무시).
// 미정의 환경변수는 빈 문자열로 대체.
func expandEnv(s string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	out := s
	for {
		start := strings.Index(out, "${")
		if start < 0 {
			break
		}
		end := strings.Index(out[start:], "}")
		if end < 0 {
			break
		}
		end += start
		varName := out[start+2 : end]
		val := os.Getenv(varName)
		out = out[:start] + val + out[end+1:]
	}
	return out
}

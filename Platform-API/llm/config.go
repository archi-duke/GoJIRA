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
	// ExtraBody 는 요청 body 에 그대로 병합되는 프로바이더 전용 파라미터.
	// 예: z.ai GLM 하이브리드 추론 끄기 — {"thinking": {"type": "disabled"}}
	// (reasoning 이 max_tokens 를 소진해 content 가 비는 것 방지)
	ExtraBody      map[string]interface{} `json:"extraBody"`
	ResponseFormat ResponseFormat         `json:"responseFormat"`
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

// registryFile 은 llm.json 의 신 스키마(레지스트리 형식) 탐지용.
//
//	{ "selected": "<name>", "models": [ {"name": "...", ...Config...}, ... ] }
//
// models 가 비어 있으면 구 스키마(평면 Config) 로 간주해 하위호환 파싱한다.
// 항목은 RawMessage 로 보관 — 선택된 항목만 DefaultConfig 위에 직접 unmarshal 해서
// "생략 필드 = default 유지" 의미를 평면 스키마와 동일하게 보장한다
// (Config 재marshal 방식은 omitempty 부재로 zero value 가 default 를 덮어씀).
type registryFile struct {
	Selected string            `json:"selected"`
	Models   []json.RawMessage `json:"models"`
}

// modelProbe — 레지스트리 항목에서 name 만 미리 읽는 용도.
type modelProbe struct {
	Name string `json:"name"`
}

// LoadConfig 는 path 의 JSON 파일을 읽어 Config 로 파싱한다.
//
// 두 스키마를 지원한다:
//   - 레지스트리(권장): models 배열에 후보 모델들을 등록하고 selected 로 하나를 지정.
//     selected 미설정 / 이름 불일치는 오류 (가용 이름을 나열해 fail-fast).
//   - 평면(구형): 파일 최상위가 Config 그 자체. 기존 배포본 하위호환.
//
// 누락 필드는 DefaultConfig 의 값으로 채운다 (Headers, ResponseFormat 등).
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("llm.json 읽기 실패: %w", err)
	}

	// 1) 레지스트리 스키마 탐지 — models 배열이 있으면 selected 로 선택.
	var reg registryFile
	if err := json.Unmarshal(data, &reg); err == nil && len(reg.Models) > 0 {
		names := make([]string, 0, len(reg.Models))
		var chosen json.RawMessage
		for _, raw := range reg.Models {
			var p modelProbe
			if err := json.Unmarshal(raw, &p); err != nil || p.Name == "" {
				return nil, fmt.Errorf("llm.json: models 항목에 name 누락/파싱 실패 (가용: %s)", strings.Join(names, ", "))
			}
			names = append(names, p.Name)
			if p.Name == reg.Selected {
				chosen = raw
			}
		}
		if reg.Selected == "" {
			return nil, fmt.Errorf("llm.json: models 레지스트리에 selected 미설정 — 가용: %s", strings.Join(names, ", "))
		}
		if chosen == nil {
			return nil, fmt.Errorf("llm.json: selected=%q 가 models 에 없음 — 가용: %s", reg.Selected, strings.Join(names, ", "))
		}
		cfg := DefaultConfig()
		if err := json.Unmarshal(chosen, cfg); err != nil {
			return nil, fmt.Errorf("llm.json: selected=%q 항목 파싱 실패: %w", reg.Selected, err)
		}
		finalize(cfg)
		return cfg, nil
	}

	// 2) 평면(구형) 스키마 — 하위호환.
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("llm.json 파싱 실패: %w", err)
	}
	finalize(cfg)
	return cfg, nil
}

// finalize — 누락 필드 default 보강 + ${VAR} 환경변수 치환 (레지스트리/평면 공통 마무리).
func finalize(cfg *Config) {
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

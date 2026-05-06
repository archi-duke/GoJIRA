package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Message 는 OpenAI chat/completions 의 한 메시지.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Options 는 호출별 override 항목. zero value 면 Config 의 값을 사용.
type Options struct {
	Model       string          // "" 이면 cfg.Model
	MaxTokens   int             // 0 이면 cfg.MaxTokens
	Temperature *float64        // nil 이면 cfg.Temperature
	Timeout     time.Duration   // 0 이면 30s
	Ctx         context.Context // nil 이면 background + Timeout
}

// Call 은 messages 배열로 LLM 을 호출하고 첫 choice 의 content 를 반환한다.
// API 키는 cfg.APIKey > 환경변수 OPENAI_API_KEY 순으로 결정.
func Call(cfg *Config, messages []Message, opts *Options) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("llm config nil")
	}
	if len(messages) == 0 {
		return "", fmt.Errorf("messages empty")
	}

	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" && cfg.AuthType != "none" {
		return "", fmt.Errorf("API 키 미설정")
	}

	model := cfg.Model
	maxTokens := cfg.MaxTokens
	temperature := cfg.Temperature
	timeout := 30 * time.Second
	parent := context.Background()

	if opts != nil {
		if opts.Model != "" {
			model = opts.Model
		}
		if opts.MaxTokens > 0 {
			maxTokens = opts.MaxTokens
		}
		if opts.Temperature != nil {
			temperature = *opts.Temperature
		}
		if opts.Timeout > 0 {
			timeout = opts.Timeout
		}
		if opts.Ctx != nil {
			parent = opts.Ctx
		}
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": temperature,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("payload marshal 실패: %w", err)
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("요청 생성 실패: %w", err)
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	switch cfg.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case "header":
		if cfg.AuthHeader != "" {
			req.Header.Set(cfg.AuthHeader, apiKey)
		}
	}

	// 호출 시작 로그 — 인증 헤더는 길이만 (시크릿 평문 노출 방지)
	authVal := ""
	if cfg.AuthType == "header" && cfg.AuthHeader != "" {
		authVal = req.Header.Get(cfg.AuthHeader)
	} else if cfg.AuthType == "bearer" {
		authVal = req.Header.Get("Authorization")
	}
	log.Printf("LLM 요청: POST %s (auth=<len=%d>)", cfg.Endpoint, len(authVal))

	// 사내망 프록시 지원을 위해 명시적 Transport (HTTP_PROXY 환경변수 자동 활용)
	client := &http.Client{Transport: http.DefaultTransport, Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API 호출 실패: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		log.Printf("LLM 응답 오류 %d: %s", resp.StatusCode, truncate(string(data), 500))
		return "", fmt.Errorf("API 오류 (%d): %s", resp.StatusCode, string(data))
	}

	// OpenAI 호환 응답 + gpt-oss 확장 (reasoning 필드)
	var parsed struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				Reasoning        string `json:"reasoning"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", fmt.Errorf("응답 파싱 실패: %s", truncate(string(data), 500))
	}

	// content 가 비어있으면 reasoning 필드들 확인 (gpt-oss 모델)
	result := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if result == "" {
		reasoning := parsed.Choices[0].Message.Reasoning
		if reasoning == "" {
			reasoning = parsed.Choices[0].Message.ReasoningContent
		}
		if reasoning != "" {
			// reasoning 에서 JSON 추출 ({"result":"...", "status":"..."})
			if start := strings.Index(reasoning, "{\"result\""); start >= 0 {
				if end := strings.Index(reasoning[start:], "}"); end >= 0 {
					result = strings.TrimSpace(reasoning[start : start+end+1])
				}
			}
		}
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// CallPrompt 는 단일 user message 로 호출하는 편의 함수.
func CallPrompt(cfg *Config, prompt string) (string, error) {
	return Call(cfg, []Message{{Role: "user", Content: prompt}}, nil)
}

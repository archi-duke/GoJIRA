package skills

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"gojira-api/config"
)

var platformClient = &http.Client{}

// CallLLM 단순 LLM 호출 (tool_use 없이)
func CallLLM(prompt string) (string, error) {
	cfg := config.LLM

	payload := map[string]interface{}{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  cfg.MaxTokens,
		"temperature": cfg.Temperature,
	}

	b, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	switch cfg.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)
	case "header":
		if cfg.AuthHeader != "" {
			req.Header.Set(cfg.AuthHeader, config.OpenAIAPIKey)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, string(data))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM response parse failed")
	}

	text := strings.TrimSpace(result.Choices[0].Message.Content)
	// 코드블록 제거
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i != -1 {
			text = text[i+1:]
		}
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	return text, nil
}

// CallLLMWithTools tool_use 지원 LLM 호출
func CallLLMWithTools(messages []map[string]interface{}, tools []map[string]interface{}) (map[string]interface{}, error) {
	cfg := config.LLM

	maxTokens := cfg.MaxTokens
	if maxTokens < 2000 {
		maxTokens = 2000 // Agent는 tool_use 응답에 더 많은 토큰 필요
	}

	payload := map[string]interface{}{
		"model":       cfg.Model,
		"messages":    messages,
		"max_tokens":  maxTokens,
		"temperature": cfg.Temperature,
		"tools":       tools,
	}

	b, _ := json.Marshal(payload)
	log.Printf("LLM request tools count=%d, messages count=%d", len(tools), len(messages))

	req, err := http.NewRequest("POST", cfg.Endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	switch cfg.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)
	case "header":
		if cfg.AuthHeader != "" {
			req.Header.Set(cfg.AuthHeader, config.OpenAIAPIKey)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	log.Printf("LLM response status=%d, body_len=%d", resp.StatusCode, len(data))

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("LLM API error (%d): %s", resp.StatusCode, string(data))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("LLM response parse failed")
	}

	// tool_calls 디버그
	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]interface{}); ok {
			if m, ok := c["message"].(map[string]interface{}); ok {
				if tc, ok := m["tool_calls"]; ok {
					log.Printf("LLM tool_calls: %v", tc)
				} else {
					content, _ := m["content"].(string)
					log.Printf("LLM no tool_calls, content: %.100s", content)
				}
			}
		}
	}

	return result, nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n...(truncated)"
	}
	return s
}

// FetchFileContent Platform-API에서 파일 내용 조회 (exported)
func FetchFileContent(projectCode, path, ref string) (string, error) {
	return fetchFileContent(projectCode, path, ref)
}

func fetchFileContent(projectCode, path, ref string) (string, error) {
	u := fmt.Sprintf("%s/DocRepo/%s/file?path=%s&ref=%s",
		config.PlatformAPI, projectCode, url.QueryEscape(path), url.QueryEscape(ref))
	resp, err := platformClient.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("file fetch failed: %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	json.Unmarshal(data, &result)
	if result.Encoding == "base64" || result.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(result.Content)
		if err != nil {
			return result.Content, nil
		}
		return string(decoded), nil
	}
	return "", nil
}

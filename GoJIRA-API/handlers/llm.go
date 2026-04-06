package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"gojira-api/config"
)

// summarizePRContent LLM을 사용하여 파일 내용을 요약해 PR 제목/본문 생성
func summarizePRContent(projectName, projectCode string, files map[string]string) (title, body string) {
	fallbackTitle := fmt.Sprintf("[%s] 설계문서 갱신", projectCode)
	fallbackBody := fmt.Sprintf("프로젝트 %s의 설계문서를 갱신합니다.", projectName)

	if config.OpenAIAPIKey == "" {
		return fallbackTitle, fallbackBody
	}

	// 파일 내용 준비 (너무 길면 truncate)
	var content string
	for path, text := range files {
		chunk := text
		if len(chunk) > 2000 {
			chunk = chunk[:2000] + "\n...(truncated)"
		}
		content += fmt.Sprintf("### %s\n%s\n\n", path, chunk)
	}
	if len(content) > 10000 {
		content = content[:10000] + "\n...(truncated)"
	}

	prompt := config.LoadPrompt("pr-summary.txt", map[string]string{
		"ProjectName": projectName,
		"ProjectCode": projectCode,
		"Content":     content,
	})
	if prompt == "" {
		// 프롬프트 파일 로드 실패 시 인라인 폴백
		prompt = fmt.Sprintf(`다음은 프로젝트 "%s" (%s)의 설계문서 파일들입니다.
이 내용을 바탕으로 GitHub PR 제목과 본문을 한국어로 작성해주세요.
규칙:
- 제목: 70자 이내, "[프로젝트코드] 핵심변경내용" 형식
- 본문: 변경 내용을 3~5줄로 요약 (마크다운 bullet 사용)
- 순수 JSON만 응답: {"title": "...", "body": "..."}
파일 내용:
%s`, projectName, projectCode, content)
	}

	text, err := callLLM(prompt)
	if err != nil {
		log.Printf("LLM 호출 실패: %v", err)
		return fallbackTitle, fallbackBody
	}

	var parsed struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		log.Printf("LLM 응답 JSON 파싱 실패: %v, text: %s", err, text)
		return fallbackTitle, fallbackBody
	}

	if parsed.Title == "" {
		parsed.Title = fallbackTitle
	}
	if parsed.Body == "" {
		parsed.Body = fallbackBody
	}
	return parsed.Title, parsed.Body
}

// callLLM 설정 파일 기반 LLM API 호출
func callLLM(prompt string) (string, error) {
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
		return "", fmt.Errorf("요청 생성 실패: %w", err)
	}

	// 설정 파일의 헤더 적용
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	// 인증 처리
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
		return "", fmt.Errorf("API 호출 실패: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("API 오류 (%d): %s", resp.StatusCode, string(data))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 {
		return "", fmt.Errorf("응답 파싱 실패: %s", string(data))
	}

	text := strings.TrimSpace(result.Choices[0].Message.Content)
	// GPT가 ```json ... ``` 코드블록으로 감쌀 경우 제거
	if strings.HasPrefix(text, "```") {
		if i := strings.Index(text, "\n"); i != -1 {
			text = text[i+1:]
		}
		if strings.HasSuffix(text, "```") {
			text = strings.TrimSuffix(text, "```")
		}
		text = strings.TrimSpace(text)
	}
	return text, nil
}

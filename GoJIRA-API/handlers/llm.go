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

	prompt := fmt.Sprintf(`다음은 프로젝트 "%s" (%s)의 설계문서 파일들입니다.
이 내용을 바탕으로 GitHub PR 제목과 본문을 한국어로 작성해주세요.

규칙:
- 제목: 70자 이내, "[프로젝트코드] 핵심변경내용" 형식
- 본문: 변경 내용을 3~5줄로 요약 (마크다운 bullet 사용)
- 순수 JSON만 응답 (코드블록이나 마크다운 없이): {"title": "...", "body": "..."}

파일 내용:
%s`, projectName, projectCode, content)

	payload := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  500,
		"temperature": 0.3,
	}

	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.OpenAIAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("LLM 호출 실패: %v", err)
		return fallbackTitle, fallbackBody
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		log.Printf("OpenAI API 오류 (%d): %s", resp.StatusCode, string(data))
		return fallbackTitle, fallbackBody
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil || len(result.Choices) == 0 {
		return fallbackTitle, fallbackBody
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

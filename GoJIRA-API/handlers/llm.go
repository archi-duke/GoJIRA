package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"gojira-api/config"
)

// summarizePRContent LLM 을 사용해 파일 내용을 요약 → PR 제목/본문 생성.
// 실제 LLM 호출은 Platform-API /LLM/call 게이트웨이로 위임 — GoJIRA-API 는 키나
// 모델 정보를 보유하지 않는다.
//
// 실패 시 error 반환. 호출자는 폴백하지 않고 작업 실패로 처리해야 한다.
func summarizePRContent(projectName, projectCode string, files map[string]string) (title, body string, err error) {
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
		return "", "", fmt.Errorf("LLM PR 요약 호출 실패: %w", err)
	}

	var parsed struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return "", "", fmt.Errorf("LLM 응답 JSON 파싱 실패: %w (text=%s)", err, text)
	}
	if parsed.Title == "" || parsed.Body == "" {
		return "", "", fmt.Errorf("LLM 응답에 title/body 누락 (text=%s)", text)
	}
	return parsed.Title, parsed.Body, nil
}

// callLLM 은 Platform-API /LLM/call 로 단일 prompt 를 보내고 응답 텍스트를 받아 후처리한다.
// (```json ... ``` fence 제거)
func callLLM(prompt string) (string, error) {
	payload := map[string]interface{}{"prompt": prompt}
	res, sc := callPlatform("POST", "/LLM/call", payload)
	if sc >= 400 || res == nil {
		return "", fmt.Errorf("Platform LLM 호출 실패 (status=%d)", sc)
	}
	resMap, ok := res.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("Platform LLM 응답 형식 오류")
	}
	if errStr, _ := resMap["error"].(string); errStr != "" {
		return "", fmt.Errorf("Platform LLM 오류: %s", errStr)
	}
	text, _ := resMap["text"].(string)
	text = strings.TrimSpace(text)
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

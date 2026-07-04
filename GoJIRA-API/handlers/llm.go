package handlers

import (
	"fmt"
	"strings"
)

// callLLM 은 Platform-API /LLM/call 로 단일 prompt 를 보내고 응답 텍스트를 받아 후처리한다.
// (```json ... ``` fence 제거)
// Platform-API 가 키/모델/엔드포인트를 단독 관리하므로 GoJIRA-API 는 이 함수로만 LLM 을 쓴다.
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

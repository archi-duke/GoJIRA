package skills

import (
	"context"
	"fmt"

	"gojira-api/config"
)

func init() {
	Register(&Skill{
		ID:          "doc_summarize",
		Name:        "문서 요약",
		Description: "프로젝트의 설계문서 내용을 요약합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드" },
				"filePath": { "type": "string", "description": "요약할 파일 경로 (예: srs.md, architecture.md)" }
			},
			"required": ["projectCode", "filePath"]
		}`),
		Execute: executeDocSummarize,
	})
}

func executeDocSummarize(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	filePath, _ := params["filePath"].(string)
	if code == "" || filePath == "" {
		return nil, fmt.Errorf("projectCode와 filePath는 필수입니다")
	}

	// Platform-API에서 파일 내용 조회
	content, err := fetchFileContent(code, filePath, "main")
	if err != nil {
		// main에 없으면 사용자 브랜치에서 시도
		content, err = fetchFileContent(code, filePath, "docs/"+user)
		if err != nil {
			return nil, fmt.Errorf("파일 조회 실패: %w", err)
		}
	}

	if len(content) == 0 {
		return map[string]interface{}{"summary": "파일이 비어 있습니다."}, nil
	}

	// LLM으로 요약
	prompt := config.LoadPrompt("skill-doc-summarize.txt", map[string]string{
		"FilePath": filePath,
		"Content":  truncate(content, 8000),
	})
	if prompt == "" {
		prompt = fmt.Sprintf("다음 마크다운 문서를 한국어로 간결하게 요약해주세요.\n\n파일: %s\n\n%s", filePath, truncate(content, 8000))
	}

	summary, err := CallLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM 요약 실패: %w", err)
	}

	return map[string]interface{}{
		"projectCode": code,
		"filePath":    filePath,
		"summary":     summary,
	}, nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n...(truncated)"
	}
	return s
}

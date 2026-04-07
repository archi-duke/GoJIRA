package skills

import (
	"context"
	"fmt"

	"gojira-api/config"
)

func init() {
	Register(&Skill{
		ID:          "diff_explain",
		Name:        "Diff 해석",
		Description: "프로젝트 문서의 main 브랜치와 사용자 브랜치 간의 차이를 자연어로 해석합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드" },
				"filePath": { "type": "string", "description": "비교할 파일 경로 (예: srs.md)" }
			},
			"required": ["projectCode", "filePath"]
		}`),
		Execute: executeDiffExplain,
	})
}

func executeDiffExplain(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	filePath, _ := params["filePath"].(string)
	if code == "" || filePath == "" {
		return nil, fmt.Errorf("projectCode와 filePath는 필수입니다")
	}

	mainContent, err1 := fetchFileContent(code, filePath, "main")
	userContent, err2 := fetchFileContent(code, filePath, "docs/"+user)

	if err1 != nil && err2 != nil {
		return nil, fmt.Errorf("파일을 찾을 수 없습니다: %s", filePath)
	}

	if mainContent == userContent {
		return map[string]interface{}{
			"filePath": filePath,
			"hasDiff":  false,
			"summary":  "main 브랜치와 사용자 브랜치의 내용이 동일합니다.",
		}, nil
	}

	prompt := config.LoadPrompt("skill-diff-explain.txt", map[string]string{
		"FilePath":    filePath,
		"MainContent": truncate(mainContent, 4000),
		"UserContent": truncate(userContent, 4000),
	})
	if prompt == "" {
		prompt = fmt.Sprintf(`다음 두 버전의 마크다운 문서를 비교하여 변경사항을 한국어로 요약해주세요.

파일: %s

=== main 브랜치 ===
%s

=== 사용자 브랜치 ===
%s

변경 요약:
- 추가된 내용
- 수정된 내용
- 삭제된 내용`, filePath, truncate(mainContent, 4000), truncate(userContent, 4000))
	}

	explanation, err := CallLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("Diff 해석 실패: %w", err)
	}

	return map[string]interface{}{
		"filePath":    filePath,
		"hasDiff":     true,
		"explanation": explanation,
	}, nil
}

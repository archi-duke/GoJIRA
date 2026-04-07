package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"gojira-api/config"
)

func init() {
	Register(&Skill{
		ID:          "doc_manage",
		Name:        "문서 관리",
		Description: "프로젝트 설계문서를 조회, 요약하거나 main과 사용자 브랜치 간 차이를 분석합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"action": { "type": "string", "description": "수행할 작업", "enum": ["summarize", "diff", "list"] },
				"projectCode": { "type": "string", "description": "프로젝트 코드" },
				"filePath": { "type": "string", "description": "파일 경로 (summarize/diff 시, 예: srs.md)" }
			},
			"required": ["action", "projectCode"]
		}`),
		Execute: executeDocManage,
	})
}

func executeDocManage(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	action, _ := params["action"].(string)
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}

	switch action {
	case "list":
		return docList(code)
	case "summarize":
		return docSummarize(ctx, code, params, user)
	case "diff":
		return docDiff(ctx, code, params, user)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s (list/summarize/diff 중 선택)", action)
	}
}

func docList(code string) (interface{}, error) {
	tree, sc := callPlatformGet(fmt.Sprintf("/DocRepo/%s/tree?ref=main", code))
	if sc >= 400 {
		return nil, fmt.Errorf("파일 목록 조회 실패")
	}
	treeMap, ok := tree.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("트리 파싱 실패")
	}
	items, _ := treeMap["tree"].([]interface{})
	var files []string
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		if t == "tree" {
			continue
		}
		path, _ := m["path"].(string)
		files = append(files, path)
	}
	return map[string]interface{}{"projectCode": code, "files": files, "count": len(files)}, nil
}

func docSummarize(ctx context.Context, code string, params map[string]interface{}, user string) (interface{}, error) {
	filePath, _ := params["filePath"].(string)
	if filePath == "" {
		return nil, fmt.Errorf("summarize: filePath는 필수입니다")
	}

	content, err := fetchFileContent(code, filePath, "main")
	if err != nil {
		content, err = fetchFileContent(code, filePath, "docs/"+user)
		if err != nil {
			return nil, fmt.Errorf("파일 조회 실패: %s", filePath)
		}
	}
	if len(content) == 0 {
		return map[string]interface{}{"summary": "파일이 비어 있습니다."}, nil
	}

	prompt := config.LoadPrompt("skill-doc-summarize.txt", map[string]string{
		"FilePath": filePath, "Content": truncate(content, 8000),
	})
	if prompt == "" {
		prompt = fmt.Sprintf("다음 마크다운 문서를 한국어로 간결하게 요약해주세요.\n\n파일: %s\n\n%s", filePath, truncate(content, 8000))
	}
	summary, err := CallLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("요약 실패: %w", err)
	}
	return map[string]interface{}{"projectCode": code, "filePath": filePath, "summary": summary}, nil
}

func docDiff(ctx context.Context, code string, params map[string]interface{}, user string) (interface{}, error) {
	filePath, _ := params["filePath"].(string)
	if filePath == "" {
		return nil, fmt.Errorf("diff: filePath는 필수입니다")
	}

	mainContent, err1 := fetchFileContent(code, filePath, "main")
	userContent, err2 := fetchFileContent(code, filePath, "docs/"+user)
	if err1 != nil && err2 != nil {
		return nil, fmt.Errorf("파일을 찾을 수 없습니다: %s", filePath)
	}
	if mainContent == userContent {
		return map[string]interface{}{"filePath": filePath, "hasDiff": false, "summary": "차이가 없습니다."}, nil
	}

	prompt := config.LoadPrompt("skill-diff-explain.txt", map[string]string{
		"FilePath": filePath, "MainContent": truncate(mainContent, 4000), "UserContent": truncate(userContent, 4000),
	})
	if prompt == "" {
		prompt = fmt.Sprintf("두 버전의 마크다운 문서를 비교하여 변경사항을 한국어로 요약해주세요.\n\n파일: %s\n\n=== main ===\n%s\n\n=== user ===\n%s",
			filePath, truncate(mainContent, 4000), truncate(userContent, 4000))
	}
	explanation, err := CallLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("Diff 해석 실패: %w", err)
	}
	return map[string]interface{}{"filePath": filePath, "hasDiff": true, "explanation": explanation}, nil
}

// callPlatformGet Platform-API GET 요청 헬퍼
func callPlatformGet(path string) (interface{}, int) {
	resp, err := platformClient.Get(config.PlatformAPI + path)
	if err != nil {
		return nil, 500
	}
	defer resp.Body.Close()
	var result interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, resp.StatusCode
}

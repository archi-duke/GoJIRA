package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"gojira-api/config"
)

func init() {
	Register(&Skill{
		ID:          "pr_create",
		Name:        "PR 생성",
		Description: "프로젝트의 사용자 브랜치에서 main으로 Pull Request를 생성합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드" },
				"title": { "type": "string", "description": "PR 제목 (생략 시 자동 생성)" },
				"body": { "type": "string", "description": "PR 본문 (생략 시 자동 생성)" }
			},
			"required": ["projectCode"]
		}`),
		Execute: executePRCreate,
	})
}

func executePRCreate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}

	title, _ := params["title"].(string)
	body, _ := params["body"].(string)
	if title == "" {
		title = fmt.Sprintf("[%s] 설계문서 배포 요청", code)
	}

	head := fmt.Sprintf("docs/%s", user)

	reqBody := map[string]string{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  "main",
	}
	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/DocRepo/%s/pr", config.PlatformAPI, code),
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", user)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PR 생성 요청 실패: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		var errResp map[string]string
		json.Unmarshal(data, &errResp)
		return nil, fmt.Errorf("PR 생성 실패: %s", errResp["error"])
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	prNum := result["number"]
	prURL, _ := result["html_url"].(string)

	log.Printf("Agent: PR 생성 %s #%v", code, prNum)

	return map[string]interface{}{
		"projectCode": code,
		"prNumber":    prNum,
		"prURL":       prURL,
		"title":       title,
		"head":        head,
	}, nil
}

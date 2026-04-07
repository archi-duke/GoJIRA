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
		ID:          "pr_manage",
		Name:        "PR 관리",
		Description: "Pull Request를 생성하거나 목록을 조회합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"action": { "type": "string", "description": "수행할 작업", "enum": ["create", "list"] },
				"projectCode": { "type": "string", "description": "프로젝트 코드" },
				"title": { "type": "string", "description": "PR 제목 (create 시, 생략하면 자동 생성)" },
				"body": { "type": "string", "description": "PR 본문 (create 시)" },
				"state": { "type": "string", "description": "PR 상태 필터 (list 시)", "enum": ["open", "closed", "all"] }
			},
			"required": ["action", "projectCode"]
		}`),
		Execute: executePRManage,
	})
}

func executePRManage(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	action, _ := params["action"].(string)
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}

	switch action {
	case "create":
		return prCreate(code, params, user)
	case "list":
		return prList(code, params)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s (create/list 중 선택)", action)
	}
}

func prCreate(code string, params map[string]interface{}, user string) (interface{}, error) {
	title, _ := params["title"].(string)
	body, _ := params["body"].(string)
	if title == "" {
		title = fmt.Sprintf("[%s] 설계문서 배포 요청", code)
	}
	head := fmt.Sprintf("docs/%s", user)

	reqBody := map[string]string{"title": title, "body": body, "head": head, "base": "main"}
	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/DocRepo/%s/pr", config.PlatformAPI, code), bytes.NewReader(b))
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
	log.Printf("Agent: PR 생성 %s #%v", code, result["number"])
	return map[string]interface{}{
		"projectCode": code, "prNumber": result["number"], "prURL": result["html_url"], "title": title, "head": head,
	}, nil
}

func prList(code string, params map[string]interface{}) (interface{}, error) {
	state, _ := params["state"].(string)
	if state == "" {
		state = "open"
	}
	resp, err := http.Get(fmt.Sprintf("%s/DocRepo/%s/prs?state=%s", config.PlatformAPI, code, state))
	if err != nil {
		return nil, fmt.Errorf("PR 목록 조회 실패: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var prs []interface{}
	json.Unmarshal(data, &prs)

	type PRSummary struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		State  string `json:"state"`
		User   string `json:"user"`
	}
	var results []PRSummary
	for _, pr := range prs {
		pm, ok := pr.(map[string]interface{})
		if !ok {
			continue
		}
		num, _ := pm["number"].(float64)
		title, _ := pm["title"].(string)
		st, _ := pm["state"].(string)
		usr := ""
		if u, ok := pm["user"].(map[string]interface{}); ok {
			usr, _ = u["login"].(string)
		}
		results = append(results, PRSummary{Number: int(num), Title: title, State: st, User: usr})
	}
	return map[string]interface{}{"projectCode": code, "state": state, "count": len(results), "prs": results}, nil
}

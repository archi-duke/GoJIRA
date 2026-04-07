package skills

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
)

func init() {
	Register(&Skill{
		ID:          "issue_query",
		Name:        "이슈 조회",
		Description: "프로젝트의 이슈 목록을 조회합니다. issueType으로 필터링할 수 있습니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드 (예: GOJIRA)" },
				"issueType": { "type": "string", "description": "이슈 타입 필터 (VOS, Feature, Requirement, Architectural Design, Detailed Design). 생략하면 전체 조회" }
			},
			"required": ["projectCode"]
		}`),
		Execute: executeIssueQuery,
	})
}

func executeIssueQuery(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	if code == "" {
		return nil, fmt.Errorf("projectCode는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{"key": bson.M{"$regex": "^" + code + "-"}}
	if issueType, ok := params["issueType"].(string); ok && issueType != "" {
		filter["issueType"] = issueType
	}

	issues, err := db.FindAllIssues(qCtx, filter)
	if err != nil {
		return nil, fmt.Errorf("이슈 조회 실패: %w", err)
	}

	// 요약 형태로 반환
	type IssueSummary struct {
		Key       string `json:"key"`
		Type      string `json:"issueType"`
		Summary   string `json:"summary"`
		Status    string `json:"status"`
		Assignee  string `json:"assignee"`
	}

	results := make([]IssueSummary, 0, len(issues))
	for _, issue := range issues {
		key, _ := issue["key"].(string)
		iType, _ := issue["issueType"].(string)
		summary, _ := issue["summary"].(string)
		status, _ := issue["status"].(string)
		assignee, _ := issue["assignee"].(string)
		results = append(results, IssueSummary{Key: key, Type: iType, Summary: summary, Status: status, Assignee: assignee})
	}

	return map[string]interface{}{
		"projectCode": code,
		"count":       len(results),
		"issues":      results,
	}, nil
}

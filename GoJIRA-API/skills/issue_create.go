package skills

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
)

func init() {
	Register(&Skill{
		ID:          "issue_create",
		Name:        "이슈 생성",
		Description: "프로젝트에 새 이슈를 생성합니다. issueType은 VOS, Feature, Requirement, Architectural Design, Detailed Design 중 하나입니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "프로젝트 코드 (예: GOJIRA)" },
				"issueType": { "type": "string", "description": "이슈 타입", "enum": ["VOS", "Feature", "Requirement", "Architectural Design", "Detailed Design"] },
				"summary": { "type": "string", "description": "이슈 제목" },
				"description": { "type": "string", "description": "이슈 설명" },
				"priority": { "type": "string", "description": "우선순위", "enum": ["Highest", "High", "Medium", "Low", "Lowest"] },
				"assignee": { "type": "string", "description": "담당자 ID" }
			},
			"required": ["projectCode", "issueType", "summary"]
		}`),
		Execute: executeIssueCreate,
	})
}

func executeIssueCreate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	issueType, _ := params["issueType"].(string)
	summary, _ := params["summary"].(string)
	if code == "" || issueType == "" || summary == "" {
		return nil, fmt.Errorf("projectCode, issueType, summary는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 다음 이슈 번호 생성
	nextNum, err := getNextIssueNumber(qCtx, code)
	if err != nil {
		return nil, fmt.Errorf("이슈 번호 생성 실패: %w", err)
	}
	key := fmt.Sprintf("%s-%d", code, nextNum)

	description, _ := params["description"].(string)
	priority, _ := params["priority"].(string)
	if priority == "" {
		priority = "Medium"
	}
	assignee, _ := params["assignee"].(string)

	doc := bson.M{
		"key":         key,
		"issueType":   issueType,
		"summary":     summary,
		"description": description,
		"status":      "To Do",
		"priority":    priority,
		"assignee":    assignee,
		"reporter":    user,
		"created":     time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"updated":     time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"labels":      []string{},
		"issueLinks":  []interface{}{},
		"_source":     "gojira-agent",
	}

	if err := db.UpsertIssue(qCtx, key, doc); err != nil {
		return nil, fmt.Errorf("이슈 생성 실패: %w", err)
	}

	log.Printf("Agent: 이슈 생성 %s (%s) %s", key, issueType, summary)

	return map[string]interface{}{
		"key":       key,
		"issueType": issueType,
		"summary":   summary,
		"status":    "To Do",
	}, nil
}

func getNextIssueNumber(ctx context.Context, projectCode string) (int, error) {
	filter := bson.M{"key": bson.M{"$regex": "^" + projectCode + "-"}}
	issues, err := db.FindAllIssues(ctx, filter)
	if err != nil {
		return 1, err
	}
	maxNum := 0
	prefix := projectCode + "-"
	for _, issue := range issues {
		key, _ := issue["key"].(string)
		if len(key) > len(prefix) {
			var num int
			fmt.Sscanf(key[len(prefix):], "%d", &num)
			if num > maxNum {
				maxNum = num
			}
		}
	}
	return maxNum + 1, nil
}

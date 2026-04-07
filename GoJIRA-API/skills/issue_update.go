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
		ID:          "issue_update",
		Name:        "이슈 수정",
		Description: "기존 이슈의 필드를 수정합니다. 상태, 요약, 설명, 우선순위, 담당자 등을 변경할 수 있습니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"key": { "type": "string", "description": "이슈 키 (예: GOJIRA-1)" },
				"summary": { "type": "string", "description": "변경할 제목" },
				"description": { "type": "string", "description": "변경할 설명" },
				"status": { "type": "string", "description": "변경할 상태", "enum": ["To Do", "In Progress", "Done"] },
				"priority": { "type": "string", "description": "변경할 우선순위", "enum": ["Highest", "High", "Medium", "Low", "Lowest"] },
				"assignee": { "type": "string", "description": "변경할 담당자 ID" }
			},
			"required": ["key"]
		}`),
		Execute: executeIssueUpdate,
	})
}

func executeIssueUpdate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("key는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 기존 이슈 확인
	existing, err := db.FindIssueByKey(qCtx, key)
	if err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", key)
	}

	update := bson.M{
		"updated": time.Now().Format("2006-01-02T15:04:05.000+0900"),
	}

	changed := []string{}
	for _, field := range []string{"summary", "description", "status", "priority", "assignee"} {
		if v, ok := params[field].(string); ok && v != "" {
			update[field] = v
			changed = append(changed, field)
		}
	}

	if len(changed) == 0 {
		return map[string]interface{}{"key": key, "message": "변경할 항목이 없습니다"}, nil
	}

	if err := db.UpsertIssue(qCtx, key, update); err != nil {
		return nil, fmt.Errorf("이슈 수정 실패: %w", err)
	}

	log.Printf("Agent: 이슈 수정 %s fields=%v", key, changed)

	summary, _ := existing["summary"].(string)
	return map[string]interface{}{
		"key":     key,
		"summary": summary,
		"changed": changed,
	}, nil
}

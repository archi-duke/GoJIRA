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
		ID:          "issue_link",
		Name:        "이슈 연결",
		Description: "두 이슈 간의 관계를 설정합니다. contain, relate, duplicate 등의 관계를 지원합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"fromKey": { "type": "string", "description": "출발 이슈 키 (예: GOJIRA-1)" },
				"toKey": { "type": "string", "description": "도착 이슈 키 (예: GOJIRA-2)" },
				"linkType": { "type": "string", "description": "관계 유형", "enum": ["contain", "relate", "duplicate", "block"] }
			},
			"required": ["fromKey", "toKey", "linkType"]
		}`),
		Execute: executeIssueLink,
	})
}

func executeIssueLink(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	fromKey, _ := params["fromKey"].(string)
	toKey, _ := params["toKey"].(string)
	linkType, _ := params["linkType"].(string)
	if fromKey == "" || toKey == "" || linkType == "" {
		return nil, fmt.Errorf("fromKey, toKey, linkType는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 양쪽 이슈 존재 확인
	if _, err := db.FindIssueByKey(qCtx, fromKey); err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", fromKey)
	}
	if _, err := db.FindIssueByKey(qCtx, toKey); err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", toKey)
	}

	// outward 방향 (from → to)
	linkID := fmt.Sprintf("%s_%s_%s", fromKey, linkType, toKey)
	outward := bson.M{
		"id":       linkID,
		"type":     linkType,
		"outward":  toKey,
		"_created": time.Now(),
	}
	// from 이슈에 link 추가
	db.Issues().UpdateOne(qCtx,
		bson.M{"key": fromKey},
		bson.M{"$addToSet": bson.M{"issueLinks": outward}},
	)

	// inward 방향 (to ← from)
	inward := bson.M{
		"id":       linkID,
		"type":     linkType,
		"inward":   fromKey,
		"_created": time.Now(),
	}
	db.Issues().UpdateOne(qCtx,
		bson.M{"key": toKey},
		bson.M{"$addToSet": bson.M{"issueLinks": inward}},
	)

	log.Printf("Agent: 이슈 연결 %s → %s (%s)", fromKey, toKey, linkType)

	return map[string]interface{}{
		"linkId":   linkID,
		"fromKey":  fromKey,
		"toKey":    toKey,
		"linkType": linkType,
	}, nil
}

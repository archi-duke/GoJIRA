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
		ID:          "issue_manage",
		Name:        "이슈 관리",
		Description: "이슈를 생성, 수정, 삭제하거나 이슈 간 관계를 설정합니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"action": { "type": "string", "description": "수행할 작업", "enum": ["create", "update", "delete", "link"] },
				"projectCode": { "type": "string", "description": "프로젝트 코드 (create 시 필수)" },
				"key": { "type": "string", "description": "이슈 키 (update/delete 시 필수)" },
				"issueType": { "type": "string", "description": "이슈 타입 (create 시 필수)", "enum": ["VOS", "Feature", "Requirement", "Architectural Design", "Detailed Design"] },
				"summary": { "type": "string", "description": "이슈 제목" },
				"description": { "type": "string", "description": "이슈 설명" },
				"status": { "type": "string", "description": "상태 (update 시)", "enum": ["To Do", "In Progress", "Done"] },
				"priority": { "type": "string", "description": "우선순위", "enum": ["Highest", "High", "Medium", "Low", "Lowest"] },
				"assignee": { "type": "string", "description": "담당자 ID" },
				"fromKey": { "type": "string", "description": "연결 출발 이슈 (link 시)" },
				"toKey": { "type": "string", "description": "연결 도착 이슈 (link 시)" },
				"linkType": { "type": "string", "description": "관계 유형 (link 시)", "enum": ["contain", "relate", "duplicate", "block"] }
			},
			"required": ["action"]
		}`),
		Execute: executeIssueManage,
	})
}

func executeIssueManage(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	action, _ := params["action"].(string)
	switch action {
	case "create":
		return issueCreate(ctx, params, user)
	case "update":
		return issueUpdate(ctx, params, user)
	case "delete":
		return issueDelete(ctx, params, user)
	case "link":
		return issueLink(ctx, params, user)
	default:
		return nil, fmt.Errorf("지원하지 않는 action: %s (create/update/delete/link 중 선택)", action)
	}
}

func issueCreate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	code, _ := params["projectCode"].(string)
	issueType, _ := params["issueType"].(string)
	summary, _ := params["summary"].(string)
	if code == "" || issueType == "" || summary == "" {
		return nil, fmt.Errorf("create: projectCode, issueType, summary는 필수입니다")
	}

	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

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
		"key": key, "issueType": issueType, "summary": summary,
		"description": description, "status": "To Do", "priority": priority,
		"assignee": assignee, "reporter": user,
		"created": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"updated": time.Now().Format("2006-01-02T15:04:05.000+0900"),
		"labels": []string{}, "issueLinks": []interface{}{},
		"_source": "gojira-agent",
	}
	if err := db.UpsertIssue(qCtx, key, doc); err != nil {
		return nil, fmt.Errorf("이슈 생성 실패: %w", err)
	}
	log.Printf("Agent: 이슈 생성 %s (%s) %s", key, issueType, summary)
	return map[string]interface{}{"key": key, "issueType": issueType, "summary": summary, "status": "To Do"}, nil
}

func issueUpdate(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("update: key는 필수입니다")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	existing, err := db.FindIssueByKey(qCtx, key)
	if err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", key)
	}

	update := bson.M{"updated": time.Now().Format("2006-01-02T15:04:05.000+0900")}
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
	return map[string]interface{}{"key": key, "summary": summary, "changed": changed}, nil
}

func issueDelete(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	key, _ := params["key"].(string)
	if key == "" {
		return nil, fmt.Errorf("delete: key는 필수입니다")
	}
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := db.FindIssueByKey(qCtx, key); err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", key)
	}
	if err := db.DeleteIssueByKey(qCtx, key); err != nil {
		return nil, fmt.Errorf("이슈 삭제 실패: %w", err)
	}
	log.Printf("Agent: 이슈 삭제 %s", key)
	return map[string]interface{}{"key": key, "deleted": true}, nil
}

func issueLink(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	fromKey, _ := params["fromKey"].(string)
	toKey, _ := params["toKey"].(string)
	linkType, _ := params["linkType"].(string)
	if fromKey == "" || toKey == "" || linkType == "" {
		return nil, fmt.Errorf("link: fromKey, toKey, linkType는 필수입니다")
	}
	qCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if _, err := db.FindIssueByKey(qCtx, fromKey); err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", fromKey)
	}
	if _, err := db.FindIssueByKey(qCtx, toKey); err != nil {
		return nil, fmt.Errorf("이슈를 찾을 수 없습니다: %s", toKey)
	}

	linkID := fmt.Sprintf("%s_%s_%s", fromKey, linkType, toKey)
	db.Issues().UpdateOne(qCtx, bson.M{"key": fromKey},
		bson.M{"$addToSet": bson.M{"issueLinks": bson.M{"id": linkID, "type": linkType, "outward": toKey, "_created": time.Now()}}})
	db.Issues().UpdateOne(qCtx, bson.M{"key": toKey},
		bson.M{"$addToSet": bson.M{"issueLinks": bson.M{"id": linkID, "type": linkType, "inward": fromKey, "_created": time.Now()}}})

	log.Printf("Agent: 이슈 연결 %s → %s (%s)", fromKey, toKey, linkType)
	return map[string]interface{}{"linkId": linkID, "fromKey": fromKey, "toKey": toKey, "linkType": linkType}, nil
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

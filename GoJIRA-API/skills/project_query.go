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
		ID:          "project_query",
		Name:        "프로젝트 조회",
		Description: "등록된 프로젝트 목록을 조회합니다. 프로젝트 수, 이름, 상태 등을 확인할 수 있습니다.",
		Parameters: MustJSON(`{
			"type": "object",
			"properties": {
				"projectCode": { "type": "string", "description": "특정 프로젝트 코드로 필터링 (생략하면 전체 조회)" }
			}
		}`),
		Execute: executeProjectQuery,
	})
}

func executeProjectQuery(ctx context.Context, params map[string]interface{}, user string) (interface{}, error) {
	qCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	projects, err := db.FindAllProjects(qCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("프로젝트 조회 실패: %w", err)
	}

	filterCode, _ := params["projectCode"].(string)

	type ProjectSummary struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	var results []ProjectSummary
	for _, prj := range projects {
		code := extractCode(prj)
		if code == "" {
			continue
		}
		if filterCode != "" && code != filterCode {
			continue
		}
		name, _ := prj["name"].(string)
		status, _ := prj["status"].(string)
		if status == "Deleted" {
			continue
		}
		results = append(results, ProjectSummary{Code: code, Name: name, Status: status})
	}

	return map[string]interface{}{
		"count":    len(results),
		"projects": results,
	}, nil
}

func extractCode(prj map[string]interface{}) string {
	return bsonStr(bsonDoc(bsonDoc(prj, "jiraCodes"), "main"), "jiraCode")
}

func bsonDoc(doc interface{}, key string) interface{} {
	switch d := doc.(type) {
	case map[string]interface{}:
		return d[key]
	case bson.M:
		return d[key]
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	}
	return nil
}

func bsonStr(doc interface{}, key string) string {
	switch d := doc.(type) {
	case map[string]interface{}:
		s, _ := d[key].(string)
		return s
	case bson.M:
		s, _ := d[key].(string)
		return s
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				s, _ := e.Value.(string)
				return s
			}
		}
	}
	return ""
}

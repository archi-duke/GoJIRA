package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"gojira-api/db"
)

// GET /api/v{version}/Project/alerts?user={userId}
// 전체 프로젝트의 알림 상태를 DB 플래그 기반으로 반환
// 웹훅/이벤트로 갱신되는 branchAlerts, gitOpenPR, gitLastCommit 필드 사용
func ProjectAlertsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	projects, err := db.FindAllProjects(ctx, nil)
	if err != nil {
		log.Printf("alerts: FindAllProjects error: %v", err)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type AlertInfo struct {
		JiraCode   string `json:"jiraCode"`
		BranchDiff bool   `json:"branchDiff"`
		HasOpenPR  bool   `json:"hasOpenPR"`
	}

	alerts := []AlertInfo{}

	for _, prj := range projects {
		code := extractJiraCode(prj)
		if code == "" {
			continue
		}
		status, _ := prj["status"].(string)
		if status == "Deleted" {
			continue
		}

		alert := AlertInfo{JiraCode: code}

		// 1. 브랜치 차이: branchAlerts 배열에 사용자가 포함되어 있거나 gitLastCommit 플래그
		if userID != "" {
			if users := bsonGetStrArray(prj, "branchAlerts"); len(users) > 0 {
				for _, u := range users {
					if u == userID {
						alert.BranchDiff = true
						break
					}
				}
			}
		}
		if !alert.BranchDiff {
			gitLastCommit := getTime(prj, "gitLastCommit")
			if !gitLastCommit.IsZero() {
				alert.BranchDiff = true
			}
		}

		// 2. 열린 PR
		if openPR, ok := prj["gitOpenPR"].(bool); ok && openPR {
			alert.HasOpenPR = true
		}

		if alert.BranchDiff || alert.HasOpenPR {
			alerts = append(alerts, alert)
		}
	}

	respondJSON(w, http.StatusOK, alerts)
}

func bsonGetStrArray(doc bson.M, key string) []string {
	switch v := doc[key].(type) {
	case []string:
		return v
	case []interface{}:
		arr := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				arr = append(arr, s)
			}
		}
		return arr
	case bson.A:
		arr := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				arr = append(arr, s)
			}
		}
		return arr
	}
	return nil
}

func getTime(doc bson.M, key string) time.Time {
	switch v := doc[key].(type) {
	case time.Time:
		return v
	case bson.DateTime:
		return v.Time()
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000+0900", v); err == nil {
			return t
		}
	}
	return time.Time{}
}

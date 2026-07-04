package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	authmw "gojira-api/auth"
	"gojira-api/config"
	"gojira-api/db"
)

// GET /api/v{version}/Project/alerts?user={userId}
// 전체 프로젝트의 알림 상태를 DB 플래그 기반으로 반환
// 웹훅/이벤트로 갱신되는 branchAlerts, gitOpenPR, gitLastCommit 필드 사용
func ProjectAlertsHandler(w http.ResponseWriter, r *http.Request) {
	userID := authmw.UserID(r.Context())

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
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

		// 1. 브랜치 차이: main과 사용자 브랜치 트리 비교 (파일 SHA 기반)
		if userID != "" {
			alert.BranchDiff = checkTreeDiff(code, userID)
		}

		// 2. 열린 PR — 리뷰어로 지정된 경우만
		if userID != "" {
			if reviewers := bsonGetStrArray(prj, "gitOpenPRReviewers"); len(reviewers) > 0 {
				for _, rv := range reviewers {
					if rv == userID {
						alert.HasOpenPR = true
						break
					}
				}
			}
		}

		if alert.BranchDiff || alert.HasOpenPR {
			alerts = append(alerts, alert)
		}
	}

	respondJSON(w, http.StatusOK, alerts)
}

// checkTreeDiff Platform-API를 통해 main과 사용자 브랜치 트리를 비교
func checkTreeDiff(projectCode, userID string) bool {
	type treeFile struct {
		Path string `json:"path"`
		SHA  string `json:"sha"`
		Type string `json:"type"`
	}
	type treeResp struct {
		Tree []treeFile `json:"tree"`
	}

	fetchTree := func(ref string) map[string]string {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/DocRepo/%s/tree?ref=%s",
			config.PlatformAPI, projectCode, url.QueryEscape(ref)), nil)
		// 서비스간 read — Platform-API auth 미들웨어 통과용 서비스 토큰(SSO 모드).
		if st := os.Getenv("SERVICE_TOKEN"); st != "" {
			req.Header.Set("X-Service-Token", st)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode >= 400 {
			return nil
		}
		defer resp.Body.Close()
		var t treeResp
		json.NewDecoder(resp.Body).Decode(&t)
		m := map[string]string{}
		for _, f := range t.Tree {
			if f.Type != "tree" {
				m[f.Path] = f.SHA
			}
		}
		return m
	}

	mainFiles := fetchTree("main")
	userFiles := fetchTree("docs/" + userID)
	if mainFiles == nil || userFiles == nil {
		return false
	}
	// docs/{user} 브랜치 미존재 시 Platform-API 가 200 + 빈 tree 로 응답하므로
	// 빈 사용자 트리는 "브랜치 없음 = diff 없음" 으로 판정
	if len(userFiles) == 0 {
		return false
	}
	if len(mainFiles) != len(userFiles) {
		return true
	}
	for p, sha := range mainFiles {
		if userFiles[p] != sha {
			return true
		}
	}
	return false
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

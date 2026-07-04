package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	authmw "gojira-api/auth"
	"gojira-api/db"
)

// GET /api/v{version}/Project/alerts?user={userId}
// 전체 프로젝트의 알림 상태를 DB 플래그 기반으로 반환 — 라이브 git 비교 없음.
//   - branchDiff: 프로젝트 branchAlerts 배열(웹훅이 push 마다 갱신)에 사용자 포함 여부
//   - hasOpenPR : gitOpenPRReviewers 배열에 사용자 포함 여부
// branchAlerts 필드가 아직 계산된 적 없는(미존재) 프로젝트는 Platform-API 의
// /alerts/recompute 를 비동기로 호출해 self-healing 한다 (응답은 기다리지 않음 —
// 계산 완료 시 SSE project_alert 로 웹에 반영된다).
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

		if userID != "" {
			// 1. 브랜치 차이 — 웹훅이 갱신하는 branchAlerts 플래그만 읽는다
			for _, u := range bsonGetStrArray(prj, "branchAlerts") {
				if u == userID {
					alert.BranchDiff = true
					break
				}
			}
			// 필드 자체가 없으면(웹훅 이전 데이터) 비동기 재계산 요청 — 다음
			// 조회/SSE 부터 반영. repo 없는 프로젝트는 계산할 것이 없어 제외.
			if prj["branchAlerts"] == nil && docRepoName(prj) != "" {
				go recomputeBranchAlerts(code)
			}

			// 2. 열린 PR — 리뷰어로 지정된 경우만
			for _, rv := range bsonGetStrArray(prj, "gitOpenPRReviewers") {
				if rv == userID {
					alert.HasOpenPR = true
					break
				}
			}
		}

		if alert.BranchDiff || alert.HasOpenPR {
			alerts = append(alerts, alert)
		}
	}

	respondJSON(w, http.StatusOK, alerts)
}

// recomputeBranchAlerts Platform-API 에 branchAlerts 재계산을 위임 (fire-and-forget)
func recomputeBranchAlerts(code string) {
	if _, sc := callPlatform("POST", fmt.Sprintf("/DocRepo/%s/alerts/recompute", code), nil); sc >= 400 {
		log.Printf("alerts: %s branchAlerts 재계산 요청 실패 (status=%d)", code, sc)
	}
}

// docRepoName 프로젝트 documents 에서 repoName 추출
func docRepoName(prj bson.M) string {
	switch docs := prj["documents"].(type) {
	case bson.M:
		s, _ := docs["repoName"].(string)
		return s
	case bson.D:
		for _, e := range docs {
			if e.Key == "repoName" {
				s, _ := e.Value.(string)
				return s
			}
		}
	}
	return ""
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

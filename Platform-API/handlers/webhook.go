package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// POST /DocRepo/{code}/alerts/recompute — branchAlerts 플래그 전체 재계산.
// 웹훅 이전에 만들어져 플래그가 계산된 적 없는(필드 미존재) 프로젝트의
// self-healing 용 — GoJIRA-API /Project/alerts 가 비동기로 호출한다.
func DocRepoAlertsRecomputeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	alerts := compareAllBranches(owner, name, code)
	if alerts == nil {
		alerts = []string{}
	}
	db.UpdateProjectField(ctx, code, "branchAlerts", alerts)
	BroadcastDocEvent(map[string]interface{}{
		"type":         "project_alert",
		"repo":         name,
		"projectCode":  code,
		"branchAlerts": alerts,
	})
	respondJSON(w, http.StatusOK, map[string]interface{}{"branchAlerts": alerts})
}

// POST /webhook/github — GitHub Webhook 수신
func GitHubWebhookHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}

	// 시그니처 검증 (secret이 설정된 경우)
	if config.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig != "" && !verifySignature(body, sig, config.WebhookSecret) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event := r.Header.Get("X-GitHub-Event")
	log.Printf("webhook: event=%s", event)

	var payload map[string]interface{}
	json.Unmarshal(body, &payload)

	switch event {
	case "pull_request":
		handlePREvent(payload)
	case "pull_request_review", "pull_request_rejected", "pull_request_approved":
		handlePRReviewEvent(payload, event)
	case "push":
		handlePushEvent(payload)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func handlePREvent(payload map[string]interface{}) {
	action, _ := payload["action"].(string)
	pr, _ := payload["pull_request"].(map[string]interface{})
	if pr == nil {
		return
	}

	number := pr["number"]
	title, _ := pr["title"].(string)
	state, _ := pr["state"].(string)
	merged, _ := pr["merged"].(bool)
	repo, _ := payload["repository"].(map[string]interface{})
	repoName, _ := repo["name"].(string)

	log.Printf("webhook PR: action=%s #%v %s state=%s merged=%v repo=%s", action, number, title, state, merged, repoName)

	// DB 프로젝트 알림 갱신
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projectCode := findProjectByRepo(ctx, repoName)
	log.Printf("webhook PR: projectCode=%s reviewers lookup...", projectCode)

	mergeable, _ := pr["mergeable"].(bool)
	mergeableState, _ := pr["mergeable_state"].(string)

	// requested_reviewers 추출
	var reviewers []string
	if rr, ok := pr["requested_reviewers"].([]interface{}); ok {
		for _, r := range rr {
			if rm, ok := r.(map[string]interface{}); ok {
				if login, ok := rm["login"].(string); ok {
					reviewers = append(reviewers, login)
				}
			}
		}
	}

	if projectCode != "" {
		switch {
		case action == "opened" || action == "reopened" || action == "review_requested":
			if len(reviewers) > 0 {
				db.UpdateProjectField(ctx, projectCode, "gitOpenPRReviewers", reviewers)
			}
		case state == "closed" && !merged:
			owner := ""
			if o, ok := repo["owner"].(map[string]interface{}); ok {
				owner, _ = o["login"].(string)
			}
			prs, _ := github.ListPRs(owner, repoName, "open")
			if prList, ok := prs.([]interface{}); ok && len(prList) == 0 {
				db.UpdateProjectField(ctx, projectCode, "gitOpenPRReviewers", []string{})
			}
		case merged:
			owner := ""
			if o, ok := repo["owner"].(map[string]interface{}); ok {
				owner, _ = o["login"].(string)
			}
			prs, _ := github.ListPRs(owner, repoName, "open")
			if prList, ok := prs.([]interface{}); ok && len(prList) == 0 {
				db.UpdateProjectField(ctx, projectCode, "gitOpenPRReviewers", []string{})
			}
		}
	}

	// SSE 브로드캐스트 — PR 상태 변경 알림
	BroadcastDocEvent(map[string]interface{}{
		"type":           "pr_update",
		"action":         action,
		"repo":           repoName,
		"projectCode":    projectCode,
		"prNumber":       number,
		"title":          title,
		"state":          state,
		"merged":         merged,
		"mergeable":      mergeable,
		"mergeableState": mergeableState,
		"reviewers":      reviewers,
	})
}

func handlePRReviewEvent(payload map[string]interface{}, webhookEvent string) {
	action, _ := payload["action"].(string)
	review, _ := payload["review"].(map[string]interface{})
	pr, _ := payload["pull_request"].(map[string]interface{})
	repo, _ := payload["repository"].(map[string]interface{})
	if pr == nil {
		return
	}

	repoName, _ := repo["name"].(string)
	number := pr["number"]
	reviewState := ""
	reviewerLogin := ""
	reviewBody := ""
	if review != nil {
		reviewState, _ = review["state"].(string)
		if reviewer, ok := review["user"].(map[string]interface{}); ok {
			reviewerLogin, _ = reviewer["login"].(string)
		}
		reviewBody, _ = review["body"].(string)
	}

	// Gitea 이벤트명으로 state 보정
	if webhookEvent == "pull_request_rejected" {
		reviewState = "changes_requested"
	} else if webhookEvent == "pull_request_approved" {
		reviewState = "approved"
	}

	prURL, _ := pr["html_url"].(string)
	prTitle, _ := pr["title"].(string)
	prUser, _ := pr["user"].(map[string]interface{})
	prCreator, _ := prUser["login"].(string)

	log.Printf("webhook PR review: event=%s action=%s #%v reviewer=%s state=%s repo=%s", webhookEvent, action, number, reviewerLogin, reviewState, repoName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projectCode := findProjectByRepo(ctx, repoName)

	prNum := 0
	if n, ok := number.(float64); ok {
		prNum = int(n)
	}

	BroadcastDocEvent(map[string]interface{}{
		"type":        "pr_review",
		"action":      action,
		"repo":        repoName,
		"projectCode": projectCode,
		"prNumber":    number,
		"reviewer":    reviewerLogin,
		"state":       reviewState,
		"body":        reviewBody,
		"prCreator":   prCreator,
	})

	// 반려 시 PR 생성자에게 알림 저장 (미접속 사용자용)
	if reviewState == "rejected" || reviewState == "changes_requested" {
		msg := fmt.Sprintf("[%s] PR #%d이 %s님에 의해 반려되었습니다: %s", projectCode, prNum, reviewerLogin, reviewBody)
		db.SaveNotification(ctx, prCreator, "pr_rejected", projectCode, msg, bson.M{
			"prNumber": prNum,
			"prTitle":  prTitle,
			"reviewer": reviewerLogin,
			"body":     reviewBody,
		})
	}

	// 승인 리뷰이고, 대기 중인 리뷰어가 없으면 머지 가능 알림
	if reviewState == "approved" {
		requested, _ := pr["requested_reviewers"].([]interface{})
		if len(requested) == 0 {
			prNum := 0
			if n, ok := number.(float64); ok {
				prNum = int(n)
			}
			notifyMergeable(repoName, prNum, prTitle, prURL, prCreator)
		}
	}
}

func handlePushEvent(payload map[string]interface{}) {
	ref, _ := payload["ref"].(string)
	repo, _ := payload["repository"].(map[string]interface{})
	repoName, _ := repo["name"].(string)
	repoOwner := ""
	if owner, ok := repo["owner"].(map[string]interface{}); ok {
		repoOwner, _ = owner["login"].(string)
	}

	// docs/{user} push — 해당 사용자의 branchAlerts 플래그만 갱신 (등록/발행 직후 반영).
	// 알림 API(GoJIRA-API /Project/alerts)는 이 플래그를 읽기만 하므로 여기서 갱신을
	// 놓치면 마크가 안 뜬다.
	if idx := strings.Index(ref, "/docs/"); idx >= 0 {
		userID := ref[idx+len("/docs/"):]
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		projectCode := findProjectByRepo(ctx, repoName)
		if projectCode == "" || userID == "" {
			return
		}
		alerts := UpdateBranchAlert(repoOwner, repoName, projectCode, userID)
		BroadcastDocEvent(map[string]interface{}{
			"type":         "project_alert",
			"repo":         repoName,
			"projectCode":  projectCode,
			"ref":          ref,
			"branchAlerts": alerts,
		})
		return
	}

	// main 브랜치 push만 처리
	if !strings.HasSuffix(ref, "/main") && !strings.HasSuffix(ref, "/master") {
		return
	}

	log.Printf("webhook push: %s/%s → %s", repoOwner, repoName, ref)

	// 해당 레포를 사용하는 프로젝트 찾기
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	projectCode := findProjectByRepo(ctx, repoName)
	if projectCode == "" {
		log.Printf("webhook push: 레포 %s에 매칭되는 프로젝트 없음", repoName)
		BroadcastDocEvent(map[string]interface{}{"type": "push", "repo": repoName, "ref": ref})
		return
	}

	// §Phase 2 — data/issues/ 변경 시 GoJIRA-API 캐시 reconcile. 비동기 실행(웹훅 응답
	// 지연 금지), 자체 로깅. commits[] 로 판별 불가하면(비어있음) 안전 측으로 실행한다.
	if pushTouchesIssueData(payload) {
		go func() {
			result, err := ResyncProjectIssues(repoOwner, repoName, projectCode)
			if err != nil {
				log.Printf("webhook push: issue resync 실패 code=%s repo=%s: %v", projectCode, repoName, err)
				return
			}
			log.Printf("webhook push: issue resync 완료 code=%s repo=%s result=%v", projectCode, repoName, result)
		}()
	}

	// 모든 docs/* 브랜치 vs 새 main 비교
	alerts := compareAllBranches(repoOwner, repoName, projectCode)
	if alerts == nil {
		alerts = []string{} // null 저장 방지 — "미계산" 과 구분
	}

	// DB 갱신: 차이가 있는 사용자 목록 저장
	db.UpdateProjectField(ctx, projectCode, "branchAlerts", alerts)
	// gitLastCommit은 머지로 해소되었으므로 초기화
	db.UpdateProjectField(ctx, projectCode, "gitLastCommit", nil)

	log.Printf("webhook push: %s 브랜치 비교 완료 — diff 있는 사용자: %v", projectCode, alerts)

	// SSE 브로드캐스트 — 프로젝트 알림 갱신
	BroadcastDocEvent(map[string]interface{}{
		"type":         "project_alert",
		"repo":         repoName,
		"projectCode":  projectCode,
		"ref":          ref,
		"branchAlerts": alerts,
	})
}

// pushTouchesIssueData — commits[].added/modified/removed 중 data/issues/ prefix 경로가
// 하나라도 있으면 true. commits 배열이 비어있어 판별 불가능하면 안전 측(true)으로 판단한다
// (§Phase 2 — 놓쳐서 이슈 캐시가 stale 해지는 것보다 과도 실행이 낫다).
func pushTouchesIssueData(payload map[string]interface{}) bool {
	commits, ok := payload["commits"].([]interface{})
	if !ok || len(commits) == 0 {
		return true
	}
	for _, c := range commits {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		for _, field := range []string{"added", "modified", "removed"} {
			list, _ := cm[field].([]interface{})
			for _, p := range list {
				if ps, ok := p.(string); ok && strings.HasPrefix(ps, "data/issues/") {
					return true
				}
			}
		}
	}
	return false
}

// UpdateBranchAlert 특정 사용자의 브랜치 알림 상태를 갱신하고 갱신된 전체 목록을 반환
func UpdateBranchAlert(owner, repoName, projectCode, userID string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branch := "docs/" + userID
	// ancestor 체크: 사용자 브랜치에 main에 없는 커밋이 있는지 확인
	hasDiff := github.HasUserCommits(owner, repoName, "main", branch)

	// 현재 branchAlerts 배열 조회 후 업데이트
	prj, _ := db.FindProjectByCode(ctx, projectCode)
	if prj == nil {
		return nil
	}
	var currentAlerts []string
	switch v := prj["branchAlerts"].(type) {
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok {
				currentAlerts = append(currentAlerts, s)
			}
		}
	case bson.A:
		for _, a := range v {
			if s, ok := a.(string); ok {
				currentAlerts = append(currentAlerts, s)
			}
		}
	}

	// 사용자 추가/제거
	found := false
	var newAlerts []string
	for _, a := range currentAlerts {
		if a == userID {
			found = true
			if hasDiff {
				newAlerts = append(newAlerts, a)
			}
			// hasDiff가 false면 제거 (추가 안 함)
		} else {
			newAlerts = append(newAlerts, a)
		}
	}
	if !found && hasDiff {
		newAlerts = append(newAlerts, userID)
	}

	// nil 이면 Mongo 에 null 로 저장돼 "미계산" 과 구분이 안 됨 — 빈 배열로 정규화
	if newAlerts == nil {
		newAlerts = []string{}
	}
	db.UpdateProjectField(ctx, projectCode, "branchAlerts", newAlerts)
	log.Printf("branchAlert: %s user=%s diff=%v alerts=%v", projectCode, userID, hasDiff, newAlerts)
	return newAlerts
}

// findProjectByRepo 레포 이름으로 프로젝트 코드 찾기
func findProjectByRepo(ctx context.Context, repoName string) string {
	projects, err := db.FindAllProjects(ctx)
	if err != nil {
		return ""
	}
	for _, prj := range projects {
		docs := db.BsonGetDoc(prj, "documents")
		name := db.BsonGetStr(docs, "repoName")
		if name == repoName {
			jc := db.BsonGetDoc(prj, "jiraCodes")
			main := db.BsonGetDoc(jc, "main")
			return db.BsonGetStr(main, "jiraCode")
		}
	}
	return ""
}

// compareAllBranches 모든 docs/* 브랜치를 main과 비교, 사용자가 실제 커밋한 경우만 반환
// ancestor 체크: 사용자 브랜치가 main의 ancestor이면 변경 없음 (단순 뒤처짐)
func compareAllBranches(owner, repoName, projectCode string) []string {
	branchesRaw, err := github.ListBranches(owner, repoName)
	if err != nil {
		log.Printf("webhook: 브랜치 목록 조회 실패: %v", err)
		return nil
	}

	branches, ok := branchesRaw.([]interface{})
	if !ok {
		return nil
	}

	var diffUsers []string
	for _, b := range branches {
		bm, ok := b.(map[string]interface{})
		if !ok {
			continue
		}
		branchName, _ := bm["name"].(string)
		if !strings.HasPrefix(branchName, "docs/") {
			continue
		}
		userID := strings.TrimPrefix(branchName, "docs/")

		// main...docs/user 비교: 사용자 커밋이 있으면 diff
		if github.HasUserCommits(owner, repoName, "main", branchName) {
			diffUsers = append(diffUsers, userID)
		}
	}
	return diffUsers
}

// extractTreeFiles 트리에서 파일별 SHA 맵 추출
func extractTreeFiles(tree interface{}) map[string]string {
	files := map[string]string{}
	tm, ok := tree.(map[string]interface{})
	if !ok {
		return files
	}
	items, _ := tm["tree"].([]interface{})
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		if t == "tree" {
			continue
		}
		path, _ := m["path"].(string)
		sha, _ := m["sha"].(string)
		files[path] = sha
	}
	return files
}

func filesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func verifySignature(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

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
	case "pull_request_review":
		handlePRReviewEvent(payload)
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

	mergeable, _ := pr["mergeable"].(bool)
	mergeableState, _ := pr["mergeable_state"].(string)

	// SSE 브로드캐스트 — PR 상태 변경 알림
	BroadcastDocEvent(map[string]interface{}{
		"type":           "pr_update",
		"action":         action,
		"repo":           repoName,
		"prNumber":       number,
		"title":          title,
		"state":          state,
		"merged":         merged,
		"mergeable":      mergeable,
		"mergeableState": mergeableState,
	})
}

func handlePRReviewEvent(payload map[string]interface{}) {
	action, _ := payload["action"].(string)
	review, _ := payload["review"].(map[string]interface{})
	pr, _ := payload["pull_request"].(map[string]interface{})
	repo, _ := payload["repository"].(map[string]interface{})
	if review == nil || pr == nil {
		return
	}

	repoName, _ := repo["name"].(string)
	number := pr["number"]
	reviewState, _ := review["state"].(string)
	reviewer, _ := review["user"].(map[string]interface{})
	reviewerLogin, _ := reviewer["login"].(string)
	reviewBody, _ := review["body"].(string)

	prURL, _ := pr["html_url"].(string)
	prTitle, _ := pr["title"].(string)
	prUser, _ := pr["user"].(map[string]interface{})
	prCreator, _ := prUser["login"].(string)

	log.Printf("webhook PR review: action=%s #%v reviewer=%s state=%s repo=%s", action, number, reviewerLogin, reviewState, repoName)

	BroadcastDocEvent(map[string]interface{}{
		"type":     "pr_review",
		"action":   action,
		"repo":     repoName,
		"prNumber": number,
		"reviewer": reviewerLogin,
		"state":    reviewState,
		"body":     reviewBody,
	})

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

	// 모든 docs/* 브랜치 vs 새 main 비교
	alerts := compareAllBranches(repoOwner, repoName, projectCode)

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

// UpdateBranchAlert 특정 사용자의 브랜치 알림 상태를 갱신 (exported — 다른 핸들러에서도 사용)
func UpdateBranchAlert(owner, repoName, projectCode, userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mainTree, err := github.GetTree(owner, repoName, "main")
	if err != nil {
		return
	}
	mainFiles := extractTreeFiles(mainTree)

	branch := "docs/" + userID
	userTree, err := github.GetTree(owner, repoName, branch)
	if err != nil {
		return // 브랜치 없으면 skip
	}
	userFiles := extractTreeFiles(userTree)

	hasDiff := !filesEqual(mainFiles, userFiles)

	// 현재 branchAlerts 배열 조회 후 업데이트
	prj, _ := db.FindProjectByCode(ctx, projectCode)
	if prj == nil {
		return
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

	db.UpdateProjectField(ctx, projectCode, "branchAlerts", newAlerts)
	log.Printf("branchAlert: %s user=%s diff=%v alerts=%v", projectCode, userID, hasDiff, newAlerts)
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

// compareAllBranches 모든 docs/* 브랜치를 main과 비교, 차이 있는 사용자 목록 반환
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

	mainTree, err := github.GetTree(owner, repoName, "main")
	if err != nil {
		return nil
	}
	mainFiles := extractTreeFiles(mainTree)

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

		userTree, err := github.GetTree(owner, repoName, branchName)
		if err != nil {
			continue
		}
		userFiles := extractTreeFiles(userTree)

		if !filesEqual(mainFiles, userFiles) {
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

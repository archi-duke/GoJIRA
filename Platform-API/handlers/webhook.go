package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"platform-api/config"
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

	// main 브랜치 push만 알림
	if strings.HasSuffix(ref, "/main") || strings.HasSuffix(ref, "/master") {
		log.Printf("webhook push: %s → %s", repoName, ref)
		BroadcastDocEvent(map[string]interface{}{
			"type": "push",
			"repo": repoName,
			"ref":  ref,
		})
	}
}

func verifySignature(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

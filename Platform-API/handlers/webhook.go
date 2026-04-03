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

	// SSE 브로드캐스트 — PR 상태 변경 알림
	BroadcastDocEvent(map[string]interface{}{
		"type":     "pr_update",
		"action":   action,
		"repo":     repoName,
		"prNumber": number,
		"title":    title,
		"state":    state,
		"merged":   merged,
	})
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

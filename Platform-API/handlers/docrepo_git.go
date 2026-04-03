package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"platform-api/db"
	"platform-api/github"
)

// POST /DocRepo/{id}/clone — 로컬에 repo clone/pull
func DocRepoCloneHandler(w http.ResponseWriter, r *http.Request) {
	id := getIDParam(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	dir, err := github.EnsureClone(owner, name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "clone 실패: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"path": dir, "status": "ready"})
}

// POST /DocRepo/{id}/git/write — 로컬 파일 쓰기 (여러 파일)
func DocRepoGitWriteHandler(w http.ResponseWriter, r *http.Request) {
	id := getIDParam(r)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	var req struct {
		Branch  string            `json:"branch"`
		Files   map[string]string `json:"files"`   // path → content
		Message string            `json:"message"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// clone/pull 확인
	if _, err := github.EnsureClone(owner, name); err != nil {
		respondError(w, http.StatusInternalServerError, "clone 실패: "+err.Error())
		return
	}

	// 브랜치 생성 또는 checkout
	if req.Branch != "" && req.Branch != "main" {
		github.CheckoutNewBranch(owner, name, req.Branch)
	} else {
		github.Pull(owner, name, "main")
		req.Branch = "main"
	}

	// 파일 쓰기
	for path, content := range req.Files {
		if err := github.WriteLocalFile(owner, name, path, content); err != nil {
			respondError(w, http.StatusInternalServerError, "파일 쓰기 실패: "+path+": "+err.Error())
			return
		}
	}

	// commit + push
	msg := req.Message
	if msg == "" {
		msg = "문서 갱신"
	}
	if err := github.CommitAndPush(owner, name, req.Branch, msg); err != nil {
		respondError(w, http.StatusInternalServerError, "commit/push 실패: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branch": req.Branch,
		"files":  len(req.Files),
		"status": "pushed",
	})
}

// POST /DocRepo/{id}/git/publish — 브랜치 생성 + 파일 쓰기 + commit/push + PR 생성
func DocRepoGitPublishHandler(w http.ResponseWriter, r *http.Request) {
	id := getIDParam(r)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	doc, err := db.FindDocRepoByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	owner, _ := doc["repoOwner"].(string)
	name, _ := doc["repoName"].(string)

	var req struct {
		Branch  string            `json:"branch"`
		Files   map[string]string `json:"files"`
		Message string            `json:"message"`
		PRTitle string            `json:"prTitle"`
		PRBody  string            `json:"prBody"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// auditors 확인
	auditors := toStringSlice(doc["auditors"])
	if len(auditors) == 0 {
		respondError(w, http.StatusBadRequest, "승인자를 먼저 지정하세요")
		return
	}

	// clone/pull
	if _, err := github.EnsureClone(owner, name); err != nil {
		respondError(w, http.StatusInternalServerError, "clone 실패: "+err.Error())
		return
	}

	// 브랜치 생성
	if err := github.CheckoutNewBranch(owner, name, req.Branch); err != nil {
		respondError(w, http.StatusInternalServerError, "브랜치 생성 실패: "+err.Error())
		return
	}

	// 파일 쓰기
	for path, content := range req.Files {
		if err := github.WriteLocalFile(owner, name, path, content); err != nil {
			respondError(w, http.StatusInternalServerError, "파일 쓰기 실패: "+err.Error())
			return
		}
	}

	// commit + push
	if err := github.CommitAndPush(owner, name, req.Branch, req.Message); err != nil {
		respondError(w, http.StatusInternalServerError, "commit/push 실패: "+err.Error())
		return
	}

	// PR 생성 (GitHub API)
	pr, err := github.CreatePR(owner, name, req.PRTitle, req.PRBody, req.Branch, "main")
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}

	// auditors를 reviewer로 할당
	prNum := int(pr["number"].(float64))
	github.RequestReviewers(owner, name, prNum, auditors)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branch": req.Branch,
		"files":  len(req.Files),
		"pr":     pr,
	})
}

func getIDParam(r *http.Request) string {
	return chi.URLParam(r, "id")
}

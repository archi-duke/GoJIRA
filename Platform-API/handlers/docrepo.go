package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// --- DocRepo CRUD ---

// POST /DocRepo — repo 생성
func DocRepoCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	projectID, _ := doc["projectId"].(string)
	repoName, _ := doc["repoName"].(string)
	if projectID == "" || repoName == "" {
		respondError(w, http.StatusBadRequest, "projectId and repoName are required")
		return
	}

	// GitHub repo 생성
	description, _ := doc["description"].(string)
	owner := config.GitHubOrg
	ghRepo, err := github.CreateRepo(repoName, description, true)
	if err != nil {
		respondError(w, http.StatusBadGateway, "GitHub repo 생성 실패: "+err.Error())
		return
	}

	if owner == "" {
		if ownerObj, ok := ghRepo["owner"].(map[string]interface{}); ok {
			owner, _ = ownerObj["login"].(string)
		}
	}

	repoURL, _ := ghRepo["html_url"].(string)
	defaultBranch, _ := ghRepo["default_branch"].(string)
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	doc["repoOwner"] = owner
	doc["repoUrl"] = repoURL
	doc["defaultBranch"] = defaultBranch

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertDocRepo(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

// GET /DocRepo?projectId=...
func DocRepoListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	filter := bson.M{}
	if pid := r.URL.Query().Get("projectId"); pid != "" {
		filter["projectId"] = pid
	}
	repos, err := db.FindAllDocRepos(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, repos)
}

// DELETE /DocRepo/{id}
func DocRepoDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteDocRepo(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "docRepo deleted")
}

// PUT /DocRepo/{id}
func DocRepoUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var updates bson.M
	if err := decodeJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.UpdateDocRepo(ctx, id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "docRepo updated")
}

// --- GitHub 프록시 API ---

func getRepoInfo(ctx context.Context, id string) (string, string, error) {
	doc, err := db.FindDocRepoByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	owner, _ := doc["repoOwner"].(string)
	name, _ := doc["repoName"].(string)
	return owner, name, nil
}

// GET /DocRepo/{id}/tree
func DocRepoTreeHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		ref = "main"
	}
	tree, err := github.GetTree(owner, name, ref)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, tree)
}

// GET /DocRepo/{id}/file?path=...&ref=...
func DocRepoFileHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	filePath := r.URL.Query().Get("path")
	ref := r.URL.Query().Get("ref")
	content, err := github.GetFileContent(owner, name, filePath, ref)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, content)
}

// GET /DocRepo/{id}/commits?branch=...&perPage=...
func DocRepoCommitsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "main"
	}
	perPage := 30
	if pp := r.URL.Query().Get("perPage"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil {
			perPage = v
		}
	}
	commits, err := github.ListCommits(owner, name, branch, perPage)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, commits)
}

// GET /DocRepo/{id}/branches
func DocRepoBranchesHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	branches, err := github.ListBranches(owner, name)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, branches)
}

// --- PR ---

// POST /DocRepo/{id}/pr
func DocRepoPRCreateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	doc, err := db.FindDocRepoByID(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	owner, _ := doc["repoOwner"].(string)
	name, _ := doc["repoName"].(string)

	// auditors 확인
	auditors := toStringSlice(doc["auditors"])
	if len(auditors) == 0 {
		respondError(w, http.StatusBadRequest, "승인자를 먼저 지정하세요")
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Base == "" {
		req.Base = "main"
	}

	pr, err := github.CreatePR(owner, name, req.Title, req.Body, req.Head, req.Base)
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}

	// auditors를 reviewer로 할당
	prNum := int(pr["number"].(float64))
	github.RequestReviewers(owner, name, prNum, auditors)

	respondJSON(w, http.StatusOK, pr)
}

// GET /DocRepo/{id}/prs?state=...
func DocRepoPRListHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}
	prs, err := github.ListPRs(owner, name, state)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, prs)
}

// POST /DocRepo/{id}/pr/{num}/review
func DocRepoPRReviewHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	numStr := chi.URLParam(r, "num")
	num, _ := strconv.Atoi(numStr)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	var req struct {
		Event string `json:"event"` // APPROVE, REQUEST_CHANGES, COMMENT
		Body  string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := github.CreateReview(owner, name, num, req.Event, req.Body); err != nil {
		respondError(w, http.StatusBadGateway, fmt.Sprintf("review failed: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, "review submitted")
}

// POST /DocRepo/{id}/pr/{num}/merge
func DocRepoPRMergeHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	numStr := chi.URLParam(r, "num")
	num, _ := strconv.Atoi(numStr)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	if err := github.MergePR(owner, name, num, "merge"); err != nil {
		respondError(w, http.StatusBadGateway, fmt.Sprintf("merge failed: %v", err))
		return
	}
	respondJSON(w, http.StatusOK, "PR merged")
}

// GET /DocRepo/{id}/pr/{num}/reviews
func DocRepoPRReviewsHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	numStr := chi.URLParam(r, "num")
	num, _ := strconv.Atoi(numStr)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}
	reviews, err := github.ListReviews(owner, name, num)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, reviews)
}

// --- File CRUD ---

// PUT /DocRepo/{id}/file — 파일 생성/수정
func DocRepoFilePutHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"` // base64 encoded
		Message string `json:"message"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha"` // 수정 시 필요
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.Message == "" {
		req.Message = "update " + req.Path
	}

	result, err := github.CreateOrUpdateFile(owner, name, req.Path, req.Content, req.Message, req.Branch, req.SHA)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// POST /DocRepo/{id}/branch — 브랜치 생성
func DocRepoBranchCreateHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, id)
	if err != nil {
		respondError(w, http.StatusNotFound, "docRepo not found")
		return
	}

	var req struct {
		Name string `json:"name"`
		From string `json:"from"` // 소스 브랜치 (기본: main)
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.From == "" {
		req.From = "main"
	}

	sha, err := github.GetBranchRef(owner, name, req.From)
	if err != nil {
		respondError(w, http.StatusBadGateway, "소스 브랜치 조회 실패: "+err.Error())
		return
	}

	if err := github.CreateBranch(owner, name, req.Name, sha); err != nil {
		respondError(w, http.StatusBadGateway, "브랜치 생성 실패: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"branch": req.Name, "sha": sha})
}

// --- DocLink ---

// POST /DocLink
func DocLinkCreateHandler(w http.ResponseWriter, r *http.Request) {
	var doc bson.M
	if err := decodeJSON(r, &doc); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.InsertDocLink(ctx, doc); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, doc)
}

// GET /DocLink?taskId=...&docRepoId=...
func DocLinkListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	filter := bson.M{}
	if tid := r.URL.Query().Get("taskId"); tid != "" {
		filter["taskId"] = tid
	}
	if did := r.URL.Query().Get("docRepoId"); did != "" {
		filter["docRepoId"] = did
	}
	links, err := db.FindDocLinks(ctx, filter)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, links)
}

// DELETE /DocLink/{id}
func DocLinkDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := db.SoftDeleteDocLink(ctx, id, "system"); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "docLink deleted")
}

// --- helpers ---

func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []interface{}:
		s := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok {
				s = append(s, str)
			}
		}
		return s
	case []string:
		return t
	}
	return nil
}

package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"

	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// --- 프로젝트 코드 기반 repo 관리 ---

// POST /DocRepo/{code} — repo 생성 (project.documents에 저장)
func DocRepoCreateHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	prj, err := db.FindProjectByCode(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, "프로젝트를 찾을 수 없습니다")
		return
	}

	// 이미 repo가 있으면 skip
	docs := db.BsonGetDoc(prj, "documents")
	if db.BsonGetStr(docs, "repoName") != "" {
		respondJSON(w, http.StatusOK, docs)
		return
	}

	var req struct {
		RepoName    string `json:"repoName"`
		Description string `json:"description"`
	}
	decodeJSON(r, &req)

	if req.RepoName == "" {
		createdAt := time.Now()
		if t, ok := prj["createdAt"].(time.Time); ok {
			createdAt = t
		}
		req.RepoName = fmt.Sprintf("%s_%s", code, createdAt.Format("20060102150405"))
	}
	if req.Description == "" {
		name, _ := prj["name"].(string)
		req.Description = fmt.Sprintf("%s의 설계 문서", name)
	}

	ghRepo, err := github.CreateRepo(req.RepoName, req.Description, true)
	if err != nil {
		respondError(w, http.StatusBadGateway, "GitHub repo 생성 실패: "+err.Error())
		return
	}

	owner := config.GitHubOrg
	if owner == "" {
		if o, ok := ghRepo["owner"].(map[string]interface{}); ok {
			owner, _ = o["login"].(string)
		}
	}
	repoURL, _ := ghRepo["html_url"].(string)

	db.UpdateProjectDocRepo(ctx, code, req.RepoName, owner, repoURL)

	// Webhook 자동 등록
	if config.WebhookURL != "" {
		github.CreateWebhook(owner, req.RepoName, config.WebhookURL, config.WebhookSecret,
			[]string{"pull_request", "push"})
	}

	log.Printf("DocRepo created: %s/%s for %s", owner, req.RepoName, code)
	respondJSON(w, http.StatusOK, map[string]string{
		"repoName": req.RepoName, "repoOwner": owner, "repoUrl": repoURL,
	})
}

// GET /DocRepo — repo가 있는 프로젝트 목록
func DocRepoListHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	projects, _ := db.FindAllProjects(ctx)
	results := []bson.M{}
	for _, prj := range projects {
		docs := db.BsonGetDoc(prj, "documents")
		repoName := db.BsonGetStr(docs, "repoName")
		if repoName == "" {
			continue
		}
		code := db.BsonGetStr(db.BsonGetDoc(db.BsonGetDoc(prj, "jiraCodes"), "main"), "jiraCode")
		results = append(results, bson.M{
			"projectCode":   code,
			"projectName":   prj["name"],
			"repoName":      repoName,
			"repoOwner":     db.BsonGetStr(docs, "repoOwner"),
			"repoUrl":       db.BsonGetStr(docs, "repoUrl"),
			"defaultBranch": db.BsonGetStr(docs, "defaultBranch"),
		})
	}
	respondJSON(w, http.StatusOK, results)
}

// POST /DocRepo/sync — GitHub vs 프로젝트 비교
func DocRepoSyncHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// GitHub repo 목록
	ghRepos, err := github.ListOrgRepos()
	if err != nil {
		respondError(w, http.StatusBadGateway, "GitHub repo 목록 조회 실패: "+err.Error())
		return
	}
	ghRepoMap := map[string]map[string]interface{}{}
	for _, gr := range ghRepos {
		name, _ := gr["name"].(string)
		ghRepoMap[name] = gr
	}

	// 프로젝트별 repo 정보
	projects, _ := db.FindAllProjects(ctx)

	type repoStatus struct {
		ProjectCode string `json:"projectCode"`
		RepoName    string `json:"repoName"`
		RepoOwner   string `json:"repoOwner"`
		GHExists    bool   `json:"ghExists"`
		Linked      bool   `json:"linked"`
		Status      string `json:"status"` // ok, missing, orphan
	}
	results := []repoStatus{}
	processedNames := map[string]bool{}

	// 프로젝트에 연결된 repo 확인
	for _, prj := range projects {
		docs := db.BsonGetDoc(prj, "documents")
		repoName := db.BsonGetStr(docs, "repoName")
		if repoName == "" {
			continue
		}
		code := db.BsonGetStr(db.BsonGetDoc(db.BsonGetDoc(prj, "jiraCodes"), "main"), "jiraCode")
		owner := db.BsonGetStr(docs, "repoOwner")
		_, ghExists := ghRepoMap[repoName]

		status := "ok"
		if !ghExists {
			status = "missing" // 빨강: GitHub에 없음
		}

		results = append(results, repoStatus{
			ProjectCode: code, RepoName: repoName, RepoOwner: owner,
			GHExists: ghExists, Linked: true, Status: status,
		})
		processedNames[repoName] = true
	}

	// GitHub에 있지만 프로젝트에 없는 repo (플랫폼 패턴만)
	for ghName, ghRepo := range ghRepoMap {
		if processedNames[ghName] {
			continue
		}
		if !strings.Contains(ghName, "_") {
			continue
		}
		owner := ""
		if o, ok := ghRepo["owner"].(map[string]interface{}); ok {
			owner, _ = o["login"].(string)
		}
		results = append(results, repoStatus{
			ProjectCode: "", RepoName: ghName, RepoOwner: owner,
			GHExists: true, Linked: false, Status: "orphan",
		})
	}

	respondJSON(w, http.StatusOK, results)
}

// DELETE /DocRepo/{code}/github — GitHub repo 삭제 + project.documents 비우기
func DocRepoGHDeleteHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, _, err := db.GetProjectDocInfo(ctx, code)
	if err != nil || name == "" {
		// code가 repoName일 수도 있음 (orphan 삭제용)
		owner = config.GitHubOrg
		name = code
	}

	if err := github.DeleteRepo(owner, name); err != nil {
		respondError(w, http.StatusBadGateway, "GitHub repo 삭제 실패: "+err.Error())
		return
	}

	db.ClearProjectDocRepo(ctx, code)
	respondJSON(w, http.StatusOK, "deleted")
}

// DELETE /DocRepo/{code}/purge — project.documents만 비우기 (GitHub 미삭제)
func DocRepoPurgeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	db.ClearProjectDocRepo(ctx, code)
	respondJSON(w, http.StatusOK, "purged")
}

// --- GitHub 프록시 API (projectCode 기반) ---

func getRepoInfo(ctx context.Context, code string) (string, string, error) {
	owner, name, _, err := db.GetProjectDocInfo(ctx, code)
	if err != nil || name == "" {
		return "", "", fmt.Errorf("repo not found for %s", code)
	}
	return owner, name, nil
}

// GET /DocRepo/{code}/tree
func DocRepoTreeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
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

// GET /DocRepo/{code}/file?path=...&ref=...
func DocRepoFileHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	content, err := github.GetFileContent(owner, name, r.URL.Query().Get("path"), r.URL.Query().Get("ref"))
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, content)
}

// GET /DocRepo/{code}/commits
func DocRepoCommitsHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "main"
	}
	perPage := 30
	if pp := r.URL.Query().Get("perPage"); pp != "" {
		if v, e := strconv.Atoi(pp); e == nil {
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

// GET /DocRepo/{code}/branches
func DocRepoBranchesHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
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

// POST /DocRepo/{code}/pr
func DocRepoPRCreateHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// auditors 확인 (프로젝트에서 가져옴)
	prj, _ := db.FindProjectByCode(ctx, code)
	auditors := toStringSlice(prj["auditors"])
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
	decodeJSON(r, &req)
	if req.Base == "" {
		req.Base = "main"
	}

	pr, err := github.CreatePR(owner, name, req.Title, req.Body, req.Head, req.Base)
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}

	prNum := int(pr["number"].(float64))
	github.RequestReviewers(owner, name, prNum, auditors)
	respondJSON(w, http.StatusOK, pr)
}

// GET /DocRepo/{code}/prs
func DocRepoPRListHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
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

// POST /DocRepo/{code}/pr/{num}/review
func DocRepoPRReviewHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	num, _ := strconv.Atoi(chi.URLParam(r, "num"))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	var req struct {
		Event string `json:"event"`
		Body  string `json:"body"`
	}
	decodeJSON(r, &req)
	if err := github.CreateReview(owner, name, num, req.Event, req.Body); err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "review submitted")
}

// POST /DocRepo/{code}/pr/{num}/merge
func DocRepoPRMergeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	num, _ := strconv.Atoi(chi.URLParam(r, "num"))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := github.MergePR(owner, name, num, "merge"); err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "merged")
}

// GET /DocRepo/{code}/pr/{num}/reviews
func DocRepoPRReviewsHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	num, _ := strconv.Atoi(chi.URLParam(r, "num"))
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	reviews, err := github.ListReviews(owner, name, num)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, reviews)
}

// --- Git 로컬 작업 ---

// POST /DocRepo/{code}/git/publish
func DocRepoGitPublishHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	prj, _ := db.FindProjectByCode(ctx, code)
	auditors := toStringSlice(prj["auditors"])
	if len(auditors) == 0 {
		respondError(w, http.StatusBadRequest, "승인자를 먼저 지정하세요")
		return
	}

	var req struct {
		Branch  string            `json:"branch"`
		Files   map[string]string `json:"files"`
		Message string            `json:"message"`
		PRTitle string            `json:"prTitle"`
		PRBody  string            `json:"prBody"`
	}
	decodeJSON(r, &req)

	if _, err := github.EnsureClone(owner, name); err != nil {
		respondError(w, http.StatusInternalServerError, "clone 실패: "+err.Error())
		return
	}
	if err := github.CheckoutNewBranch(owner, name, req.Branch); err != nil {
		respondError(w, http.StatusInternalServerError, "브랜치 생성 실패: "+err.Error())
		return
	}
	for path, content := range req.Files {
		if err := github.WriteLocalFile(owner, name, path, content); err != nil {
			respondError(w, http.StatusInternalServerError, "파일 쓰기 실패: "+err.Error())
			return
		}
	}
	if err := github.CommitAndPush(owner, name, req.Branch, req.Message); err != nil {
		respondError(w, http.StatusInternalServerError, "commit/push 실패: "+err.Error())
		return
	}

	pr, err := github.CreatePR(owner, name, req.PRTitle, req.PRBody, req.Branch, "main")
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}
	prNum := int(pr["number"].(float64))
	github.RequestReviewers(owner, name, prNum, auditors)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branch": req.Branch, "files": len(req.Files), "pr": pr,
	})
}

// --- helpers ---

func toStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case bson.A:
		s := make([]string, 0, len(t))
		for _, item := range t {
			if str, ok := item.(string); ok {
				s = append(s, str)
			}
		}
		return s
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

package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

// GET /DocRepo/git-users — Git 플랫폼 사용자 목록
func GitUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, err := github.ListUsers()
	if err != nil {
		respondError(w, http.StatusBadGateway, "사용자 목록 조회 실패: "+err.Error())
		return
	}
	// login, full_name만 추출
	result := []map[string]string{}
	for _, u := range users {
		login, _ := u["login"].(string)
		fullName, _ := u["full_name"].(string)
		result = append(result, map[string]string{"login": login, "fullName": fullName})
	}
	respondJSON(w, http.StatusOK, result)
}

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

	// 조직 Members 팀에 repo 추가 (멤버가 접근 가능하도록)
	if err := github.AddRepoToOrgTeam(req.RepoName); err != nil {
		log.Printf("조직 팀에 repo 추가 실패: %v", err)
	}

	// Webhook 자동 등록
	if config.WebhookURL != "" {
		github.CreateWebhook(owner, req.RepoName, config.WebhookURL, config.WebhookSecret,
			[]string{"pull_request", "pull_request_review", "push"})
	}

	// Branch protection: 모든 리뷰 완료 전 머지 불가
	auditors := toStringSlice(prj["auditors"])
	reviewCount := len(auditors)
	if reviewCount == 0 {
		reviewCount = 1
	}
	if err := github.SetBranchProtection(owner, req.RepoName, "main", reviewCount); err != nil {
		log.Printf("branch protection 설정 실패 (나중에 재시도 가능): %v", err)
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

	// 정상 연결된 레포에 branch protection 설정 (없으면 적용)
	for _, rs := range results {
		if rs.Status == "ok" && rs.RepoOwner != "" && rs.RepoName != "" {
			go func(owner, repo string) {
				if err := github.SetBranchProtection(owner, repo, "main", 1); err != nil {
					log.Printf("sync: branch protection 설정 실패 %s/%s: %v", owner, repo, err)
				} else {
					log.Printf("sync: branch protection 확인/설정 완료 %s/%s", owner, repo)
				}
			}(rs.RepoOwner, rs.RepoName)
		}
	}

	// 로컬 clone 폴더 중 GitHub에 없는 것 자동 삭제
	activeRepos := map[string]bool{}
	for _, gr := range ghRepos {
		name, _ := gr["name"].(string)
		activeRepos[name] = true
	}
	for _, local := range github.ListLocalRepos() {
		if !activeRepos[local.Repo] {
			log.Printf("sync: removing stale local repo %s/%s", local.Owner, local.Repo)
			github.RemoveLocalRepo(local.Owner, local.Repo)
		}
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

	github.RemoveLocalRepo(owner, name)
	db.ClearProjectDocRepo(ctx, code)
	respondJSON(w, http.StatusOK, "deleted")
}

// DELETE /DocRepo/{code}/purge — project.documents만 비우기 (GitHub 미삭제)
func DocRepoPurgeHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// 로컬 clone 폴더도 삭제
	owner, name, _, err := db.GetProjectDocInfo(ctx, code)
	if err == nil && name != "" {
		github.RemoveLocalRepo(owner, name)
	}

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

// PUT /DocRepo/{code}/file — 파일 수정 (사용자 브랜치에 커밋)
func DocRepoFileUpdateHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userId := r.Header.Get("X-User-Id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	// 사용자 브랜치 (docs/{userId})
	branch := "docs/" + userId
	// 브랜치가 없으면 main에서 생성
	if _, err := github.GetBranchRef(owner, name, branch); err != nil {
		mainSHA, _ := github.GetBranchRef(owner, name, "main")
		if mainSHA != "" {
			github.CreateBranch(owner, name, branch, mainSHA)
		}
	}
	// 현재 파일의 SHA (사용자 브랜치 기준, 없으면 main 기준)
	sha := ""
	existing, err := github.GetFileContent(owner, name, body.Path, branch)
	if err == nil {
		sha, _ = existing["sha"].(string)
	} else {
		existing, err = github.GetFileContent(owner, name, body.Path, "main")
		if err == nil {
			sha, _ = existing["sha"].(string)
		}
	}
	// 사용자 토큰 가져오기
	userToken := getUserToken(r)
	// content를 base64로 인코딩
	encoded := base64.StdEncoding.EncodeToString([]byte(body.Content))
	result, err := github.CreateOrUpdateFileAs(owner, name, body.Path, encoded, body.Message, branch, sha, userToken)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// DELETE /DocRepo/{code}/file — 파일 삭제 (사용자 브랜치에서)
func DocRepoFileDeleteHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userId := r.Header.Get("X-User-Id")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	var body struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	branch := "docs/" + userId
	existing, err := github.GetFileContent(owner, name, body.Path, branch)
	if err != nil {
		respondError(w, http.StatusNotFound, "file not found")
		return
	}
	sha, _ := existing["sha"].(string)
	userToken := getUserToken(r)
	if err := github.DeleteFileAs(owner, name, body.Path, body.Message, branch, sha, userToken); err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
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

// GET /DocRepo/{code}/graph — 전체 브랜치의 커밋 그래프 데이터
// 파라미터: offset=0, limit=30 (페이지네이션)
func DocRepoGraphHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 페이지네이션 파라미터
	offset := 0
	limit := 30
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	branchData, err := github.ListBranches(owner, name)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	branchList, _ := branchData.([]interface{})

	// 각 브랜치의 커밋을 수집하고, 브랜치 이름 매핑
	type graphCommit struct {
		SHA     string   `json:"sha"`
		Message string   `json:"message"`
		Author  string   `json:"author"`
		Date    string   `json:"date"`
		Parents []string `json:"parents"`
		Refs    []string `json:"refs"`
		Lane    int      `json:"lane"`      // 그래프 lane 번호
		Lanes   []int    `json:"lanes"`     // merge 커밋의 모든 parent lane
	}
	commitMap := map[string]*graphCommit{}
	var branchNames []string

	for _, b := range branchList {
		bm, _ := b.(map[string]interface{})
		bName, _ := bm["name"].(string)
		branchNames = append(branchNames, bName)

		// offset + limit만큼 가져오기 (페이지네이션)
		commits, err := github.ListCommits(owner, name, bName, offset+limit)
		if err != nil {
			continue
		}
		commitList, _ := commits.([]interface{})
		for _, c := range commitList {
			cm, _ := c.(map[string]interface{})
			sha, _ := cm["sha"].(string)
			if sha == "" {
				continue
			}
			if _, exists := commitMap[sha]; !exists {
				commit, _ := cm["commit"].(map[string]interface{})
				msg := ""
				author := ""
				date := ""
				if commit != nil {
					msg, _ = commit["message"].(string)
					if a, ok := commit["author"].(map[string]interface{}); ok {
						author, _ = a["name"].(string)
						date, _ = a["date"].(string)
					}
				}
				var parents []string
				if ps, ok := cm["parents"].([]interface{}); ok {
					for _, p := range ps {
						pm, _ := p.(map[string]interface{})
						psha, _ := pm["sha"].(string)
						if psha != "" {
							parents = append(parents, psha)
						}
					}
				}
				commitMap[sha] = &graphCommit{
					SHA: sha, Message: msg, Author: author, Date: date, Parents: parents,
				}
			}
			// 브랜치 HEAD에 ref 추가
			headSHA := ""
			if bc, ok := bm["commit"].(map[string]interface{}); ok {
				headSHA, _ = bc["id"].(string)
			}
			if sha == headSHA {
				commitMap[sha].Refs = append(commitMap[sha].Refs, bName)
			}
		}
	}

	// 위상정렬 (topological sort): 자식이 부모보다 먼저 나오도록
	childCount := map[string]int{}
	for _, c := range commitMap {
		for _, p := range c.Parents {
			childCount[p]++
		}
	}
	// 자식이 없는(=HEAD) 커밋부터 시작
	var queue []string
	for sha := range commitMap {
		if childCount[sha] == 0 {
			queue = append(queue, sha)
		}
	}
	// 같은 깊이에서는 main 브랜치 우선, 그 다음 날짜순
	var allCommits []*graphCommit
	visited := map[string]bool{}
	for len(queue) > 0 {
		// queue를 정렬: main ref 우선, 날짜 최신 우선
		for i := 0; i < len(queue); i++ {
			for j := i + 1; j < len(queue); j++ {
				ci, cj := commitMap[queue[i]], commitMap[queue[j]]
				// main ref가 있으면 우선
				iMain := hasRef(ci.Refs, "main")
				jMain := hasRef(cj.Refs, "main")
				if jMain && !iMain {
					queue[i], queue[j] = queue[j], queue[i]
				} else if iMain == jMain && cj.Date > ci.Date {
					queue[i], queue[j] = queue[j], queue[i]
				}
			}
		}
		sha := queue[0]
		queue = queue[1:]
		if visited[sha] {
			continue
		}
		visited[sha] = true
		c := commitMap[sha]
		allCommits = append(allCommits, c)
		for _, p := range c.Parents {
			childCount[p]--
			if childCount[p] == 0 {
				queue = append(queue, p)
			}
		}
	}

	// Lane assignment: git log --graph 스타일
	// 각 commit에 lane 번호를 할당 (branch 추적용)
	commitLane := map[string]int{}
	laneMap := map[int]string{} // lane -> head commit sha
	maxLane := 0

	for _, c := range allCommits {
		var assignedLane int

		if len(c.Parents) == 0 {
			// Root commit: 새 lane
			assignedLane = maxLane
			maxLane++
		} else {
			// 첫 parent의 lane 사용 (main lane)
			parentSHA := c.Parents[0]
			if lane, ok := commitLane[parentSHA]; ok {
				assignedLane = lane
			} else {
				// parent 정보가 없으면 새 lane
				assignedLane = maxLane
				maxLane++
			}

			// Merge 커밋: 다른 parent들의 lane도 기록
			if len(c.Parents) > 1 {
				for _, parentSHA := range c.Parents[1:] {
					if lane, ok := commitLane[parentSHA]; ok {
						c.Lanes = append(c.Lanes, lane)
					}
				}
			}
		}

		c.Lane = assignedLane
		commitLane[c.SHA] = assignedLane
		laneMap[assignedLane] = c.SHA
	}

	// offset/limit 적용
	var sliced []*graphCommit
	if offset < len(allCommits) {
		end := offset + limit
		if end > len(allCommits) {
			end = len(allCommits)
		}
		sliced = allCommits[offset:end]
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branches": branchNames,
		"commits":  sliced,
		"total":    len(allCommits),
		"hasMore":  offset+limit < len(allCommits),
		"maxLane":  maxLane,
	})
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

	// 리뷰어 계정 존재 여부 확인
	if missing := github.CheckUsersExist(auditors); len(missing) > 0 {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("Git 플랫폼에 존재하지 않는 사용자: %s — 해당 계정을 먼저 생성하세요", strings.Join(missing, ", ")))
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

	userToken := getUserToken(r)
	pr, err := github.CreatePR(owner, name, req.Title, req.Body, req.Head, req.Base, userToken)
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}

	prNum := int(pr["number"].(float64))
	prURL, _ := pr["html_url"].(string)
	prTitle, _ := pr["title"].(string)
	if errs := github.RequestReviewers(owner, name, prNum, auditors); errs != nil {
		log.Printf("리뷰어 등록 실패: %v", errs)
	}

	// 승인자에게 메일 알림
	notifyPRCreated(name, prNum, prTitle, prURL, auditors)

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
	if req.Body == "" {
		req.Body = "LGTM"
	}

	userToken := getUserToken(r)
	reviewID, err := github.CreateReview(owner, name, num, req.Event, req.Body, userToken)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Gitea 1.25 API 버그: APPROVE/REQUEST_CHANGES가 PENDING으로 저장됨 → DB 직접 수정
	if reviewID > 0 {
		if err := github.FixReviewState(reviewID, req.Event); err != nil {
			log.Printf("review state fix 실패 (id=%d): %v", reviewID, err)
		}
	}

	// 승인/반려 후 requested_reviewers에서 해당 사용자 제거
	knoxID := r.Header.Get("X-User-Id")
	if knoxID != "" {
		github.RemoveRequestedReviewer(owner, name, num, knoxID)
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

	// Gitea 리뷰 상태 기반 머지 검증
	reviews, _ := github.ListReviews(owner, name, num)
	reviewList, _ := reviews.([]interface{})

	byUser := map[string]string{}
	for _, rv := range reviewList {
		rm, _ := rv.(map[string]interface{})
		state, _ := rm["state"].(string)
		if state == "PENDING" || state == "REQUEST_REVIEW" {
			continue
		}
		user, _ := rm["user"].(map[string]interface{})
		login, _ := user["login"].(string)
		byUser[login] = state
	}

	// 반려된 리뷰가 있으면 차단
	for user, state := range byUser {
		if state == "CHANGES_REQUESTED" {
			respondError(w, http.StatusConflict, fmt.Sprintf("%s님이 반려했습니다. 수정 후 다시 요청하세요", user))
			return
		}
	}
	// 승인된 리뷰가 하나도 없으면 차단
	hasApproval := false
	for _, state := range byUser {
		if state == "APPROVED" {
			hasApproval = true
			break
		}
	}
	if !hasApproval {
		respondError(w, http.StatusConflict, "최소 1명 이상의 승인이 필요합니다")
		return
	}

	// admin 토큰으로 머지 (branch protection 우회)
	if err := github.MergePR(owner, name, num, "merge"); err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}

	// merge 후 처리: workspace 삭제 + 세션 해제 (브랜치는 유지)
	// PR에서 head branch의 userID 추출
	pr, _ := github.GetPR(owner, name, num)
	if pr != nil {
		if head, ok := pr["head"].(map[string]interface{}); ok {
			if ref, ok := head["ref"].(string); ok && strings.HasPrefix(ref, "docs/") {
				mergedUserID := strings.TrimPrefix(ref, "docs/")
				// workspace만 삭제 (브랜치는 유지)
				github.RemoveWorktreeOnly(owner, name, mergedUserID)
				// 세션 lock 해제
				db.ReleaseEditSession(ctx, code, mergedUserID)
				log.Printf("merge 후 처리: workspace 삭제, 세션 ���제 (user=%s)", mergedUserID)
			}
		}
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

	// 리뷰어 계정 존재 여부 확인
	if missing := github.CheckUsersExist(auditors); len(missing) > 0 {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("Git 플랫폼에 존재하지 않는 사용자: %s — 해당 계정을 먼저 생성하세요", strings.Join(missing, ", ")))
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

	// 브랜치에서 userID 추출
	userID := req.Branch
	if strings.HasPrefix(req.Branch, "docs/") {
		userID = strings.TrimPrefix(req.Branch, "docs/")
	}

	// 사용자별 worktree 생성
	if _, _, err := github.EnsureWorktree(owner, name, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "worktree 생성 실패: "+err.Error())
		return
	}
	if err := github.CheckoutNewBranch(owner, name, req.Branch); err != nil {
		respondError(w, http.StatusInternalServerError, "브랜치 리셋 실패: "+err.Error())
		return
	}
	for path, content := range req.Files {
		if err := github.WriteWorktreeFile(owner, name, userID, path, content); err != nil {
			respondError(w, http.StatusInternalServerError, "파일 쓰기 실패: "+err.Error())
			return
		}
	}
	if err := github.CommitAndPush(owner, name, req.Branch, req.Message); err != nil {
		respondError(w, http.StatusInternalServerError, "commit/push 실패: "+err.Error())
		return
	}

	// PR은 생성하지 않음 — 사용자가 Edit 탭에서 확인/수정 후 명시적으로 배포 요청
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branch": req.Branch, "files": len(req.Files),
		"message": "문서가 작업 브랜치에 생성되었습니다. Edit 탭에서 확인 후 배포(PR) 요청하세요.",
	})
}

// --- 문서 편집 API ---

// POST /DocRepo/{code}/edit/start — 편집 시작 (worktree 생성 + 세션 lock)
func DocRepoEditStartHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	// 만료된 세션 정리
	db.ExpireStaleSessions(ctx, config.EditSessionTTL)

	// 세션 lock 획득 시도
	acquired, existingUser, err := db.AcquireEditSession(ctx, code, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "세션 lock 획득 실패: "+err.Error())
		return
	}
	if !acquired {
		respondJSON(w, http.StatusConflict, map[string]interface{}{
			"locked":       true,
			"lockedBy":     existingUser,
			"readOnly":     true,
			"message":      fmt.Sprintf("다른 사용자(%s)가 편집 중입니다. 읽기 전용으로 열립니다.", existingUser),
		})
		return
	}

	_, branch, err := github.EnsureWorktree(owner, name, userID)
	if err != nil {
		db.ReleaseEditSession(ctx, code, userID)
		respondError(w, http.StatusInternalServerError, "작업 공간 생성 실패: "+err.Error())
		return
	}

	// 자동 rebase (main 동기화)
	rebaseResult := github.RebaseOnMain(owner, name, userID)

	resp := map[string]interface{}{
		"branch":   branch,
		"userId":   userID,
		"locked":   false,
		"readOnly": false,
	}
	if rebaseResult.Conflict {
		resp["conflict"] = true
		resp["diff"] = rebaseResult.Diff
		resp["conflictMessage"] = rebaseResult.Message
	}

	respondJSON(w, http.StatusOK, resp)
}

// POST /DocRepo/{code}/edit/heartbeat — 세션 하트비트
func DocRepoEditHeartbeatHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}
	if err := db.UpdateHeartbeat(ctx, code, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "하트비트 갱신 실패")
		return
	}
	respondJSON(w, http.StatusOK, "ok")
}

// POST /DocRepo/{code}/edit/release — 세션 lock 해제
func DocRepoEditReleaseHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}
	if err := db.ReleaseEditSession(ctx, code, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "세션 해제 실패")
		return
	}
	respondJSON(w, http.StatusOK, "released")
}

// POST /DocRepo/{code}/edit/rebase — main 동기화 (rebase)
func DocRepoEditRebaseHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	result := github.RebaseOnMain(owner, name, userID)
	if result.Conflict {
		respondJSON(w, http.StatusConflict, result)
		return
	}
	if !result.Success {
		respondError(w, http.StatusInternalServerError, result.Message)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /DocRepo/{code}/edit/pending — 워크트리 미커밋 변경 파일 목록
func DocRepoEditPendingHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}
	files := github.PendingFiles(owner, name, userID)
	if files == nil {
		files = []string{}
	}
	respondJSON(w, http.StatusOK, files)
}

// GET /DocRepo/{code}/edit/file?path=...  — worktree에서 파일 읽기
func DocRepoEditFileHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		respondError(w, http.StatusBadRequest, "path 필요")
		return
	}

	content, err := github.ReadWorktreeFile(owner, name, userID, filePath)
	if err != nil {
		respondError(w, http.StatusNotFound, "파일을 찾을 수 없습니다: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"path": filePath, "content": content})
}

// PUT /DocRepo/{code}/edit/file — worktree에 파일 저장 (commit 없이)
func DocRepoEditSaveHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	decodeJSON(r, &req)
	if req.Path == "" {
		respondError(w, http.StatusBadRequest, "path 필요")
		return
	}

	if err := github.WriteWorktreeFile(owner, name, userID, req.Path, req.Content); err != nil {
		respondError(w, http.StatusInternalServerError, "파일 저장 실패: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "saved")
}

// POST /DocRepo/{code}/edit/commit — worktree 변경사항 commit + push
func DocRepoEditCommitHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	decodeJSON(r, &req)
	if req.Message == "" {
		req.Message = "문서 수정"
	}

	branch := fmt.Sprintf("docs/%s", userID)
	if err := github.CommitAndPush(owner, name, branch, req.Message); err != nil {
		respondError(w, http.StatusInternalServerError, "commit/push 실패: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"branch": branch, "message": req.Message})
}

// POST /DocRepo/{code}/edit/pr — 배포(PR) 요청
func DocRepoEditPRHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	prj, _ := db.FindProjectByCode(ctx, code)
	auditors := toStringSlice(prj["auditors"])
	if len(auditors) == 0 {
		respondError(w, http.StatusBadRequest, "승인자를 먼저 지정하세요")
		return
	}
	if missing := github.CheckUsersExist(auditors); len(missing) > 0 {
		respondError(w, http.StatusBadRequest,
			fmt.Sprintf("Git 플랫폼에 존재하지 않는 사용자: %s", strings.Join(missing, ", ")))
		return
	}

	var req struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	decodeJSON(r, &req)

	branch := fmt.Sprintf("docs/%s", userID)

	// 미저장 변경이 있으면 먼저 커밋
	if err := github.CommitAndPush(owner, name, branch, "배포 전 자동 커밋"); err != nil {
		log.Printf("edit/pr auto-commit: %v", err)
	}

	if req.Title == "" {
		req.Title = fmt.Sprintf("[%s] 설계문서 배포 요청", code)
	}
	if req.Body == "" {
		req.Body = fmt.Sprintf("%s의 설계문서 변경 사항을 main에 반영합니다.", code)
	}

	userToken := getUserToken(r)
	pr, err := github.CreatePR(owner, name, req.Title, req.Body, branch, "main", userToken)
	if err != nil {
		respondError(w, http.StatusBadGateway, "PR 생성 실패: "+err.Error())
		return
	}

	prNum := int(pr["number"].(float64))
	prURL, _ := pr["html_url"].(string)
	prTitle, _ := pr["title"].(string)
	github.RequestReviewers(owner, name, prNum, auditors)
	notifyPRCreated(name, prNum, prTitle, prURL, auditors)

	respondJSON(w, http.StatusOK, pr)
}

// GET /DocRepo/{code}/edit/status — 미완료 작업 확인
func DocRepoEditStatusHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	hasPending := github.HasPendingWork(owner, name, userID)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"hasPending": hasPending,
		"branch":     fmt.Sprintf("docs/%s", userID),
	})
}

// DELETE /DocRepo/{code}/edit/discard — 작업 폐기
func DocRepoEditDiscardHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		respondError(w, http.StatusBadRequest, "로그인 정보 없음")
		return
	}

	if err := github.DiscardWorktree(owner, name, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "폐기 실패: "+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, "discarded")
}

func hasRef(refs []string, name string) bool {
	for _, r := range refs {
		if r == name {
			return true
		}
	}
	return false
}

// getUserToken 요청에서 X-User-Id 헤더로 Gitea 토큰 조회
func getUserToken(r *http.Request) string {
	userID := r.Header.Get("X-User-Id")
	if userID == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return db.GetUserGiteaToken(ctx, userID)
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

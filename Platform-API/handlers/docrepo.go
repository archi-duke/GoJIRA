package handlers

import (
	"context"
	"encoding/base64"
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

// GET /DocRepo/check?name=<raw> — 사용자가 입력한 Gitea repo 이름이 사용 가능한지 검사.
//
// 응답: { name, slug, exists, suggestion, valid }
//   - slug:       slugify 결과 (소문자 영문/숫자/하이픈만). 빈 문자열이면 valid=false.
//   - exists:     같은 owner(config.GitHubOrg) 아래 동일 이름 repo 존재 여부.
//   - suggestion: exists=true 일 때 다음 가용한 이름 (slug-2, slug-3, ...). 없으면 빈 문자열.
//   - valid:      slug 가 비지 않았고, 입력 자체가 유효한 형태(영문/숫자/하이픈)일 때 true.
//
// 사용 흐름: ProjectDialog blur 시 호출 → exists=true 면 suggestion 으로 안내.
func DocRepoCheckHandler(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("name")
	slug := slugifyRepoName(raw)
	resp := map[string]interface{}{
		"name":       raw,
		"slug":       slug,
		"exists":     false,
		"suggestion": "",
		"valid":      slug != "",
	}
	if slug == "" {
		respondJSON(w, http.StatusOK, resp)
		return
	}
	exists, err := github.RepoExists(config.GitHubOrg, slug)
	if err != nil {
		respondError(w, http.StatusBadGateway, "repo 중복 검사 실패: "+err.Error())
		return
	}
	resp["exists"] = exists
	if exists {
		if uniq, err := uniqueRepoSlug(slug); err == nil && uniq != slug {
			resp["suggestion"] = uniq
		}
	}
	respondJSON(w, http.StatusOK, resp)
}

// POST /DocRepo/{code} — repo 생성 (project.documents에 저장)
func DocRepoCreateHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// stateless 모드 지원: doc 가 아직 없을 수도 있음 (프로젝트 생성 흐름에서
	// repo 를 먼저 만들고 doc 을 나중에 생성하기 위해).
	//   - doc 있음: 기존 패턴 (skip 또는 UpdateProjectDocRepo 갱신)
	//   - doc 없음: request body 의 projectName 사용, DB 갱신 없이 결과만 반환
	prj, _ := db.FindProjectByCode(ctx, code) // not found 도 OK
	hasDoc := prj != nil

	if hasDoc {
		docs := db.BsonGetDoc(prj, "documents")
		if db.BsonGetStr(docs, "repoName") != "" {
			respondJSON(w, http.StatusOK, docs)
			return
		}
	}

	var req struct {
		ProjectName string `json:"projectName"` // Description 에 사용 (없으면 doc 의 name)
		RepoName    string `json:"repoName"`    // 필수 — 사용자가 입력한 Gitea repo 이름
		Description string `json:"description"`
	}
	decodeJSON(r, &req)

	// projectName: 명시 입력 > doc 의 name > 빈값 (Description fallback 용)
	projectName := req.ProjectName
	if projectName == "" && hasDoc {
		projectName, _ = prj["name"].(string)
	}

	// repoName 은 사용자 입력 필수. slugify 후 빈 문자열이면 거부.
	cleanName := slugifyRepoName(req.RepoName)
	if cleanName == "" {
		respondError(w, http.StatusBadRequest,
			"repoName 필요 — 영문/숫자/하이픈 1자 이상 (입력값: "+req.RepoName+")")
		return
	}
	// 사용자가 입력한 이름이 이미 존재하면 409 — 호출자가 다른 이름 받아 재요청.
	if exists, err := github.RepoExists(config.GitHubOrg, cleanName); err != nil {
		respondError(w, http.StatusBadGateway, "repo 중복 검사 실패: "+err.Error())
		return
	} else if exists {
		respondError(w, http.StatusConflict, fmt.Sprintf("repo '%s' 이미 존재", cleanName))
		return
	}
	req.RepoName = cleanName

	if req.Description == "" && projectName != "" {
		req.Description = fmt.Sprintf("%s의 설계 문서", projectName)
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

	// doc 가 이미 있을 때만 documents 필드 갱신.
	// 없으면 호출자(GoJIRA-API)가 응답을 받아 doc 만들 때 documents 필드를 직접 채운다.
	if hasDoc {
		db.UpdateProjectDocRepo(ctx, code, req.RepoName, owner, repoURL)
	}

	// 조직 Members 팀에 repo 추가 (멤버가 접근 가능하도록)
	if err := github.AddRepoToOrgTeam(req.RepoName); err != nil {
		log.Printf("조직 팀에 repo 추가 실패: %v", err)
	}

	// Webhook 자동 등록
	if config.WebhookURL != "" {
		github.CreateWebhook(owner, req.RepoName, config.WebhookURL, config.WebhookSecret,
			[]string{"pull_request", "pull_request_review", "push"})
	}

	// auto_init + default_branch=main 으로 만들어졌어야 하지만, Gitea 버전/설정에
	// 따라 default_branch 가 master 로 떨어지는 사례가 있어 명시적으로 검증한다.
	// main 이 없으면 빈 clone 상태가 되어 launcher /files 가 "origin/main 없음" 으로
	// 실패한다. 여기서 fail-fast 해 호출자(GoJIRA-API)가 502 + project doc 미생성으로
	// 롤백하도록 한다.
	if _, err := github.GetBranchRef(owner, req.RepoName, "main"); err != nil {
		respondError(w, http.StatusBadGateway,
			"repo 생성 직후 main 브랜치 미존재 — Gitea 의 [repository] DEFAULT_BRANCH=main 또는 token 의 default_branch 적용 여부 확인 필요: "+err.Error())
		return
	}

	// 기본 폴더 구조 및 초기 파일 생성 (브랜치 보호 전에 main에 직접 커밋)
	initialFiles := map[string]string{
		"business/project-description.md": "# Project Description\n\n" +
			"과제의 배경, 목적, 주요 사항을 기록하는 문서입니다.\n\n" +
			"## 과제 배경\n\n(작성 필요)\n\n" +
			"## 과제 목적\n\n(작성 필요)\n\n" +
			"## 주요 사항\n\n(작성 필요)\n\n" +
			"> VOS 자동생성 시 이 문서의 내용을 참조합니다. 반드시 작성해 주세요.\n",
		"business/use-cases.md": "# Use Cases\n\n" +
			"과제의 유즈케이스를 기술하는 문서입니다.\n\n" +
			"## Actors\n\n(작성 필요)\n\n" +
			"## Use Case 목록\n\n(작성 필요)\n\n" +
			"## Use Case 상세\n\n(작성 필요)\n",
		"business/requirements.md": "# Requirements\n\n" +
			"과제의 요구사양을 기술하는 문서입니다.\n\n" +
			"## Functional Requirements (FR)\n\n(작성 필요)\n\n" +
			"## Quality Attributes / Non-Functional Requirements (NFR)\n\n(작성 필요)\n\n" +
			"## Constraints (CNST)\n\n(작성 필요)\n",
		"architecture/architecture.md": "# Architecture Design\n\n" +
			"과제의 주요 설계 사항을 기술하는 문서입니다.\n" +
			"Architecture의 스타일과 주요 디자인 포인트의 설계, 다양한 관점의 통합 디자인 및 설계 평가(SATAM)를 포함합니다.\n\n" +
			"## Architecture Style\n\n(작성 필요)\n\n" +
			"## Key Design Points\n\n(작성 필요)\n\n" +
			"## Integrated Design Views\n\n(작성 필요)\n\n" +
			"## SATAM (Sensitivity, Architecture Tradeoff Analysis)\n\n(작성 필요)\n",
	}
	var initErrs []string
	for path, content := range initialFiles {
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		if _, err := github.CreateFile(owner, req.RepoName, path, encoded, path+" 초기 생성", "main"); err != nil {
			initErrs = append(initErrs, fmt.Sprintf("%s: %v", path, err))
		}
	}
	if len(initErrs) > 0 {
		// 초기 파일이 하나라도 실패 = 사용자가 받게 될 launcher /files 응답에 누락. 즉시 알린다.
		respondError(w, http.StatusBadGateway,
			"초기 파일 생성 실패: "+strings.Join(initErrs, "; "))
		return
	}

	// Branch protection: 모든 리뷰 완료 전 머지 불가
	reviewCount := 1
	if hasDoc {
		auditors := toStringSlice(prj["auditors"])
		if len(auditors) > 0 {
			reviewCount = len(auditors)
		}
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

	// Gitea API (docs/<user> 브랜치 포함). Gitea 미가동 시 로컬 bare repo fallback.
	tree, err := github.GetTree(owner, name, ref)
	if err != nil {
		// Gitea 미가동 시 로컬 bare repo 에서 git ls-tree 로 대체
		entries, lsErr := buildBareTree(owner, name, ref)
		if lsErr != nil {
			// 둘 다 실패 — ref 가 아직 없는 케이스 (사용자가 /sync 한 적 없어 docs/{user} 미존재)
			// 는 정상 흐름이므로 빈 트리 + 200 으로 응답해 클라이언트 console noise 제거.
			// 진짜 backend 장애와 구분되지 않지만 클라이언트(branchDiff 체크 등)는 빈 트리로 graceful 처리 가능.
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"tree":        []interface{}{},
				"total_count": 0,
				"source":      "empty",
				"notFound":    true,
				"ref":         ref,
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"tree":        entries,
			"total_count": len(entries),
			"source":      "local-bare",
		})
		return
	}
	respondJSON(w, http.StatusOK, tree)
}

// buildBareTree — 로컬 bare repo 에서 git ls-tree 로 트리를 생성 (Gitea 미가동 시 fallback).
func buildBareTree(owner, name, ref string) ([]map[string]interface{}, error) {
	bareDir := github.GetLocalRepoDir(owner, name)
	out, err := github.GitCmd(bareDir, "ls-tree", "-r", "--long", ref)
	if err != nil {
		return nil, err
	}
	var entries []map[string]interface{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		// format: <mode> <type> <sha> <size>\t<path>
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := strings.Fields(parts[0])
		if len(meta) < 4 {
			continue
		}
		size := int64(0)
		fmt.Sscanf(meta[3], "%d", &size)
		entries = append(entries, map[string]interface{}{
			"path": parts[1],
			"type": meta[1],
			"mode": meta[0],
			"sha":  meta[2],
			"size": size,
		})
	}
	return entries, nil
}

// GET /DocRepo/{code}/file?path=...&ref=...
// docs/{user} 브랜치 포함 모든 ref 를 Gitea API 로 조회.
func DocRepoFileHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
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
	// 자식이 없는(=HEAD) 커밋부터 시작 — commitMap에 있는 것만
	var queue []string
	for sha := range commitMap {
		if childCount[sha] == 0 {
			queue = append(queue, sha)
		}
	}
	// childCount에만 있고 commitMap에 없는 parent는 제거
	for i := len(queue) - 1; i >= 0; i-- {
		if commitMap[queue[i]] == nil {
			queue = append(queue[:i], queue[i+1:]...)
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
				if ci == nil || cj == nil { continue }
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
		if c == nil {
			continue
		}
		allCommits = append(allCommits, c)
		for _, p := range c.Parents {
			childCount[p]--
			if childCount[p] == 0 && commitMap[p] != nil {
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
		// SkipAutoCommit — 무시됨(deprecated). 로컬 에이전트 전환 이후 서버는
		// 절대 commit/push 하지 않는다. 런처가 docs/{user} 를 로컬에서 push 한다.
		SkipAutoCommit bool `json:"skipAutoCommit"`
	}
	decodeJSON(r, &req)
	if req.Base == "" {
		req.Base = "main"
	}

	// PR 생성 전 diff 확인 — head가 base의 ancestor이면 변경사항 없음
	compare, _ := github.CompareBranches(owner, name, req.Base, req.Head)
	if compare != nil {
		if files, ok := compare["files"].([]interface{}); ok && len(files) == 0 {
			respondError(w, http.StatusConflict, "변경사항이 없습니다. 파일을 수정한 후 발행 신청해주세요.")
			return
		}
		if totalCommits, ok := compare["total_commits"].(float64); ok && totalCommits == 0 {
			respondError(w, http.StatusConflict, "변경사항이 없습니다. 파일을 수정한 후 발행 신청해주세요.")
			return
		}
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

	// 프로젝트 알림: PR 생성 (리뷰어 목록 저장)
	db.UpdateProjectField(ctx, code, "gitOpenPRReviewers", auditors)

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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
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

	// 선행 PR 확인 — 현재 PR보다 먼저 생성된 open PR이 있으면 안내
	openPRs, _ := github.ListPRs(owner, name, "open")
	if prList, ok := openPRs.([]interface{}); ok {
		for _, p := range prList {
			pm, _ := p.(map[string]interface{})
			prNum, _ := pm["number"].(float64)
			if int(prNum) < num {
				prTitle, _ := pm["title"].(string)
				respondError(w, http.StatusConflict,
					fmt.Sprintf("PR #%d (%s)이 먼저 대기 중입니다. 선행 PR을 먼저 처리해주세요.", int(prNum), prTitle))
				return
			}
		}
	}

	// 현재 PR의 mergeable 상태 확인
	prInfo, _ := github.GetPR(owner, name, num)
	mergeable := true
	if prInfo != nil {
		if m, ok := prInfo["mergeable"].(bool); ok {
			mergeable = m
		}
	}

	// 충돌 시 자동 동기화 시도
	if !mergeable {
		log.Printf("merge: PR #%d 충돌 감지, 자동 동기화 시도", num)
		// rebase 시도 → 실패하면 merge 방식 시도
		if err := github.UpdatePRBranch(owner, name, num, "rebase"); err != nil {
			if err2 := github.UpdatePRBranch(owner, name, num, "merge"); err2 != nil {
				respondError(w, http.StatusConflict,
					"충돌을 자동으로 해결할 수 없습니다. PR을 닫고 Git Pull로 동기화한 후 다시 작업해주세요.")
				return
			}
		}
		log.Printf("merge: PR #%d 자동 동기화 성공", num)
	}

	// force_merge로 branch protection 우회하여 머지
	if err := github.MergePR(owner, name, num, "merge"); err != nil {
		if strings.Contains(err.Error(), "405") {
			respondError(w, http.StatusConflict, "머지할 수 없습니다. 변경사항이 없거나 충돌이 있습니다. PR을 확인해주세요.")
			return
		}
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}

	// 프로젝트 알림: PR 머지 완료 → 플래그 해제
	db.UpdateProjectField(ctx, code, "gitOpenPRReviewers", []string{})
	db.UpdateProjectField(ctx, code, "gitLastCommit", nil)

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

// uniqueRepoSlug 는 base slug 가 이미 존재하면 "-N" 접미사를 붙여 고유한 이름을 찾는다.
//
// 시도 순서: base, base-2, base-3, ... 최대 99 까지.
// 검사 owner 는 config.GitHubOrg (있으면) 우선. RepoExists 가 일시 오류를 반환하면
// 그 자리에서 중단해 호출자에게 에러 노출 (잘못된 이름으로 진행 방지).
func uniqueRepoSlug(base string) (string, error) {
	if base == "" {
		return "", fmt.Errorf("base slug 가 비어 있음")
	}
	owner := config.GitHubOrg
	candidate := base
	for i := 0; i < 100; i++ {
		exists, err := github.RepoExists(owner, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i+2)
	}
	return "", fmt.Errorf("고유 이름 탐색 실패 (base=%q, 100 회 시도 후 포기)", base)
}

// slugifyRepoName: GitHub repo 이름으로 변환.
//
// 입력은 LLM 의 영문 slug 응답 또는 한국어 원문 모두 가능.
//   1) 공백·하이픈 기준으로 단어 분리
//   2) 각 단어에서 영문 소문자/숫자만 보존
//   3) 빈 단어 제거 후 '-' 로 join
//
// 길이 제한 없음 (Gitea/GitHub 의 100자 한도까지 허용).
// 한국어만 있어 결과가 빈 문자열이면 호출자가 에러 처리.
//
// 예: "Test Prj" → "test-prj"
//
//   "GoJIRA Sample 2" → "gojira-sample-2"
//   "security-enhancement" → "security-enhancement"
//   "클라우드마이그레이션" → ""
func slugifyRepoName(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r == '-' || r == ' ' || r == '_'
	})
	var result []string
	for _, word := range words {
		var clean strings.Builder
		for _, r := range word {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				clean.WriteRune(r)
			}
		}
		if cleaned := clean.String(); cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return strings.Join(result, "-")
}

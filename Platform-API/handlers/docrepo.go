package handlers

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
	"platform-api/llm"
)

// repo별 init-files 동시 실행 방지 뮤텍스
var initFilesMu sync.Map // key: "owner/repo" → *sync.Mutex

func getRepoMutex(owner, name string) *sync.Mutex {
	key := owner + "/" + name
	v, _ := initFilesMu.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

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
		ProjectName string `json:"projectName"` // doc 없을 때 LLM 호출 / Description 에 사용
		RepoName    string `json:"repoName"`
		Description string `json:"description"`
	}
	decodeJSON(r, &req)

	// projectName 결정: 명시 입력 > doc 의 name > 빈값
	projectName := req.ProjectName
	if projectName == "" && hasDoc {
		projectName, _ = prj["name"].(string)
	}

	if req.RepoName == "" {
		if projectName == "" {
			respondError(w, http.StatusBadRequest, "projectName 또는 repoName 필요")
			return
		}
		slug, err := translateRepoName(ctx, projectName)
		if err != nil {
			respondError(w, http.StatusBadGateway, "repo 이름 LLM 번역 실패: "+err.Error())
			return
		}
		// 중복 회피: 같은 slug 의 repo 가 이미 있으면 -2, -3, ... 접미사로 회피.
		// owner 결정은 CreateRepo 호출 후에야 가능하지만, 검사용으로 GitHubOrg 우선 사용.
		uniqueSlug, suffixErr := uniqueRepoSlug(slug)
		if suffixErr != nil {
			respondError(w, http.StatusBadGateway, "고유 repo 이름 생성 실패: "+suffixErr.Error())
			return
		}
		req.RepoName = uniqueSlug
	}
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
	for path, content := range initialFiles {
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		if _, err := github.CreateFile(owner, req.RepoName, path, encoded, path+" 초기 생성", "main"); err != nil {
			log.Printf("초기 파일 생성 실패 (%s): %v", path, err)
		}
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

// POST /DocRepo/{code}/init-files — 초기 폴더/파일 자동 생성 (main 브랜치, 없는 것만)
func DocRepoInitFilesHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// 동일 repo에 대한 동시 init-files 호출 직렬화
	mu := getRepoMutex(owner, name)
	mu.Lock()
	defer mu.Unlock()

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

	var created []string
	needProtectionToggle := false
	for path := range initialFiles {
		if _, err := github.GetFileContent(owner, name, path, "main"); err != nil {
			needProtectionToggle = true
			break
		}
	}

	if needProtectionToggle {
		// 브랜치 보호 임시 해제
		github.DeleteBranchProtection(owner, name, "main")

		for path, content := range initialFiles {
			if _, err := github.GetFileContent(owner, name, path, "main"); err == nil {
				continue // 이미 존재하면 스킵
			}
			encoded := base64.StdEncoding.EncodeToString([]byte(content))
			if _, err := github.CreateFile(owner, name, path, encoded, fmt.Sprintf("%s 초기 생성", path), "main"); err != nil {
				log.Printf("init-files: %s 생성 실패: %v", path, err)
				continue
			}
			created = append(created, path)
		}

		// 브랜치 보호 복원
		prj, _ := db.FindProjectByCode(ctx, code)
		auditors := toStringSlice(prj["auditors"])
		reviewCount := len(auditors)
		if reviewCount == 0 {
			reviewCount = 1
		}
		github.SetBranchProtection(owner, name, "main", reviewCount)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"created": created,
		"count":   len(created),
	})
}

// POST /DocRepo/{code}/ensure-branch — 사용자 브랜치 없으면 main에서 자동 생성
func DocRepoEnsureBranchHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userId := r.Header.Get("X-User-Id")
	if userId == "" {
		respondError(w, http.StatusBadRequest, "X-User-Id 필요")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	branch := "docs/" + userId

	// 로컬 worktree 가 이미 존재하면 Gitea 없이도 성공
	if _, _, wtErr := github.EnsureWorktree(owner, name, userId); wtErr == nil {
		go UpdateBranchAlert(owner, name, code, userId)
		respondJSON(w, http.StatusOK, map[string]interface{}{"branch": branch, "created": false})
		return
	}

	// Gitea 에서 브랜치 목록 확인
	branchList, _ := github.ListBranches(owner, name)
	branches, _ := branchList.([]interface{})
	branchExists := false
	mainSHA := ""
	for _, b := range branches {
		bm, _ := b.(map[string]interface{})
		bName, _ := bm["name"].(string)
		if bName == branch {
			branchExists = true
		}
		if bName == "main" {
			if commit, ok := bm["commit"].(map[string]interface{}); ok {
				mainSHA, _ = commit["id"].(string)
			}
		}
	}
	if branchExists {
		go UpdateBranchAlert(owner, name, code, userId)
		respondJSON(w, http.StatusOK, map[string]interface{}{"branch": branch, "created": false})
		return
	}
	if mainSHA == "" {
		respondError(w, http.StatusBadGateway, "main 브랜치 조회 실패")
		return
	}
	if err := github.CreateBranch(owner, name, branch, mainSHA); err != nil {
		respondError(w, http.StatusBadGateway, "브랜치 생성 실패: "+err.Error())
		return
	}
	log.Printf("ensure-branch: %s/%s branch=%s 생성 (from main=%s)", owner, name, branch, mainSHA[:8])
	go UpdateBranchAlert(owner, name, code, userId)
	respondJSON(w, http.StatusOK, map[string]interface{}{"branch": branch, "created": true})
}

// POST /DocRepo/{code}/sync-branch — 사용자 브랜치를 main에 동기화 (main의 내용을 merge)
func DocRepoSyncBranchHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userId := r.Header.Get("X-User-Id")
	if userId == "" {
		respondError(w, http.StatusBadRequest, "X-User-Id 필요")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	branch := "docs/" + userId

	// 로컬 worktree 가 있으면 로컬에서 직접 main merge (Gitea 불필요)
	wtDir, _, wtErr := github.EnsureWorktree(owner, name, userId)
	if wtErr == nil {
		bareDir := github.GetLocalRepoDir(owner, name)
		github.GitCmd(bareDir, "fetch", "origin")
		github.GitCmd(wtDir, "checkout", "--", ".")
		if out, mergeErr := github.GitCmd(wtDir, "merge", "main", "--no-edit"); mergeErr != nil {
			// merge 실패 시 hard reset
			github.GitCmd(wtDir, "merge", "--abort")
			github.GitCmd(wtDir, "reset", "--hard", "main")
			log.Printf("sync-branch: merge 실패, hard reset to main — %s: %s", wtDir, out)
		} else {
			log.Printf("sync-branch: worktree merged main — %s", wtDir)
		}
		go UpdateBranchAlert(owner, name, code, userId)
		respondJSON(w, http.StatusOK, map[string]interface{}{"branch": branch, "synced": true, "source": "local"})
		return
	}

	// fallback: Gitea API 경유
	branchList, _ := github.ListBranches(owner, name)
	branches, _ := branchList.([]interface{})
	branchExists := false
	mainSHA := ""
	for _, b := range branches {
		bm, _ := b.(map[string]interface{})
		bName, _ := bm["name"].(string)
		if bName == branch {
			branchExists = true
		}
		if bName == "main" {
			if commit, ok := bm["commit"].(map[string]interface{}); ok {
				mainSHA, _ = commit["id"].(string)
			}
		}
	}

	if mainSHA == "" {
		respondError(w, http.StatusBadGateway, "main 브랜치 조회 실패")
		return
	}

	if branchExists {
		github.DeleteBranch(owner, name, branch)
		time.Sleep(500 * time.Millisecond)
	}

	if err := github.CreateBranch(owner, name, branch, mainSHA); err != nil {
		respondError(w, http.StatusBadGateway, "브랜치 동기화 실패: "+err.Error())
		return
	}

	// 알림 갱신
	go UpdateBranchAlert(owner, name, code, userId)

	log.Printf("sync-branch: %s/%s branch=%s → main(%s)", owner, name, branch, mainSHA[:8])
	respondJSON(w, http.StatusOK, map[string]interface{}{"synced": true, "branch": branch})
}

// POST /DocRepo/{code}/conflict-resolve — 변경 보관 → sync → 재반영
// 사용자 브랜치의 변경 파일을 보관하고, main으로 동기화한 뒤, 변경 내용을 다시 적용
func DocRepoConflictResolveHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userId := r.Header.Get("X-User-Id")
	if userId == "" {
		respondError(w, http.StatusBadRequest, "X-User-Id 필요")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	owner, name, err := getRepoInfo(ctx, code)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	branch := "docs/" + userId

	// 1. main과 user branch 트리 비교 → 다른 파일 찾기
	mainTree, err := github.GetTree(owner, name, "main")
	if err != nil {
		respondError(w, http.StatusBadGateway, "main 트리 조회 실패")
		return
	}
	userTree, err := github.GetTree(owner, name, branch)
	if err != nil {
		respondError(w, http.StatusBadGateway, "사용자 브랜치 트리 조회 실패")
		return
	}

	mainFiles := extractTreeFiles(mainTree)
	userFiles := extractTreeFiles(userTree)

	// user에만 있거나 SHA가 다른 파일의 내용을 보관
	type savedFile struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	var saved []savedFile
	for path, sha := range userFiles {
		if mainFiles[path] != sha {
			// 사용자 파일 내용 다운로드
			fileData, err := github.GetFileContent(owner, name, path, branch)
			if err != nil {
				continue
			}
			content, _ := fileData["content"].(string)
			if content != "" {
				saved = append(saved, savedFile{Path: path, Content: content})
			}
		}
	}

	if len(saved) == 0 {
		respondJSON(w, http.StatusOK, map[string]interface{}{"resolved": false, "message": "변경 파일 없음"})
		return
	}

	log.Printf("conflict-resolve: %s/%s branch=%s 보관 파일 %d개", owner, name, branch, len(saved))

	// 2. 브랜치 삭제 → main에서 재생성 (sync)
	github.DeleteBranch(owner, name, branch)
	time.Sleep(500 * time.Millisecond)

	branchList, _ := github.ListBranches(owner, name)
	branches, _ := branchList.([]interface{})
	mainSHA := ""
	for _, b := range branches {
		bm, _ := b.(map[string]interface{})
		if bName, _ := bm["name"].(string); bName == "main" {
			if commit, ok := bm["commit"].(map[string]interface{}); ok {
				mainSHA, _ = commit["id"].(string)
			}
		}
	}
	if mainSHA == "" {
		respondError(w, http.StatusBadGateway, "main SHA 조회 실패")
		return
	}
	if err := github.CreateBranch(owner, name, branch, mainSHA); err != nil {
		respondError(w, http.StatusBadGateway, "브랜치 재생성 실패: "+err.Error())
		return
	}

	// 3. 보관된 변경 파일을 user branch에 다시 적용
	var applied []string
	for _, f := range saved {
		// 브랜치 보호 우회를 위해 직접 API 호출
		existingFile, _ := github.GetFileContent(owner, name, f.Path, branch)
		if existingFile != nil {
			// 파일이 이미 존재 (main에서 온 것) → SHA로 업데이트
			sha, _ := existingFile["sha"].(string)
			_, err = github.CreateOrUpdateFile(owner, name, f.Path, f.Content,
				fmt.Sprintf("%s 충돌 해결 재반영", f.Path), branch, sha)
		} else {
			// 새 파일
			_, err = github.CreateFileOnBranch(owner, name, f.Path, f.Content,
				fmt.Sprintf("%s 충돌 해결 재반영", f.Path), branch, "")
		}
		if err != nil {
			log.Printf("conflict-resolve: %s 재반영 실패: %v", f.Path, err)
			continue
		}
		applied = append(applied, f.Path)
	}

	go UpdateBranchAlert(owner, name, code, userId)

	log.Printf("conflict-resolve: %s 완료 — %d/%d 파일 재반영", branch, len(applied), len(saved))
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"resolved": true,
		"saved":    len(saved),
		"applied":  applied,
	})
}

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

	// 사용자 브랜치 → 로컬 worktree 에서 트리 생성
	if strings.HasPrefix(ref, "docs/") {
		userID := strings.TrimPrefix(ref, "docs/")
		wtDir, _, wtErr := github.EnsureWorktree(owner, name, userID)
		if wtErr == nil {
			entries := buildLocalTree(wtDir, "")
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"tree":        entries,
				"total_count": len(entries),
				"source":      "worktree",
			})
			return
		}
	}

	// fallback: Gitea API
	tree, err := github.GetTree(owner, name, ref)
	if err != nil {
		// Gitea 미가동 시 로컬 bare repo 에서 git ls-tree 로 대체
		entries, lsErr := buildBareTree(owner, name, ref)
		if lsErr != nil {
			respondError(w, http.StatusBadGateway, err.Error())
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

// buildLocalTree — worktree 디렉토리를 재귀 탐색해 Gitea tree 호환 응답 생성.
// sha 는 git blob hash (git hash-object 호환) 로 계산해 main 과 비교 가능하게 한다.
func buildLocalTree(baseDir, prefix string) []map[string]interface{} {
	var entries []map[string]interface{}
	dir := filepath.Join(baseDir, prefix)
	items, err := os.ReadDir(dir)
	if err != nil {
		return entries
	}
	for _, item := range items {
		if item.Name() == ".git" {
			continue
		}
		rel := item.Name()
		if prefix != "" {
			rel = prefix + "/" + item.Name()
		}
		if item.IsDir() {
			entries = append(entries, map[string]interface{}{
				"path": rel,
				"type": "tree",
				"mode": "040000",
				"size": 0,
			})
			entries = append(entries, buildLocalTree(baseDir, rel)...)
		} else {
			info, _ := item.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			fullPath := filepath.Join(baseDir, rel)
			sha := gitBlobHash(fullPath)
			entries = append(entries, map[string]interface{}{
				"path": rel,
				"type": "blob",
				"mode": "100644",
				"size": size,
				"sha":  sha,
			})
		}
	}
	return entries
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

// gitBlobHash — git hash-object 호환 SHA-1 계산.
func gitBlobHash(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	header := fmt.Sprintf("blob %d\x00", len(data))
	h := sha1.New()
	h.Write([]byte(header))
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// GET /DocRepo/{code}/file?path=...&ref=...
// 사용자 브랜치(docs/{user}) 요청 시 로컬 worktree 우선, fallback 으로 Gitea API.
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

	// 사용자 브랜치 요청 시 로컬 worktree 우선 조회
	// worktree 파일은 항상 UTF-8 plain text 로 저장되어 있다 (WriteWorktreeFile 보장).
	// API 호환을 위해 base64 인코딩해서 반환.
	if strings.HasPrefix(ref, "docs/") {
		userID := strings.TrimPrefix(ref, "docs/")
		if localContent, err := github.ReadWorktreeFile(owner, name, userID, filePath); err == nil {
			encoded := base64.StdEncoding.EncodeToString([]byte(localContent))
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"path":    filePath,
				"content": encoded,
				"source":  "worktree",
			})
			return
		}
	}

	// fallback: Gitea API
	content, err := github.GetFileContent(owner, name, filePath, ref)
	if err != nil {
		respondError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, content)
}

// PUT /DocRepo/{code}/file — 파일 수정 (로컬 worktree 에 write, push 하지 않음)
//
// 변경: Gitea API 경유 commit+push 를 제거하고 로컬 worktree 에 직접 write.
// push 는 PR 생성(POST /DocRepo/{code}/pr) 시 일괄 수행.
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

	// worktree 확보 (없으면 자동 생성)
	if _, _, err := github.EnsureWorktree(owner, name, userId); err != nil {
		respondError(w, http.StatusInternalServerError, "worktree 생성 실패: "+err.Error())
		return
	}

	// 로컬 파일 write (commit/push 없음)
	if err := github.WriteWorktreeFile(owner, name, userId, body.Path, body.Content); err != nil {
		respondError(w, http.StatusInternalServerError, "파일 저장 실패: "+err.Error())
		return
	}

	// SSE 브로드캐스트 — UI DocEditor 자동 갱신
	branch := "docs/" + userId
	BroadcastDocEvent(map[string]interface{}{
		"type":        "user_doc_update",
		"projectCode": code,
		"user":        userId,
		"path":        body.Path,
		"branch":      branch,
	})
	respondJSON(w, http.StatusOK, map[string]string{"path": body.Path, "status": "saved"})
}

// DELETE /DocRepo/{code}/file — 파일 삭제 (로컬 worktree 에서, push 하지 않음)
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

	// 로컬 worktree 에서 파일 삭제
	branch := "docs/" + userId
	wtDir, _, err := github.EnsureWorktree(owner, name, userId)
	if err == nil {
		fullPath := filepath.Join(wtDir, body.Path)
		if removeErr := os.Remove(fullPath); removeErr != nil && !os.IsNotExist(removeErr) {
			respondError(w, http.StatusInternalServerError, "파일 삭제 실패: "+removeErr.Error())
			return
		}
	} else {
		// worktree 없으면 기존 Gitea API fallback
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
	}

	BroadcastDocEvent(map[string]interface{}{
		"type":        "user_doc_update",
		"projectCode": code,
		"user":        userId,
		"path":        body.Path,
		"branch":      branch,
		"deleted":     true,
	})
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
	}
	decodeJSON(r, &req)
	if req.Base == "" {
		req.Base = "main"
	}

	// 발행 전 로컬 worktree 변경사항을 commit + push (로컬 Worktree 아키텍처)
	if strings.HasPrefix(req.Head, "docs/") {
		userID := strings.TrimPrefix(req.Head, "docs/")
		commitMsg := req.Title
		if commitMsg == "" {
			commitMsg = fmt.Sprintf("[%s] 설계문서 발행", code)
		}
		if err := github.CommitAndPush(owner, name, req.Head, commitMsg); err != nil {
			log.Printf("PR 선행 commit+push 경고: %v", err)
		}
		_ = userID
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

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"branch":   branch,
		"userId":   userID,
		"locked":   false,
		"readOnly": false,
	})
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

	// PR 생성 전 diff 확인
	compare, _ := github.CompareBranches(owner, name, "main", branch)
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

func DocRepoOpenFolderHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
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
	wtDir := filepath.Join(config.RepoBaseDir, owner, name+".worktrees", userID)
	if _, err := os.Stat(wtDir); os.IsNotExist(err) {
		respondError(w, http.StatusNotFound, "작업공간 폴더가 없습니다. Pull을 먼저 실행하세요.")
		return
	}
	var cmd string
	switch {
	case fileExists("/usr/bin/open"):
		cmd = "open"
	case fileExists("/usr/bin/explorer.exe") || fileExists("/mnt/c/Windows/explorer.exe"):
		cmd = "explorer.exe"
	default:
		cmd = "xdg-open"
	}
	go func() {
		exec.Command(cmd, wtDir).Run()
	}()
	respondJSON(w, http.StatusOK, map[string]string{"path": wtDir, "cmd": cmd})
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
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

// translateRepoName: 과제명을 LLM 으로 영문 slug 로 번역.
// llm.json (config.LLM) 기반 호출 — 모델/엔드포인트는 설정에서 결정.
// 호출 자체에 특화된 옵션(max_tokens=20, temperature=0)은 Options 로 override.
//
// 실패 시 error 반환. 호출자는 폴백하지 않고 작업 실패로 처리해야 한다.
// LLM 응답은 한 번 더 slugifyRepoName 으로 sanitize 해 영문/숫자/하이픈만 보존.
func translateRepoName(ctx context.Context, name string) (string, error) {
	if config.LLM == nil {
		return "", fmt.Errorf("LLM 설정 미로드")
	}
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("프로젝트명이 비어 있어 LLM 호출 불가")
	}

systemPrompt := config.LLM.SystemPrompt
if systemPrompt == "" {
	systemPrompt = "Convert Korean project titles to English GitHub repo slugs. Rules: lowercase, alphanumeric/hyphens only, preserve meaning faithfully. Examples: 'SW 설계 개발'→'sw-design-development', '보안강화프로젝트'→'security-enhancement-project', '클라우드 마이그레이션'→'cloud-migration'. Output ONLY JSON: {\"result\":\"slug\",\"status\":\"ok\"}."
}
messages := []llm.Message{
	{Role: "system", Content: systemPrompt},
	{Role: "user", Content: name},
}
temp := 0.0
maxTokens := config.LLM.MaxTokens
if maxTokens <= 0 {
	maxTokens = 500
}
log.Printf("translateRepoName: LLM config maxTokens=%d, using %d", config.LLM.MaxTokens, maxTokens)
opts := &llm.Options{
	MaxTokens:   maxTokens,
	Temperature: &temp,
	Timeout:     10 * time.Second,
	Ctx:         ctx,
}

text, err := llm.Call(config.LLM, messages, opts)
log.Printf("translateRepoName: LLM response raw=[%s], err=%v", text, err)
if err != nil {
	return "", fmt.Errorf("LLM 호출 실패: %w", err)
}

// JSON 응답 파싱: {"result":"sw-design-development", "status":"ok"}
var jsonResp struct {
	Result string `json:"result"`
	Status string `json:"status"`
}
if err := json.Unmarshal([]byte(text), &jsonResp); err == nil && jsonResp.Result != "" {
	slug := slugifyRepoName(jsonResp.Result)
	log.Printf("translateRepoName: '%s' -> '%s' (via JSON)", name, slug)
	return slug, nil
}

// Fallback: 일반 텍스트 응답
slug := slugifyRepoName(text)
log.Printf("translateRepoName: '%s' -> '%s' (fallback)", name, slug)
if slug == "" {
	return "", fmt.Errorf("LLM 응답에서 유효한 slug 추출 실패 (raw=%q)", text)
}
return slug, nil
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

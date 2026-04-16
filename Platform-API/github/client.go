package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"platform-api/config"
)

var httpClient = &http.Client{}

func doRequest(method, path string, body interface{}) ([]byte, int, error) {
	return doRequestWithToken(method, path, body, config.GitHubToken)
}

// doRequestAs 사용자 토큰으로 Gitea API 호출 (토큰이 없으면 관리자 토큰 사용)
func doRequestAs(method, path string, body interface{}, userToken string) ([]byte, int, error) {
	token := userToken
	if token == "" {
		token = config.GitHubToken
	}
	return doRequestWithToken(method, path, body, token)
}

func doRequestWithToken(method, path string, body interface{}, token string) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}

	u := config.GitHubAPI + path
	req, err := http.NewRequest(method, u, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

func parseJSON(data []byte) interface{} {
	var result interface{}
	json.Unmarshal(data, &result)
	return result
}

// --- User ---

// CreateUser Gitea에 사용자 생성
func CreateUser(username, email, fullName, password string) error {
	body := map[string]interface{}{
		"username":             username,
		"email":                email,
		"full_name":            fullName,
		"password":             password,
		"must_change_password": false,
	}
	data, status, err := doRequest("POST", "/admin/users", body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("create user failed (%d): %s", status, string(data))
	}
	return nil
}

// getOrgMembersTeamID 조직의 Members 팀 ID 조회
func getOrgMembersTeamID() (int, error) {
	if config.GitHubOrg == "" {
		return 0, fmt.Errorf("no org configured")
	}
	data, status, err := doRequest("GET", "/orgs/"+config.GitHubOrg+"/teams", nil)
	if err != nil || status >= 400 {
		return 0, fmt.Errorf("get teams failed")
	}
	var teams []map[string]interface{}
	json.Unmarshal(data, &teams)
	for _, t := range teams {
		name, _ := t["name"].(string)
		if name == "Members" {
			return int(t["id"].(float64)), nil
		}
	}
	return 0, fmt.Errorf("Members team not found")
}

// AddUserToOrgTeam 사용자를 조직 Members 팀에 추가
func AddUserToOrgTeam(username string) error {
	teamID, err := getOrgMembersTeamID()
	if err != nil {
		return err
	}
	_, st, err := doRequest("PUT", fmt.Sprintf("/teams/%d/members/%s", teamID, username), nil)
	if err != nil || st >= 400 {
		return fmt.Errorf("add to team failed (%d)", st)
	}
	return nil
}

// AddRepoToOrgTeam repo를 조직 Members 팀에 추가 (멤버가 repo에 접근 가능하도록)
func AddRepoToOrgTeam(repoName string) error {
	if config.GitHubOrg == "" {
		return nil
	}
	teamID, err := getOrgMembersTeamID()
	if err != nil {
		return err
	}
	_, st, err := doRequest("PUT", fmt.Sprintf("/teams/%d/repos/%s/%s", teamID, config.GitHubOrg, repoName), nil)
	if err != nil || st >= 400 {
		return fmt.Errorf("add repo to team failed (%d)", st)
	}
	return nil
}

// CheckUsersExist 사용자 목록 중 존재하지 않는 사용자를 반환
func CheckUsersExist(usernames []string) []string {
	var missing []string
	for _, u := range usernames {
		_, status, err := doRequest("GET", "/users/"+u, nil)
		if err != nil || status == 404 {
			missing = append(missing, u)
		}
	}
	return missing
}

// ListUsers Git 플랫폼의 사용자 목록 조회
func ListUsers() ([]map[string]interface{}, error) {
	data, status, err := doRequest("GET", "/admin/users?limit=100", nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list users failed (%d)", status)
	}
	var users []map[string]interface{}
	json.Unmarshal(data, &users)
	return users, nil
}

// CheckEmailExists Gitea에서 이메일 중복 여부 확인
func CheckEmailExists(email string) bool {
	users, err := ListUsers()
	if err != nil {
		return false
	}
	for _, u := range users {
		if e, _ := u["email"].(string); e == email {
			return true
		}
	}
	return false
}

// IsOwnEmail 해당 Gitea 사용자의 이메일인지 확인
func IsOwnEmail(username, email string) bool {
	if username == "" {
		return false
	}
	data, status, err := doRequest("GET", "/users/"+username, nil)
	if err != nil || status >= 400 {
		return false
	}
	var user map[string]interface{}
	json.Unmarshal(data, &user)
	e, _ := user["email"].(string)
	return e == email
}

// CreateUserToken Gitea 사용자의 API 토큰 생성 (Basic Auth 사용)
func CreateUserToken(username, password string) (string, error) {
	u := config.GitHubAPI + fmt.Sprintf("/users/%s/tokens", username)
	body := map[string]interface{}{
		"name":   "gojira-auto",
		"scopes": []string{"all"},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", u, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(username, password)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("create token failed (%d): %s", resp.StatusCode, string(data))
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	token, _ := result["sha1"].(string)
	return token, nil
}

// --- Repo ---

func CreateRepo(name, description string, isPrivate bool) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"name":        name,
		"description": description,
		"private":     isPrivate,
		"auto_init":   true,
	}

	// 조직으로 먼저 시도, 실패하면 개인 계정으로 fallback
	if config.GitHubOrg != "" {
		data, status, err := doRequest("POST", "/orgs/"+config.GitHubOrg+"/repos", body)
		if err != nil {
			return nil, err
		}
		if status < 400 {
			result := map[string]interface{}{}
			json.Unmarshal(data, &result)
			return result, nil
		}
	}

	// 개인 계정
	data, status, err := doRequest("POST", "/user/repos", body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create repo failed (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

func DeleteRepo(owner, repo string) error {
	_, status, err := doRequest("DELETE", "/repos/"+owner+"/"+repo, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete repo failed (%d)", status)
	}
	return nil
}

func GetRepo(owner, repo string) (map[string]interface{}, error) {
	data, status, err := doRequest("GET", "/repos/"+owner+"/"+repo, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get repo failed (%d)", status)
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// ListOrgRepos 조직 또는 사용자의 repo 목록 조회
func ListOrgRepos() ([]map[string]interface{}, error) {
	path := "/user/repos?per_page=100&sort=updated"
	if config.GitHubOrg != "" {
		path = "/orgs/" + config.GitHubOrg + "/repos?per_page=100&sort=updated"
	}
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	// org가 없으면 user로 fallback
	if status >= 400 && config.GitHubOrg != "" {
		data, status, err = doRequest("GET", "/user/repos?per_page=100&sort=updated", nil)
		if err != nil {
			return nil, err
		}
	}
	if status >= 400 {
		return nil, fmt.Errorf("list repos failed (%d)", status)
	}
	var repos []map[string]interface{}
	json.Unmarshal(data, &repos)
	return repos, nil
}

// --- Webhook ---

// CreateWebhook 레포지토리에 webhook을 등록한다.
func CreateWebhook(owner, repo, webhookURL, secret string, events []string) error {
	if webhookURL == "" {
		return nil // URL이 없으면 skip
	}
	path := fmt.Sprintf("/repos/%s/%s/hooks", owner, repo)
	body := map[string]interface{}{
		"config": map[string]string{
			"url":          webhookURL,
			"content_type": "json",
			"secret":       secret,
		},
		"events": events,
		"active": true,
	}
	data, status, err := doRequest("POST", path, body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("create webhook failed (%d): %s", status, string(data))
	}
	return nil
}

// --- File CRUD (Contents API) ---

// CreateOrUpdateFile creates or updates a file in a repo.
// sha is required for updates (empty for new files).
func CreateFile(owner, repo, filePath, content, message, branch string) (map[string]interface{}, error) {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"content": content, // base64 encoded
	}
	if branch != "" {
		body["branch"] = branch
	}
	data, status, err := doRequest("POST", u, body)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create file failed (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// CreateFileOnBranch creates a new file on a specific branch with user token.
func CreateFileOnBranch(owner, repo, filePath, content, message, branch, userToken string) (map[string]interface{}, error) {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"content": content,
		"branch":  branch,
	}
	var data []byte
	var status int
	var err error
	// Gitea git-push 경쟁(cannot lock ref) 대비: 최대 5회 지수 백오프 재시도.
	for attempt := 0; attempt < 5; attempt++ {
		if userToken != "" {
			data, status, err = doRequestAs("POST", u, body, userToken)
		} else {
			data, status, err = doRequest("POST", u, body)
		}
		if err != nil {
			return nil, err
		}
		if status < 400 {
			break
		}
		// Gitea 는 race 에러 세부 메시지를 body 에 담지 않고 generic swagger URL 만 반환하는 경우가 있어
		// 500 은 무조건 재시도 대상으로 처리 (idempotent 한 create 요청이므로 안전)
		retriable := status == 500
		if !retriable {
			return nil, fmt.Errorf("create file failed (%d): %s", status, string(data))
		}
		time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
	}
	if status >= 400 {
		return nil, fmt.Errorf("create file failed after retry (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

func CreateOrUpdateFile(owner, repo, filePath, content, message, branch, sha string) (map[string]interface{}, error) {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"content": content, // base64 encoded
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}
	var data []byte
	var status int
	var err error
	// Gitea git-push race(cannot lock ref) 대비: 최대 5회 지수 백오프 재시도
	for attempt := 0; attempt < 5; attempt++ {
		data, status, err = doRequest("PUT", u, body)
		if err != nil {
			return nil, err
		}
		if status < 400 {
			break
		}
		if status != 500 {
			return nil, fmt.Errorf("create/update file failed (%d): %s", status, string(data))
		}
		time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
	}
	if status >= 400 {
		return nil, fmt.Errorf("create/update file failed after retry (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// CreateOrUpdateFileAs creates/updates a file with user token.
func CreateOrUpdateFileAs(owner, repo, filePath, content, message, branch, sha, userToken string) (map[string]interface{}, error) {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"content": content,
		"branch":  branch,
	}
	if sha != "" {
		body["sha"] = sha
	}
	var data []byte
	var status int
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		data, status, err = doRequestAs("PUT", u, body, userToken)
		if err != nil {
			return nil, err
		}
		if status < 400 || status != 500 {
			break
		}
		time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
	}
	if status >= 400 {
		return nil, fmt.Errorf("create/update file failed (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// DeleteFile deletes a file from a repo.
func DeleteFile(owner, repo, filePath, message, branch, sha string) error {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"sha":     sha,
		"branch":  branch,
	}
	_, status, err := doRequest("DELETE", u, body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete file failed (%d)", status)
	}
	return nil
}

// DeleteFileAs deletes a file with user token.
func DeleteFileAs(owner, repo, filePath, message, branch, sha, userToken string) error {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, filePath)
	body := map[string]interface{}{
		"message": message,
		"sha":     sha,
		"branch":  branch,
	}
	_, status, err := doRequestAs("DELETE", u, body, userToken)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete file failed (%d)", status)
	}
	return nil
}

// --- Branch ---

// GetBranchRef gets the SHA of a branch.
func GetBranchRef(owner, repo, branch string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", owner, repo, branch)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return "", err
	}
	if status >= 400 {
		return "", fmt.Errorf("get branch ref failed (%d)", status)
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	obj, _ := result["object"].(map[string]interface{})
	sha, _ := obj["sha"].(string)
	return sha, nil
}

// CreateBranch creates a new branch from a source branch SHA.
func CreateBranch(owner, repo, newBranch, fromSHA string) error {
	if IsGitea() {
		// Gitea API: POST /repos/{owner}/{repo}/branches
		path := fmt.Sprintf("/repos/%s/%s/branches", owner, repo)
		body := map[string]string{
			"new_branch_name": newBranch,
			"old_branch_name": "main",
		}
		data, status, err := doRequest("POST", path, body)
		if err != nil {
			return err
		}
		if status >= 400 {
			return fmt.Errorf("create branch failed (%d): %s", status, string(data))
		}
		return nil
	}
	// GitHub API
	path := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)
	body := map[string]string{
		"ref": "refs/heads/" + newBranch,
		"sha": fromSHA,
	}
	data, status, err := doRequest("POST", path, body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("create branch failed (%d): %s", status, string(data))
	}
	return nil
}

// --- Tree ---

func GetTree(owner, repo, ref string) (interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, url.PathEscape(ref))
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get tree failed (%d)", status)
	}
	return parseJSON(data), nil
}

// --- File content ---

func GetFileContent(owner, repo, path string, ref string) (map[string]interface{}, error) {
	u := fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	data, status, err := doRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get file failed (%d)", status)
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// --- Commits ---

func ListCommits(owner, repo, branch string, perPage int) (interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=%d", owner, repo, branch, perPage)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list commits failed (%d)", status)
	}
	return parseJSON(data), nil
}

// --- Branches ---

func ListBranches(owner, repo string) (interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/branches", owner, repo)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list branches failed (%d)", status)
	}
	return parseJSON(data), nil
}

// --- Pull Requests ---

func CreatePR(owner, repo, title, body, head, base string, userToken ...string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	payload := map[string]string{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}
	token := ""
	if len(userToken) > 0 {
		token = userToken[0]
	}
	data, status, err := doRequestAs("POST", path, payload, token)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("create PR failed (%d): %s", status, string(data))
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

func ListPRs(owner, repo, state string) (interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls?state=%s", owner, repo, state)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list PRs failed (%d)", status)
	}
	return parseJSON(data), nil
}

func GetPR(owner, repo string, prNumber int) (map[string]interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, prNumber)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("get PR failed (%d)", status)
	}
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result, nil
}

// CreateReview PR에 리뷰(승인/반려)를 생성하고, 리뷰 ID를 반환한다.
func CreateReview(owner, repo string, prNumber int, event, body string, userToken ...string) (int, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	payload := map[string]string{"event": event, "body": body}
	token := ""
	if len(userToken) > 0 {
		token = userToken[0]
	}
	data, status, err := doRequestAs("POST", path, payload, token)
	if err != nil {
		return 0, err
	}
	if status >= 400 {
		return 0, fmt.Errorf("create review failed (%d): %s", status, string(data))
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	id, _ := result["id"].(float64)
	return int(id), nil
}

func MergePR(owner, repo string, prNumber int, mergeMethod string, userToken ...string) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	method := "PUT"
	var payload interface{} = map[string]string{"merge_method": mergeMethod}
	if IsGitea() {
		method = "POST"
		// force_merge: branch protection 우회 (admin 권한 필요)
		payload = map[string]interface{}{
			"Do":          mergeMethod,
			"force_merge": true,
		}
	}
	token := ""
	if len(userToken) > 0 {
		token = userToken[0]
	}

	// Gitea 405 (repo lock) 재시도
	maxRetries := 5
	for i := 0; i <= maxRetries; i++ {
		data, status, err := doRequestAs(method, path, payload, token)
		if err != nil {
			return err
		}
		if status == 405 && i < maxRetries {
			log.Printf("MergePR: repo locked (405), retry %d/%d after 2s", i+1, maxRetries)
			time.Sleep(2 * time.Second)
			continue
		}
		if status >= 400 {
			return fmt.Errorf("merge PR failed (%d): %s", status, string(data))
		}
		return nil
	}
	return fmt.Errorf("merge PR failed: repo locked after %d retries", maxRetries)
}

// UpdatePRBranch PR 브랜치를 base(main)와 동기화 (rebase 또는 merge)
func UpdatePRBranch(owner, repo string, prNumber int, style string) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/update", owner, repo, prNumber)
	if style == "" {
		style = "rebase"
	}
	payload := map[string]string{"style": style}
	data, status, err := doRequest("POST", path, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("update branch failed (%d): %s", status, string(data))
	}
	return nil
}

// HasUserCommits 사용자 브랜치에 main에 없는 커밋이 있는지 확인
// main...head 비교에서 total_commits > 0이면 사용자 변경 있음
func HasUserCommits(owner, repo, base, head string) bool {
	result, err := CompareBranches(owner, repo, base, head)
	if err != nil {
		return false
	}
	if tc, ok := result["total_commits"].(float64); ok {
		return tc > 0
	}
	return false
}

// CompareBranches 두 브랜치 간 diff 비교
func CompareBranches(owner, repo, base, head string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/compare/%s...%s", owner, repo,
		url.PathEscape(base), url.PathEscape(head))
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("compare failed (%d): %s", status, string(data))
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

// --- Reviewers ---

func RequestReviewers(owner, repo string, prNumber int, reviewers []string) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", owner, repo, prNumber)
	payload := map[string]interface{}{"reviewers": reviewers}
	data, status, err := doRequest("POST", path, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("request reviewers failed (%d): %s", status, string(data))
	}
	return nil
}

// RemoveRequestedReviewer 리뷰 완료 후 requested_reviewers에서 제거
func RemoveRequestedReviewer(owner, repo string, prNumber int, username string) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", owner, repo, prNumber)
	payload := map[string]interface{}{"reviewers": []string{username}}
	doRequest("DELETE", path, payload)
}

// --- Branch Protection ---

// SetBranchProtection main 브랜치에 보호 규칙 설정 (모든 리뷰 완료 전 머지 불가)
func DeleteBranch(owner, repo, branch string) error {
	path := fmt.Sprintf("/repos/%s/%s/branches/%s", owner, repo, branch)
	_, status, err := doRequest("DELETE", path, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete branch failed (%d)", status)
	}
	return nil
}

func DeleteBranchProtection(owner, repo, branch string) error {
	if IsGitea() {
		path := fmt.Sprintf("/repos/%s/%s/branch_protections/%s", owner, repo, branch)
		_, _, _ = doRequest("DELETE", path, nil)
		return nil
	}
	path := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, branch)
	_, _, _ = doRequest("DELETE", path, nil)
	return nil
}

func SetBranchProtection(owner, repo, branch string, requiredReviewers int) error {
	if IsGitea() {
		return setBranchProtectionGitea(owner, repo, branch, requiredReviewers)
	}
	return setBranchProtectionGitHub(owner, repo, branch, requiredReviewers)
}

func setBranchProtectionGitHub(owner, repo, branch string, requiredReviewers int) error {
	path := fmt.Sprintf("/repos/%s/%s/branches/%s/protection", owner, repo, branch)
	body := map[string]interface{}{
		"required_pull_request_reviews": map[string]interface{}{
			"required_approving_review_count": requiredReviewers,
			"dismiss_stale_reviews":           true,
		},
		"enforce_admins":         false,
		"required_status_checks": nil,
		"restrictions":           nil,
	}
	data, status, err := doRequest("PUT", path, body)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("set branch protection failed (%d): %s", status, string(data))
	}
	return nil
}

func setBranchProtectionGitea(owner, repo, branch string, requiredReviewers int) error {
	path := fmt.Sprintf("/repos/%s/%s/branch_protections", owner, repo)
	body := map[string]interface{}{
		"branch_name":                   branch,
		"enable_push":                   false,
		"enable_merge_whitelist":        false,
		"enable_approvals_whitelist":    false,
		"required_approvals":            requiredReviewers,
		"dismiss_stale_approvals":       true,
		"block_on_rejected_reviews":     true,
		"block_on_outdated_branch":      false,
	}
	data, status, err := doRequest("POST", path, body)
	if err != nil {
		return err
	}
	// 이미 존재하면 PATCH로 업데이트
	if status == 422 || status == 409 {
		path = fmt.Sprintf("/repos/%s/%s/branch_protections/%s", owner, repo, branch)
		data, status, err = doRequest("PATCH", path, body)
		if err != nil {
			return err
		}
	}
	if status >= 400 {
		return fmt.Errorf("set branch protection failed (%d): %s", status, string(data))
	}
	return nil
}

// FixReviewState Gitea DB에서 리뷰 type을 직접 업데이트 (Gitea 1.25 API 버그 우회)
// event: "APPROVE" → type=1, "REQUEST_CHANGES" → type=2
func FixReviewState(reviewID int, event string) error {
	typeVal := 0
	switch event {
	case "APPROVE":
		typeVal = 1
	case "REQUEST_CHANGES":
		typeVal = 2
	default:
		return nil
	}
	cmd := exec.Command("docker", "exec", "gojira-gitea", "sqlite3", "/data/gitea/gitea.db",
		fmt.Sprintf("UPDATE review SET type=%d, official=1 WHERE id=%d;", typeVal, reviewID))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fix review state failed: %v (%s)", err, string(out))
	}
	return nil
}

// IsGitea API URL로 Gitea 여부 판별
func IsGitea() bool {
	return strings.Contains(config.GitHubAPI, "/api/v1")
}

func ListReviews(owner, repo string, prNumber int) (interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	data, status, err := doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("list reviews failed (%d)", status)
	}
	return parseJSON(data), nil
}

package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"platform-api/config"
)

var httpClient = &http.Client{}

func doRequest(method, path string, body interface{}) ([]byte, int, error) {
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
	if config.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+config.GitHubToken)
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
	data, status, err := doRequest("PUT", u, body)
	if err != nil {
		return nil, err
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
	path := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, ref)
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

func CreatePR(owner, repo, title, body, head, base string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	payload := map[string]string{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}
	data, status, err := doRequest("POST", path, payload)
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

func CreateReview(owner, repo string, prNumber int, event, body string) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, prNumber)
	payload := map[string]string{"event": event, "body": body}
	data, status, err := doRequest("POST", path, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("create review failed (%d): %s", status, string(data))
	}
	return nil
}

func MergePR(owner, repo string, prNumber int, mergeMethod string) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, prNumber)
	payload := map[string]string{"merge_method": mergeMethod}
	data, status, err := doRequest("PUT", path, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("merge PR failed (%d): %s", status, string(data))
	}
	return nil
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

// --- Branch Protection ---

// SetBranchProtection main 브랜치에 보호 규칙 설정 (모든 리뷰 완료 전 머지 불가)
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

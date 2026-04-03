package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

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

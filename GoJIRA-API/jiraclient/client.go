package jiraclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"gojira-api/config"
)

var httpClient = &http.Client{}

func baseURL() string {
	return config.JiraMockURL + config.JiraAPIBase
}

// GetIssue fetches a single issue by key or ID.
func GetIssue(issueKeyOrID string) (map[string]interface{}, error) {
	resp, err := httpClient.Get(baseURL() + "/issue/" + url.PathEscape(issueKeyOrID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("issue not found: %s", issueKeyOrID)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListIssues fetches issue list with pagination.
func ListIssues(startAt, maxResults int) (map[string]interface{}, error) {
	u := fmt.Sprintf("%s/issue?startAt=%d&maxResults=%d", baseURL(), startAt, maxResults)
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// SearchIssues searches issues with JQL.
func SearchIssues(jql string, startAt, maxResults int) (map[string]interface{}, error) {
	u := fmt.Sprintf("%s/search?jql=%s&startAt=%d&maxResults=%d",
		baseURL(), url.QueryEscape(jql), startAt, maxResults)
	resp, err := httpClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateIssue creates a new issue.
func CreateIssue(fields map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{"fields": fields}
	body, _ := json.Marshal(payload)

	resp, err := httpClient.Post(baseURL()+"/issue", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create issue failed (%d): %s", resp.StatusCode, string(errBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateIssue updates an existing issue.
func UpdateIssue(issueKey string, fields map[string]interface{}) error {
	payload := map[string]interface{}{"fields": fields}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPut, baseURL()+"/issue/"+url.PathEscape(issueKey), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update issue failed (%d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// UpdateIssueWithOps updates using the "update" operation format.
func UpdateIssueWithOps(issueKey string, update map[string]interface{}) error {
	payload := map[string]interface{}{"update": update}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPut, baseURL()+"/issue/"+url.PathEscape(issueKey), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update issue failed (%d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// DeleteIssue deletes an issue.
func DeleteIssue(issueKey string) error {
	req, _ := http.NewRequest(http.MethodDelete, baseURL()+"/issue/"+url.PathEscape(issueKey), nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete issue failed (%d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// AddLabel adds a label to an issue.
func AddLabel(issueKey, label string) error {
	update := map[string]interface{}{
		"labels": []map[string]interface{}{
			{"add": label},
		},
	}
	return UpdateIssueWithOps(issueKey, update)
}

// RemoveLabel removes a label from an issue.
func RemoveLabel(issueKey, label string) error {
	update := map[string]interface{}{
		"labels": []map[string]interface{}{
			{"remove": label},
		},
	}
	return UpdateIssueWithOps(issueKey, update)
}

// CreateIssueLink creates a link between two issues.
func CreateIssueLink(inwardKey, outwardKey string) error {
	payload := map[string]interface{}{
		"type":         map[string]string{"name": "Contains(WBSGantt)"},
		"inwardIssue":  map[string]string{"key": inwardKey},
		"outwardIssue": map[string]string{"key": outwardKey},
	}
	body, _ := json.Marshal(payload)

	resp, err := httpClient.Post(baseURL()+"/issueLink", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create link failed (%d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

// DeleteIssueLink deletes an issue link.
func DeleteIssueLink(linkID string) error {
	req, _ := http.NewRequest(http.MethodDelete, baseURL()+"/issueLink/"+url.PathEscape(linkID), nil)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete link failed (%d): %s", resp.StatusCode, string(errBody))
	}
	return nil
}

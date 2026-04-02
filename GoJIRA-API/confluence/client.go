package confluence

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

func doRequest(req *http.Request) (*http.Response, error) {
	if config.ConfluenceUser != "" {
		req.SetBasicAuth(config.ConfluenceUser, config.ConfluencePass)
	}
	req.Header.Set("Content-Type", "application/json")
	return httpClient.Do(req)
}

// FindPage searches for a page by spaceKey and title.
func FindPage(spaceKey, title string) (map[string]interface{}, error) {
	u := fmt.Sprintf("%s/rest/api/content?spaceKey=%s&title=%s&expand=version",
		config.ConfluenceURL, url.QueryEscape(spaceKey), url.QueryEscape(title))
	req, _ := http.NewRequest("GET", u, nil)
	resp, err := doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	results, _ := result["results"].([]interface{})
	if len(results) == 0 {
		return nil, nil
	}
	page, _ := results[0].(map[string]interface{})
	return page, nil
}

// CreatePage creates a new Confluence page.
func CreatePage(spaceKey, title, bodyHTML string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"type":  "page",
		"title": title,
		"space": map[string]string{"key": spaceKey},
		"body": map[string]interface{}{
			"storage": map[string]string{
				"value":          bodyHTML,
				"representation": "storage",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", config.ConfluenceURL+"/rest/api/content", bytes.NewReader(body))
	resp, err := doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create page failed (%d): %s", resp.StatusCode, string(errBody))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// UpdatePage updates an existing Confluence page.
func UpdatePage(pageID, title, bodyHTML string, version int) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"type":    "page",
		"title":   title,
		"version": map[string]int{"number": version},
		"body": map[string]interface{}{
			"storage": map[string]string{
				"value":          bodyHTML,
				"representation": "storage",
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PUT", config.ConfluenceURL+"/rest/api/content/"+pageID, bytes.NewReader(body))
	resp, err := doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update page failed (%d): %s", resp.StatusCode, string(errBody))
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// UpsertPage creates or updates a page (find by title, create if not exists, update if exists).
func UpsertPage(spaceKey, title, bodyHTML string) (map[string]interface{}, error) {
	existing, err := FindPage(spaceKey, title)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		return CreatePage(spaceKey, title, bodyHTML)
	}

	pageID := fmt.Sprintf("%v", existing["id"])
	ver := 1
	if version, ok := existing["version"].(map[string]interface{}); ok {
		if n, ok := version["number"].(float64); ok {
			ver = int(n) + 1
		}
	}
	return UpdatePage(pageID, title, bodyHTML, ver)
}

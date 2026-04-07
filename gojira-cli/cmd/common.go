package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func apiURL() string {
	if v := os.Getenv("GOJIRA_API_URL"); v != "" {
		return v
	}
	return "http://localhost:9001/api/v2"
}

func platformURL() string {
	if v := os.Getenv("GOJIRA_PLATFORM_URL"); v != "" {
		return v
	}
	return "http://localhost:9000/api/v2"
}

func userID() string {
	if v := os.Getenv("GOJIRA_USER"); v != "" {
		return v
	}
	return ""
}

func apiGet(path string, headers map[string]string) (interface{}, error) {
	req, _ := http.NewRequest("GET", apiURL()+path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func platformGet(path string) (interface{}, error) {
	req, _ := http.NewRequest("GET", platformURL()+path, nil)
	return doRequest(req)
}

func apiPost(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func apiPut(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func apiDelete(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("DELETE", apiURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func platformPost(path string, body interface{}) (interface{}, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", platformURL()+path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID())
	return doRequest(req)
}

func doRequest(req *http.Request) (interface{}, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	var result interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func outputJSON(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func parseFlag(args []string, name string) string {
	for i, a := range args {
		if a == "--"+name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--"+name+"=") {
			return a[len("--"+name+"="):]
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name {
			return true
		}
	}
	return false
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

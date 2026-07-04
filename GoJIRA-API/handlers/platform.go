package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"gojira-api/config"
)

// Platform-API 서비스간 HTTP 클라이언트 (project.go, llm.go 등에서 공용)

var platformAPI = "http://localhost:9000/api/v2"

func init() {
	if v := config.PlatformAPI; v != "" {
		platformAPI = v
	}
}

var platformClient = &http.Client{Timeout: 120 * time.Second}

func callPlatform(method, path string, body interface{}, userID ...string) (interface{}, int) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, platformAPI+path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if len(userID) > 0 && userID[0] != "" {
		req.Header.Set("X-User-Id", userID[0])
	}
	// 서비스간 호출 — Platform-API auth 미들웨어 통과용 서비스 토큰(SSO 모드).
	if st := os.Getenv("SERVICE_TOKEN"); st != "" {
		req.Header.Set("X-Service-Token", st)
	}
	resp, err := platformClient.Do(req)
	if err != nil {
		return nil, 500
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result interface{}
	json.Unmarshal(data, &result)
	return result, resp.StatusCode
}

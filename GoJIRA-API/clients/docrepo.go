// Package clients — 외부 서비스 HTTP 클라이언트.
//
// DocRepo: Platform-API 의 /DocRepo/{code}/* 엔드포인트 래퍼.
// 이슈 YAML 등 git 원천 파일을 user 브랜치에 저장/삭제한다.
package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gojira-api/config"
)

var docRepoHTTP = &http.Client{Timeout: 30 * time.Second}

// SaveFile — PUT /DocRepo/{code}/file  (user 브랜치 커밋)
// Platform-API 가 X-User-Id 헤더로 docs/{user} 브랜치를 식별한다.
func SaveFile(code, path, content, message, user string) error {
	if user == "" {
		return fmt.Errorf("user required")
	}
	payload, _ := json.Marshal(map[string]string{
		"path":    path,
		"content": content,
		"message": message,
	})
	req, _ := http.NewRequest(
		http.MethodPut,
		fmt.Sprintf("%s/DocRepo/%s/file", config.PlatformAPI, code),
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", user)
	resp, err := docRepoHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("docrepo save_file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("docrepo save_file %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// DeleteFile — DELETE /DocRepo/{code}/file  (user 브랜치)
// Platform-API 는 path/message 를 JSON body 로 받는다.
// 404 는 성공으로 취급 (이미 없음).
func DeleteFile(code, path, message, user string) error {
	if user == "" {
		return fmt.Errorf("user required")
	}
	payload, _ := json.Marshal(map[string]string{"path": path, "message": message})
	req, _ := http.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("%s/DocRepo/%s/file", config.PlatformAPI, code),
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", user)
	resp, err := docRepoHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("docrepo delete_file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 || resp.StatusCode < 400 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("docrepo delete_file %d: %s", resp.StatusCode, string(b))
}

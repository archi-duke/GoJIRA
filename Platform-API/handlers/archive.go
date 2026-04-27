package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"

	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// POST /api/v{version}/Project/{code}/archive
//
// archive-project 스킬과 동등한 통합 흐름:
//  1. mongo dump (GoJIRA-API GET /Project/{code}/dump)
//  2. project.documents 의 bare repo 복사
//  3. zip 묶기 → $HOME/.gojira-backups/{code}-{ts}.zip
//  4. Gitea/GitHub repo + 로컬 클론 삭제
//  5. mongo hard delete (GoJIRA-API POST /Project/{code}/reset deleteProject=true)
//
// Body: { confirm: bool }
//   - confirm=false: dry-run. zip 만들기 전 영향 범위만 반환
//   - confirm=true: 실행
//
// 헤더 X-User-Id 필수.
func ProjectArchiveHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		respondError(w, http.StatusBadRequest, "code path param required")
		return
	}
	user := r.Header.Get("X-User-Id")
	if user == "" {
		respondError(w, http.StatusForbidden, "X-User-Id header required")
		return
	}
	var body struct {
		Confirm bool `json:"confirm"`
	}
	_ = decodeJSON(r, &body)

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// 1. mongo dump (GoJIRA-API)
	dumpData, err := callGoJira(ctx, "GET", fmt.Sprintf("/Project/%s/dump", code), nil, user)
	if err != nil {
		respondError(w, http.StatusBadGateway, "dump 호출 실패: "+err.Error())
		return
	}

	// 2. project doc → repo 정보
	prj, prjErr := db.FindProjectByCode(ctx, code)
	if prjErr != nil || prj == nil {
		respondError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", code))
		return
	}
	docs := db.BsonGetDoc(prj, "documents")
	repoOwner := db.BsonGetStr(docs, "repoOwner")
	repoName := db.BsonGetStr(docs, "repoName")

	// 3. dry-run
	if !body.Confirm {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"code":      code,
			"mode":      "dry-run",
			"repoOwner": repoOwner,
			"repoName":  repoName,
			"dumpBytes": len(dumpData),
			"message":   "DRY-RUN. confirm=true 로 재호출 시 zip 생성 + 자원 정리 실행",
		})
		return
	}

	// 4. zip 만들기 → $HOME/.gojira-backups
	timestamp := time.Now().Format("20060102150405")
	backupDir := filepath.Join(os.Getenv("HOME"), ".gojira-backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "backup dir 생성 실패: "+err.Error())
		return
	}
	zipPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s.zip", code, timestamp))
	zipFile, err := os.Create(zipPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "zip 생성 실패: "+err.Error())
		return
	}
	zw := zip.NewWriter(zipFile)
	cleanupZipOnError := func() {
		_ = zw.Close()
		_ = zipFile.Close()
		_ = os.Remove(zipPath)
	}

	// 4-1. mongo dump
	if err := writeZipBytes(zw, "mongo/dump.json", dumpData); err != nil {
		cleanupZipOnError()
		respondError(w, http.StatusInternalServerError, "zip mongo: "+err.Error())
		return
	}

	// 4-2. manifest.json
	manifest, _ := json.MarshalIndent(map[string]interface{}{
		"schemaVersion": 1,
		"code":          code,
		"createdAt":     time.Now().Format(time.RFC3339),
		"createdBy":     user,
		"repoOwner":     repoOwner,
		"repoName":      repoName,
	}, "", "  ")
	if err := writeZipBytes(zw, "manifest.json", manifest); err != nil {
		cleanupZipOnError()
		respondError(w, http.StatusInternalServerError, "zip manifest: "+err.Error())
		return
	}

	// 4-3. bare repo 복사
	repoArchived := false
	if repoOwner != "" && repoName != "" {
		bareDir := filepath.Join(config.RepoBaseDir, repoOwner, repoName+".git")
		if _, statErr := os.Stat(bareDir); statErr == nil {
			archiveBase := fmt.Sprintf("repo/%s/%s.git", repoOwner, repoName)
			if err := zipDirectory(zw, bareDir, archiveBase); err != nil {
				log.Printf("repo zip warn: %v", err)
			} else {
				repoArchived = true
			}
		}
	}

	if err := zw.Close(); err != nil {
		_ = zipFile.Close()
		_ = os.Remove(zipPath)
		respondError(w, http.StatusInternalServerError, "zip close: "+err.Error())
		return
	}
	if err := zipFile.Close(); err != nil {
		_ = os.Remove(zipPath)
		respondError(w, http.StatusInternalServerError, "file close: "+err.Error())
		return
	}

	// 5. 자원 정리 — zip 성공한 후에만 실행
	githubDeleted := false
	if repoOwner != "" && repoName != "" {
		if err := github.DeleteRepo(repoOwner, repoName); err != nil {
			log.Printf("github DeleteRepo warn (%s/%s): %v", repoOwner, repoName, err)
		} else {
			githubDeleted = true
		}
		_ = github.RemoveLocalRepo(repoOwner, repoName)
		db.ClearProjectDocRepo(ctx, code)
	}

	// 6. mongo hard delete via GoJIRA-API
	resetBody, _ := json.Marshal(map[string]interface{}{
		"confirm":           true,
		"resetIssues":       true,
		"resetArchSettings": true,
		"deleteProject":     true,
	})
	resetResp, resetErr := callGoJira(ctx, "POST", fmt.Sprintf("/Project/%s/reset", code), resetBody, user)
	var resetResult map[string]interface{}
	if resetErr == nil {
		_ = json.Unmarshal(resetResp, &resetResult)
	} else {
		log.Printf("reset 호출 실패: %v", resetErr)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"code":          code,
		"mode":          "executed",
		"backupPath":    zipPath,
		"repoArchived":  repoArchived,
		"githubDeleted": githubDeleted,
		"reset":         resetResult,
	})
}

// callGoJira: Platform-API → GoJIRA-API HTTP 호출 헬퍼
func callGoJira(ctx context.Context, method, path string, bodyJSON []byte, user string) ([]byte, error) {
	url := config.GoJiraAPI + path
	var reqBody io.Reader
	if bodyJSON != nil {
		reqBody = bytes.NewReader(bodyJSON)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}
	if bodyJSON != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("X-User-Id", user)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(data))
	}
	return data, nil
}

func writeZipBytes(zw *zip.Writer, name string, data []byte) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	return err
}

func zipDirectory(zw *zip.Writer, srcDir, baseInZip string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		nameInZip := filepath.ToSlash(filepath.Join(baseInZip, rel))
		f, err := zw.Create(nameInZip)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(f, src)
		return err
	})
}

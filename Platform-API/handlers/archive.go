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

	authmw "platform-api/auth"
	"platform-api/config"
	"platform-api/db"
	"platform-api/github"
)

// POST /api/v{version}/Project/{code}/archive
//
// archive-project 스킬과 동등한 통합 흐름:
//  1. mongo dump (GoJIRA-API GET /Project/{code}/dump)
//  2. project.documents 의 bare repo 복사
//  3. zip 묶기 → <getBackupDir()>/{code}-{ts}.zip
//  4. Gitea/GitHub repo + 로컬 클론 삭제
//  5. mongo hard delete (GoJIRA-API POST /Project/{code}/reset deleteProject=true)

// getBackupDir 백업 zip 의 저장 디렉토리를 결정한다.
// 우선순위: GOJIRA_BACKUP_DIR env > $HOME/.gojira-backups > /tmp/.gojira-backups (fallback).
// 컨테이너에서 `user: "${UID}:${GID}"` override 시 /etc/passwd 에 uid 가 없어 HOME 이
// "/" 로 떨어지는 케이스가 있어 fallback 이 필요하다.
func getBackupDir() string {
	if d := os.Getenv("GOJIRA_BACKUP_DIR"); d != "" {
		return d
	}
	h := os.Getenv("HOME")
	if h == "" || h == "/" {
		return "/tmp/.gojira-backups"
	}
	return filepath.Join(h, ".gojira-backups")
}
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
	user := authmw.UserID(r.Context())
	if user == "" {
		respondError(w, http.StatusUnauthorized, "unauthorized")
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

	// 4. zip 만들기 → getBackupDir()
	//
	// 내보내기와 같은 함수(buildProjectArchive)를 쓴다. 예전에는 config.RepoBaseDir 의
	// bare 디렉터리를 복사했는데, Gitea 로 저장소가 넘어간 뒤 그 경로가 비어 있어
	// repoArchived=false 로 **조용히 건너뛰어졌다** (2026-11-18 발견). 삭제는 되돌릴 수
	// 없으므로 문서 빠진 백업은 그대로 영구 손실이다. 이제 묶음이 실패하면 삭제로
	// 넘어가지 않는다.
	build, err := buildProjectArchive(ctx, code, user)
	if err != nil {
		respondError(w, http.StatusBadGateway,
			"백업 묶음 생성 실패 — 아무것도 삭제하지 않았다: "+err.Error())
		return
	}
	defer build.Cleanup()

	timestamp := time.Now().Format("20060102150405")
	backupDir := getBackupDir()
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "backup dir 생성 실패: "+err.Error())
		return
	}
	zipPath := filepath.Join(backupDir, fmt.Sprintf("%s-%s.zip", code, timestamp))
	if err := copyFile(build.ZipPath, zipPath); err != nil {
		respondError(w, http.StatusInternalServerError, "백업 저장 실패 — 삭제하지 않았다: "+err.Error())
		return
	}
	repoArchived := true
	log.Printf("archive: %s 백업 %s (이슈 %d건, 문서 %d개)",
		code, zipPath, build.Manifest.IssueCount, bundleFileCount(build.Manifest))

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
	// 서비스간 호출 — GoJIRA-API auth 미들웨어를 위해 서비스 토큰 동반(SSO 모드 enforcement).
	if st := os.Getenv("SERVICE_TOKEN"); st != "" {
		req.Header.Set("X-Service-Token", st)
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

// copyFile 임시 zip 을 백업 디렉터리로 옮긴다.
// os.Rename 은 컨테이너에서 /tmp 와 /data 가 다른 파일시스템이라 쓸 수 없다.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

// bundleFileCount 로그용 — 요약이 없으면 -1.
func bundleFileCount(m transferManifest) int {
	if m.Repo == nil {
		return -1
	}
	return m.Repo.FileCount
}

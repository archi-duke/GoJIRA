package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// 지원 플랫폼: dist 디렉토리 하위명 → 다운로드 파일명.
// dist 레이아웃: $LAUNCHER_DIST_DIR/<os>/gojira-launcher[.exe]
var launcherPlatforms = map[string]struct {
	subdir   string
	filename string
}{
	"linux":   {"linux-amd64", "gojira-launcher"},
	"darwin":  {"darwin-arm64", "gojira-launcher"},
	"windows": {"windows-amd64", "gojira-launcher.exe"},
}

// GET /api/v2/launcher/download/{os} — 미리 빌드된 런처 바이너리 다운로드.
// 웹이 런처 미감지 상태에서 "설치" 버튼을 누르면 호출. dist 디렉토리는 bind mount.
func LauncherDownloadHandler(w http.ResponseWriter, r *http.Request) {
	osKey := strings.ToLower(chi.URLParam(r, "os"))
	plat, ok := launcherPlatforms[osKey]
	if !ok {
		respondError(w, http.StatusBadRequest, "지원하지 않는 OS: "+osKey+" (linux|darwin|windows)")
		return
	}
	distDir := os.Getenv("LAUNCHER_DIST_DIR")
	if distDir == "" {
		distDir = "/app/launcher-dist"
	}
	path := filepath.Join(distDir, plat.subdir, plat.filename)
	f, err := os.Open(path)
	if err != nil {
		respondError(w, http.StatusNotFound, "해당 OS 의 런처 바이너리가 호스팅되어 있지 않습니다: "+osKey)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "런처 파일 정보 조회 실패")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", plat.filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("X-Launcher-Os", osKey)
	_, _ = io.Copy(w, f)
}

// GET /api/v2/launcher/manifest — 다운로드 가능한 OS 목록 + 크기.
// 웹의 설치 모달이 호출해서 어떤 OS 가 실제로 호스팅되어 있는지 표시.
func LauncherManifestHandler(w http.ResponseWriter, r *http.Request) {
	distDir := os.Getenv("LAUNCHER_DIST_DIR")
	if distDir == "" {
		distDir = "/app/launcher-dist"
	}
	type entry struct {
		OS    string `json:"os"`
		Size  int64  `json:"size"`
		Ready bool   `json:"ready"`
	}
	out := []entry{}
	for osKey, plat := range launcherPlatforms {
		path := filepath.Join(distDir, plat.subdir, plat.filename)
		e := entry{OS: osKey}
		if info, err := os.Stat(path); err == nil {
			e.Size = info.Size()
			e.Ready = true
		}
		out = append(out, e)
	}
	respondJSON(w, http.StatusOK, map[string]any{"platforms": out})
}

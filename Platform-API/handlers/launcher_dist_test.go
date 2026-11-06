package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 게시본이 없을 때의 응답 — 원인을 추적할 수 있어야 한다.
//
//	실행: MONGO_URI=x PLATFORM_GITHUB_TOKEN=x PLATFORM_GITHUB_ORG=x \
//	      GITHUB_URL=http://x/ GOJIRA_API_URL=x go test ./handlers
//
// (config.init() 이 필수 환경변수를 요구하며 TestMain 보다 먼저 실행된다)
//
// 실제 사고: 체크아웃이 산출물(catalog.json)을 지웠는데 응답은 평문 "not found" 뿐이라,
// 런처에는 "카탈로그 없음", 웹에는 "설치할 서비스 없음" 으로만 보여 서버 게시 누락인지
// 네트워크 문제인지 구분할 수 없었다.
func TestServeLauncherDistFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LAUNCHER_DIST_DIR", dir)

	t.Run("있으면 그대로 서빙", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(`{"services":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		serveLauncherDistFile(w, "catalog.json", "application/json")
		if w.Code != http.StatusOK {
			t.Fatalf("HTTP %d, want 200", w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("Cache-Control=%q, want no-store", got)
		}
	})

	t.Run("없으면 404 에 파일명과 조치를 담는다", func(t *testing.T) {
		w := httptest.NewRecorder()
		serveLauncherDistFile(w, "release-manifest.json", "application/json")
		if w.Code != http.StatusNotFound {
			t.Fatalf("HTTP %d, want 404", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "release-manifest.json") {
			t.Errorf("응답에 파일명이 없다 — 어느 게시본이 빠졌는지 알 수 없다: %q", body)
		}
		if strings.TrimSpace(body) == "not found" {
			t.Error("사유 없는 평문 — 운영자가 게시 누락인지 판단할 수 없다")
		}
	})

	t.Run("디렉터리면 500 (읽기 실패와 누락을 구분)", func(t *testing.T) {
		if err := os.Mkdir(filepath.Join(dir, "services"), 0o700); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		serveLauncherDistFile(w, "services", "application/json")
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("HTTP %d, want 500 — 누락(404)과 달라야 한다", w.Code)
		}
	})
}

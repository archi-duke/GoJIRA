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

// GET /api/v2/launcher/version — 런처 자동 업데이트(ClickOnce 식) 매니페스트.
// 런처가 기동 시/주기적으로 호출해 자기 버전과 비교하고, 더 높으면
// /api/v2/launcher/download/{os} 로 새 바이너리를 받아 교체한다.
//
// 응답:
//
//	{ "version": "0.2.0",
//	  "platforms": { "windows": {"sha256": "...", "size": 123, "ready": true}, ... } }
//
// version 은 $LAUNCHER_DIST_DIR/version.txt, sha256 은 각 바이너리의 `<file>.sha256` 사이드카에서 읽는다
// (둘 다 build-launcher.sh 가 생성). version.txt 가 없으면 빈 문자열 → 런처는 업데이트 없음으로 간주.
func LauncherVersionHandler(w http.ResponseWriter, r *http.Request) {
	distDir := os.Getenv("LAUNCHER_DIST_DIR")
	if distDir == "" {
		distDir = "/app/launcher-dist"
	}
	ver := ""
	if data, err := os.ReadFile(filepath.Join(distDir, "version.txt")); err == nil {
		ver = strings.TrimSpace(string(data))
	}
	type platInfo struct {
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
		Ready  bool   `json:"ready"`
	}
	platforms := map[string]platInfo{}
	for osKey, plat := range launcherPlatforms {
		path := filepath.Join(distDir, plat.subdir, plat.filename)
		pi := platInfo{}
		if info, err := os.Stat(path); err == nil {
			pi.Ready = true
			pi.Size = info.Size()
			if sha, err := os.ReadFile(path + ".sha256"); err == nil {
				// sha256sum 형식("<hex>  <name>") 또는 hex 단독 모두 허용 — 첫 토큰만.
				if fields := strings.Fields(string(sha)); len(fields) > 0 {
					pi.SHA256 = fields[0]
				}
			}
		}
		platforms[osKey] = pi
	}
	out := map[string]any{"version": ver, "platforms": platforms}
	// 서명 매니페스트가 게시돼 있으면 sig 를 **추가만** 한다 — 기존 필드·형식은 그대로라
	// 서명을 모르는 ≤0.1.38 런처에 영향이 없다(설계 §10.7 구버전 호환).
	if sig, err := os.ReadFile(filepath.Join(distDir, releaseManifestFile+".sig")); err == nil {
		out["sig"] = strings.TrimSpace(string(sig))
	}
	respondJSON(w, http.StatusOK, out)
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


// releaseManifestFile — 루트 키로 서명되는 런처 릴리스 매니페스트 원문 파일명.
// launcher-release 워크플로가 빌드·서명해 launcher-dist 에 게시한다(설계 §10.6).
const releaseManifestFile = "release-manifest.json"

// GET /api/v2/launcher/release-manifest      — 서명 대상 원문(JSON) 그대로
// GET /api/v2/launcher/release-manifest.sig  — base64 ed25519 서명
//
// 서명 검증은 **원문 바이트**에 대해 이뤄지므로 서버가 JSON 을 다시 조립하면 안 된다 —
// 파일을 그대로 흘려보낸다. 미게시(파일 없음)면 404 → 런처는 조용히 기존 경로로 폴백한다.
// 설계서 §10.2 는 이 용도로 `/launcher/manifest` 를 지목했으나 그 경로는 이미
// 웹 설치 모달용(OS별 다운로드 가능 여부)으로 쓰이고 있어 별도 경로로 분리했다.
func LauncherReleaseManifestHandler(w http.ResponseWriter, r *http.Request) {
	serveLauncherDistFile(w, releaseManifestFile, "application/json")
}

func LauncherReleaseManifestSigHandler(w http.ResponseWriter, r *http.Request) {
	serveLauncherDistFile(w, releaseManifestFile+".sig", "text/plain; charset=utf-8")
}

func serveLauncherDistFile(w http.ResponseWriter, name, contentType string) {
	distDir := os.Getenv("LAUNCHER_DIST_DIR")
	if distDir == "" {
		distDir = "/app/launcher-dist"
	}
	data, err := os.ReadFile(filepath.Join(distDir, name))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}


// ── 서비스 카탈로그·패키지 서빙 (설계 §10.2) ────────────────────────────────
//
// 런처는 Platform-API 한 곳만 바라본다(D6). `/launcher/` 는 신원 미들웨어 공개 구간이라
// 인증 없이 서빙하고, 무결성·출처는 카탈로그 서명으로 보장한다.

const catalogFile = "catalog.json"

// GET /api/v2/launcher/catalog      — 서명 대상 원문(JSON) 그대로
// GET /api/v2/launcher/catalog.sig  — base64 ed25519 서명 (카탈로그 키)
func LauncherCatalogHandler(w http.ResponseWriter, r *http.Request) {
	serveLauncherDistFile(w, catalogFile, "application/json")
}

func LauncherCatalogSigHandler(w http.ResponseWriter, r *http.Request) {
	serveLauncherDistFile(w, catalogFile+".sig", "text/plain; charset=utf-8")
}

// GET /api/v2/launcher/services/{name}/{version}/{file} — 서비스 패키지 다운로드.
//
// 경로 세그먼트가 그대로 파일 경로가 되므로 `..` 이나 구분자가 섞인 값을 막는다
// (디렉터리 탈출 방지). sha256 검증은 받는 쪽(런처)이 카탈로그 값으로 수행한다.
func LauncherServicePackageHandler(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")
	file := chi.URLParam(r, "file")
	for _, seg := range []string{name, version, file} {
		if seg == "" || seg == "." || seg == ".." ||
			strings.ContainsAny(seg, `/\`) || strings.Contains(seg, "..") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}
	serveLauncherDistFile(w, filepath.Join("services", name, version, file), "application/zip")
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// UpdateManifest — 백엔드 GET /api/v2/launcher/version 응답.
//
//	{
//	  "version": "0.2.0",
//	  "platforms": {
//	    "windows": {"sha256": "<hex>", "size": 123, "ready": true},
//	    "linux":   {"sha256": "<hex>", "ready": true},
//	    "darwin":  {"ready": false}
//	  }
//	}
//
// 다운로드는 기존 엔드포인트 GET /api/v2/launcher/download/{os} 를 재사용한다.
type UpdateManifest struct {
	Version   string                  `json:"version"`
	Platforms map[string]PlatformInfo `json:"platforms"`
}

type PlatformInfo struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Ready  bool   `json:"ready"`
}

// manifestURL — 설정에서 명시 URL 을 쓰거나, 없으면 backend.platformApi 에서
// /launcher/version 을 유추한다(platformApi 는 이미 .../api/v2 를 포함). 폐쇄망: 사내 백엔드가 서빙.
func (c *Config) manifestURL() string {
	if c.Update.ManifestURL != "" {
		return c.Update.ManifestURL
	}
	base := c.Backend.PlatformAPI
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/launcher/version"
}

// startUpdater — 기동 시 1회(설정 시) + 주기적으로 업데이트를 확인하는 goroutine 을 띄운다.
// 데몬 본 흐름을 막지 않도록 전부 백그라운드. 실제 교체가 일어나면 os.Exit 로 종료
// (자동시작/감시 프로세스가 새 exe 를 다시 띄운다 — Windows 작업 스케줄러/Run 키).
func (s *Server) startUpdater() {
	mURL := s.cfg.manifestURL()
	if mURL == "" {
		log.Printf("자동 업데이트 비활성 — manifestUrl/backend 미설정")
		return
	}
	checkOnStart := s.cfg.Update.CheckOnStart == nil || *s.cfg.Update.CheckOnStart
	interval := s.cfg.Update.IntervalMinutes

	go func() {
		if checkOnStart {
			s.checkAndUpdate(mURL)
		}
		if interval < 0 {
			log.Printf("자동 업데이트 주기 체크 비활성(intervalMinutes<0). manifest=%s", mURL)
			return
		}
		if interval == 0 {
			interval = 60
		}
		t := time.NewTicker(time.Duration(interval) * time.Minute)
		defer t.Stop()
		log.Printf("자동 업데이트 주기 체크 %d분 간격. manifest=%s", interval, mURL)
		for range t.C {
			s.checkAndUpdate(mURL)
		}
	}()
}

// checkAndUpdate — 매니페스트를 받아 현재 버전보다 높으면 새 exe 를 받아 교체한다.
// 실패는 전부 비치명적(로그만) — 다음 주기에 재시도.
func (s *Server) checkAndUpdate(mURL string) {
	// 서명된 릴리스 매니페스트를 먼저 시도한다(§10.6). 성공하면 그 내용으로 업데이트하고,
	// 없거나 검증에 실패하면 경고 모드에서는 기존 /launcher/version 경로로 내려간다(§10.7 A).
	if m, ok := s.signedManifest(mURL); ok {
		s.applyUpdate(m, mURL)
		return
	}
	if signatureMode == "require" {
		log.Printf("서명 매니페스트를 쓸 수 없어 업데이트를 건너뜁니다(필수 모드)")
		return
	}

	m, err := fetchManifest(mURL)
	if err != nil {
		log.Printf("업데이트 체크 실패(manifest): %v", err)
		return
	}
	if !isNewer(m.Version, version) {
		log.Printf("업데이트 없음 — 현재 v%s, 최신 v%s", version, m.Version)
		return
	}
	// 직전에 교체했는데 버전이 오르지 않았던 버전이면 건너뛴다(무한 루프 차단).
	if shouldSkipVersion(m.Version) {
		log.Printf("업데이트 건너뜀 — v%s 는 이전에 교체해도 버전이 오르지 않았습니다 "+
			"(서버 배포본 확인 필요). 더 높은 버전이 올라오면 다시 시도합니다", m.Version)
		return
	}
	pi, ok := m.Platforms[runtime.GOOS]
	if !ok || !pi.Ready {
		log.Printf("업데이트 보류 — 새 v%s 발견했으나 %s 바이너리가 호스팅되지 않음", m.Version, runtime.GOOS)
		return
	}
	log.Printf("새 버전 발견 v%s (현재 v%s) — 다운로드 시작", m.Version, version)

	dlURL := downloadURL(mURL)
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("업데이트 중단 — 실행 파일 경로 확인 실패: %v", err)
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	newPath := exePath + ".new"
	if err := downloadVerify(dlURL, newPath, pi.SHA256); err != nil {
		log.Printf("업데이트 중단 — 다운로드/검증 실패: %v", err)
		_ = os.Remove(newPath)
		return
	}
	log.Printf("새 exe 다운로드/검증 완료: %s — 교체 후 재시작합니다", newPath)

	markUpdateAttempt(m.Version) // 재기동 후 reconcileUpdateState 가 성패를 판정한다
	if err := applySwapAndRestart(exePath, newPath); err != nil {
		log.Printf("업데이트 중단 — 교체 실패: %v", err)
		_ = os.Remove(newPath)
		return
	}
}

// fetchManifest — version.json 을 받아 파싱.
func fetchManifest(url string) (*UpdateManifest, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var m UpdateManifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, err
	}
	if m.Version == "" {
		return nil, fmt.Errorf("매니페스트에 version 없음")
	}
	return &m, nil
}

// downloadURL — manifest URL(.../launcher/version) 과 같은 디렉터리의 기존 다운로드
// 엔드포인트를 OS 키로 조립: .../launcher/version → .../launcher/download/<os>.
func downloadURL(manifestURL string) string {
	dir := manifestURL
	if i := strings.LastIndexByte(dir, '/'); i >= 0 {
		dir = dir[:i] // .../launcher/version → .../launcher
	}
	return dir + "/download/" + runtime.GOOS
}

// downloadVerify — url 을 dst 로 받고 sha256(있으면) 검증. 성공 시 dst 는 실행권한.
func downloadVerify(url, dst, wantSHA string) error {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("다운로드 HTTP %d (%s)", resp.StatusCode, url)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	got := hex.EncodeToString(h.Sum(nil))
	if wantSHA != "" && !strings.EqualFold(got, wantSHA) {
		return fmt.Errorf("sha256 불일치 — 기대 %s, 실제 %s", wantSHA, got)
	}
	return nil
}

// applySwapAndRestart — 실행 중인 exe 는 자기 자신을 덮어쓸 수 없으므로 staged swap.
//
//   - Windows: 현재 exe 를 .old 로 rename → .new 를 본 경로로 rename → 새 exe 를
//     `update-finalize <oldPath>` 로 detached 실행(부모가 .old 삭제 잔여정리)하고 본 프로세스 종료.
//     실행 중 파일은 삭제 불가하지만 rename 은 가능하다(같은 볼륨).
//   - Unix: .new 를 본 경로로 rename(원자적) 후 syscall.Exec 대신 새 프로세스 spawn + 종료.
func applySwapAndRestart(exePath, newPath string) error {
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath) // 이전 잔여 제거(실행중이 아니면 성공)

	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("기존 exe rename 실패: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		// 롤백: 원래 위치 복구.
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("새 exe 배치 실패: %w", err)
	}

	// 새 exe 를 같은 인자로 재기동. update-finalize 로 .old 정리를 맡긴다.
	args := []string{"update-finalize", oldPath}
	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	detach(cmd) // OS별 detached 속성(install_windows.go / install_unix.go)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("새 exe 재기동 실패: %w", err)
	}
	log.Printf("v%s 로 교체 완료 — 새 프로세스 pid=%d. 현재 프로세스를 종료합니다.", filepath.Base(exePath), cmd.Process.Pid)
	os.Exit(0)
	return nil
}

// finalizeUpdate — `gojira-launcher update-finalize <oldPath>` 진입점.
// 새 버전 exe 가 기동 직후 호출 — 이전 .old 파일을 정리(잠깐 잠겨있을 수 있어 재시도)하고
// 그 다음 정상 데몬으로 넘어간다(호출부에서 계속 진행).
func finalizeUpdate(oldPath string) {
	for i := 0; i < 20; i++ {
		if err := os.Remove(oldPath); err == nil || os.IsNotExist(err) {
			log.Printf("업데이트 정리 완료 — 구 exe 제거: %s", oldPath)
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	log.Printf("업데이트 정리 경고 — 구 exe 제거 실패(다음 기동 시 재시도): %s", oldPath)
}

// cleanupStaleArtifacts — 기동 시 남아있을 수 있는 .old/.new 잔여를 정리(베스트에포트).
func cleanupStaleArtifacts() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	for _, suffix := range []string{".old", ".new"} {
		_ = os.Remove(exePath + suffix)
	}
}

// isNewer — semver 비교(단순). a 가 b 보다 높으면 true. 비교 불가 시 문자열 불일치로 판단.
func isNewer(a, b string) bool {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var out [3]int
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		// "2-rc1" 같은 꼬리 제거.
		num := parts[i]
		for j := 0; j < len(num); j++ {
			if num[j] < '0' || num[j] > '9' {
				num = num[:j]
				break
			}
		}
		out[i], _ = strconv.Atoi(num)
	}
	return out
}


// ── 서명 릴리스 매니페스트 (§10.6, §10.7) ──────────────────────────────────

// signedManifest — `/launcher/release-manifest` 원문과 `.sig` 를 받아 검증한다.
// 성공 시 (UpdateManifest, true). 미존재·검증 실패면 (nil, false) — 호출자가 모드에 따라 처리.
func (s *Server) signedManifest(mURL string) (*UpdateManifest, bool) {
	base := mURL
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[:i] // .../launcher/version → .../launcher
	}
	raw, err := fetchBytes(base + "/release-manifest")
	if err != nil {
		return nil, false // 구버전 서버 — 경고 없이 조용히 폴백
	}
	sigRaw, err := fetchBytes(base + "/release-manifest.sig")
	if err != nil {
		log.Printf("⚠️  릴리스 매니페스트는 있으나 서명(.sig)을 받지 못했습니다: %v", err)
		sigRaw = nil
	}
	rm, err := parseReleaseManifest(raw)
	if err != nil {
		log.Printf("⚠️  릴리스 매니페스트 해석 실패: %v", err)
		return nil, false
	}
	ok, err := acceptManifest(raw, string(sigRaw), rm, s.lastManifestIssuedAt())
	if err != nil {
		log.Printf("업데이트 중단 — %v", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	if rm.IssuedAt != "" {
		s.saveManifestIssuedAt(rm.IssuedAt)
	}
	return &UpdateManifest{Version: rm.Version, Platforms: rm.Platforms}, true
}

// applyUpdate — 매니페스트 기준으로 버전 비교 후 내려받아 교체한다(서명·구형식 공통 경로).
func (s *Server) applyUpdate(m *UpdateManifest, mURL string) {
	if !isNewer(m.Version, version) {
		log.Printf("업데이트 없음 — 현재 v%s, 최신 v%s", version, m.Version)
		return
	}
	// 직전에 교체했는데 버전이 오르지 않았던 버전이면 건너뛴다(무한 루프 차단).
	if shouldSkipVersion(m.Version) {
		log.Printf("업데이트 건너뜀 — v%s 는 이전에 교체해도 버전이 오르지 않았습니다 "+
			"(서버 배포본 확인 필요). 더 높은 버전이 올라오면 다시 시도합니다", m.Version)
		return
	}
	pi, ok := m.Platforms[runtime.GOOS]
	if !ok || !pi.Ready {
		log.Printf("업데이트 보류 — 새 v%s 발견했으나 %s 바이너리가 호스팅되지 않음", m.Version, runtime.GOOS)
		return
	}
	log.Printf("새 버전 발견 v%s (현재 v%s) — 다운로드 시작", m.Version, version)

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("업데이트 중단 — 실행 파일 경로 확인 실패: %v", err)
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	newPath := exePath + ".new"
	if err := downloadVerify(downloadURL(mURL), newPath, pi.SHA256); err != nil {
		log.Printf("업데이트 중단 — 다운로드/검증 실패: %v", err)
		_ = os.Remove(newPath)
		return
	}
	log.Printf("새 exe 다운로드/검증 완료: %s — 교체 후 재시작합니다", newPath)
	markUpdateAttempt(m.Version) // 재기동 후 reconcileUpdateState 가 성패를 판정한다
	if err := applySwapAndRestart(exePath, newPath); err != nil {
		log.Printf("업데이트 중단 — 교체 실패: %v", err)
		_ = os.Remove(newPath)
	}
}

// fetchBytes — 작은 텍스트 리소스(매니페스트·서명)를 받는다. 404 등은 error.
func fetchBytes(url string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// lastManifestIssuedAt / saveManifestIssuedAt — 재전송·다운그레이드 방지용 수용 기록(§10.6).
// 실패는 무해 — 기록이 없으면 역행 검사를 건너뛴다.
func (s *Server) lastManifestIssuedAt() string {
	dir, err := gojiraDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, manifestStateFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (s *Server) saveManifestIssuedAt(issuedAt string) {
	dir, err := gojiraDir()
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, manifestStateFile), []byte(issuedAt+"\n"), 0o600)
}

const manifestStateFile = ".launcher-manifest-issued"

// ── 업데이트 실패 기억 (무한 루프 차단) ─────────────────────────────────────
//
// 실제 사고: 서버의 version.txt 는 0.1.39 인데 실제 바이너리가 0.1.38 이었다.
// 런처는 "새 버전 발견 → 다운로드 → sha256 통과 → 교체 → 재시작" 을 끝내지만
// 재시작한 자기 버전이 그대로라, 다음 체크에서 또 같은 버전을 발견해 영원히 반복했다.
// 1초 간격으로 다운로드가 반복되며 로그가 가득 찼다.
//
// 서버가 어떤 상태든 클라이언트는 스스로를 보호해야 한다:
//   업데이트 직전 목표 버전을 pending 으로 적고, 재기동 시 자기 버전이 그만큼
//   오르지 않았으면 그 버전을 failed 로 기록해 다시 시도하지 않는다.
//   더 높은 새 버전이 나오면 그때 다시 시도한다.

const updateStateFile = ".launcher-update-state"

type updateState struct {
	Pending string `json:"pending,omitempty"` // 교체를 시도한 목표 버전
	Failed  string `json:"failed,omitempty"`  // 올라가지 않아 건너뛸 버전
}

func loadUpdateState() updateState {
	var st updateState
	dir, err := gojiraDir()
	if err != nil {
		return st
	}
	b, err := os.ReadFile(filepath.Join(dir, updateStateFile))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(b, &st)
	return st
}

func saveUpdateState(st updateState) {
	dir, err := gojiraDir()
	if err != nil {
		return
	}
	// 최초 실행이면 ~/.gojira 가 아직 없을 수 있다 — 없으면 기록이 조용히 사라져
	// 루프 차단이 동작하지 않는다.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, updateStateFile), b, 0o600)
}

// reconcileUpdateState — 기동 시 1회. 직전 교체 시도의 성패를 판정한다.
func reconcileUpdateState() {
	st := loadUpdateState()
	if st.Pending == "" {
		return
	}
	if !isNewer(st.Pending, version) {
		// 목표 버전 이상으로 올라왔다 — 성공.
		log.Printf("업데이트 성공 확인 — v%s", version)
		saveUpdateState(updateState{})
		return
	}
	// 교체했는데 버전이 그대로다. 서버 매니페스트와 실제 바이너리가 어긋난 상태로 보인다.
	log.Printf("⚠️  업데이트 실패 — v%s 로 교체했으나 실행 버전은 v%s 입니다. "+
		"이 버전은 더 시도하지 않습니다(서버 배포본 확인 필요)", st.Pending, version)
	saveUpdateState(updateState{Failed: st.Pending})
}

// shouldSkipVersion — 이전에 실패한 버전이면 건너뛴다.
func shouldSkipVersion(v string) bool {
	return loadUpdateState().Failed == v
}

// markUpdateAttempt — 교체 직전에 목표 버전을 기록한다.
func markUpdateAttempt(v string) {
	st := loadUpdateState()
	st.Pending = v
	saveUpdateState(st)
}

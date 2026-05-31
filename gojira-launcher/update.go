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
	m, err := fetchManifest(mURL)
	if err != nil {
		log.Printf("업데이트 체크 실패(manifest): %v", err)
		return
	}
	if !isNewer(m.Version, version) {
		log.Printf("업데이트 없음 — 현재 v%s, 최신 v%s", version, m.Version)
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

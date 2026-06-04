package main

// 의존성 프로비저닝 — 스킬이 사용하는 외부 CLI(jq, python3)를 클라이언트 PC 에 확보한다.
//
// 배포 환경 가정: 인터넷은 되지만 일부 사이트가 막힐 수 있음(완전 폐쇄망 아님).
// 따라서 동봉(embed)이 아니라 "검사 → 없으면 공식 릴리스 직접 다운로드 → 실패 시 엄격 중단".
//
// 산출물은 모두 런처 관리 디렉터리 ~/.gojira/bin 에 떨어지고, spawn.go 의 agentEnv 가
// 이 디렉터리를 에이전트 PATH 맨 앞에 주입한다(시스템 PATH·관리자 권한 불필요).
//   - jq      : 공식 단일 바이너리(jqlang/jq 릴리스)를 받아 bin/jq[.exe]
//   - python3 : python3 가 이미 있으면 노옵. python 만 있으면 bin/python3 shim(POSIX) 생성
//               (스킬은 `python3` 호출을 그대로 유지). 둘 다 없으면 공식 설치 프로그램으로 설치.

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	jqVersion     = "1.7.1"
	pythonVersion = "3.12.7"
)

// EnsureDependencies — install 시 호출. jq·python 을 확보하지 못하면 에러를 돌려
// 호출부(doInstall)가 설치를 중단한다(엄격).
func EnsureDependencies(binDir string) error {
	fmt.Println("의존성 검사: jq, python …")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("%s 생성 실패: %w", binDir, err)
	}
	if err := ensureJQ(binDir); err != nil {
		return fmt.Errorf("jq 확보 실패: %w", err)
	}
	if err := ensurePython(binDir); err != nil {
		return fmt.Errorf("python 확보 실패: %w", err)
	}
	fmt.Println("✓ 의존성 확보 완료 (jq, python)")
	return nil
}

// DepsHealth — 데몬 기동/health 용 상태 요약(부수효과 없음, 비치명적).
func DepsHealth() map[string]bool {
	binDir, _ := gojiraBinDir()
	jqOK := hasWorkingJQ()
	if !jqOK && binDir != "" {
		jqOK = runOK(filepath.Join(binDir, jqExeName()), "--version")
	}
	pyOK := pythonWorks("python3") || pythonWorks("python")
	if !pyOK && binDir != "" {
		pyOK = fileExists(filepath.Join(binDir, "python3"))
	}
	return map[string]bool{"jq": jqOK, "python": pyOK}
}

// ── jq ────────────────────────────────────────────────────────────────────

func jqExeName() string {
	if runtime.GOOS == "windows" {
		return "jq.exe"
	}
	return "jq"
}

func hasWorkingJQ() bool {
	p, err := exec.LookPath("jq")
	if err != nil {
		return false
	}
	return runOK(p, "--version")
}

// jqDownloadURL — 플랫폼별 공식 릴리스 단일 바이너리 URL.
func jqDownloadURL() (string, error) {
	base := "https://github.com/jqlang/jq/releases/download/jq-" + jqVersion + "/"
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64":
		return base + "jq-windows-amd64.exe", nil
	case "windows/arm64":
		return base + "jq-windows-amd64.exe", nil // arm64 윈도우는 x64 에뮬레이션으로 동작
	case "linux/amd64":
		return base + "jq-linux-amd64", nil
	case "linux/arm64":
		return base + "jq-linux-arm64", nil
	case "darwin/amd64":
		return base + "jq-macos-amd64", nil
	case "darwin/arm64":
		return base + "jq-macos-arm64", nil
	}
	return "", fmt.Errorf("지원하지 않는 플랫폼: %s/%s", runtime.GOOS, runtime.GOARCH)
}

func ensureJQ(binDir string) error {
	if hasWorkingJQ() {
		fmt.Println("  ✓ jq 이미 설치됨 (PATH)")
		return nil
	}
	target := filepath.Join(binDir, jqExeName())
	if runOK(target, "--version") {
		fmt.Printf("  ✓ jq 이미 설치됨: %s\n", target)
		return nil
	}
	url, err := jqDownloadURL()
	if err != nil {
		return err
	}
	fmt.Printf("• jq 가 없습니다 — 설치가 필요합니다. 다운로드 시도: %s\n", url)
	if err := downloadFile(url, target, 0o755); err != nil {
		return fmt.Errorf("다운로드 실패(접속 차단/네트워크 오류일 수 있음): %w", err)
	}
	if !runOK(target, "--version") {
		_ = os.Remove(target)
		return fmt.Errorf("다운로드한 jq 실행 검증 실패: %s", target)
	}
	fmt.Printf("  ✓ jq 설치: %s\n", target)
	return nil
}

// ── python ──────────────────────────────────────────────────────────────────

// pythonWorks — 해당 명령이 실제 작동하는 인터프리터인지(WindowsApps Store 스텁 배제).
// 스텁은 비대화 실행 시 아무것도 출력하지 않으므로 마커 출력으로 진짜 여부를 가린다.
func pythonWorks(cmd string) bool {
	out, err := runCapture(cmd, "-c", "import sys; sys.stdout.write('PYOK')")
	return err == nil && strings.Contains(out, "PYOK")
}

func ensurePython(binDir string) error {
	if pythonWorks("python3") {
		fmt.Println("  ✓ python3 이미 설치됨 (PATH)")
		return nil
	}
	if pythonWorks("python") {
		fmt.Println("  • python3 는 없고 python 만 있음 — python3 shim 생성")
		return writePython3Shim(binDir, "python")
	}
	fmt.Println("• python 이 없습니다 — 설치가 필요합니다. 다운로드·설치 시도…")
	if err := downloadAndInstallPython(binDir); err != nil {
		return err
	}
	// 설치 후 재검증 — 현재 프로세스 PATH 가 갱신되지 않을 수 있어 절대경로까지 탐색.
	if pythonWorks("python3") {
		return nil
	}
	if pythonWorks("python") {
		return writePython3Shim(binDir, "python")
	}
	if abs := findInstalledWindowsPython(); abs != "" && runOK(abs, "-c", "import sys") {
		return writePython3Shim(binDir, abs)
	}
	return fmt.Errorf("설치했으나 실행 검증 실패 — 수동 설치 후 재시도하세요")
}

// writePython3Shim — bin/python3 에 POSIX 셸 shim 을 만든다. 에이전트 Bash(Git Bash/MSYS,
// 또는 unix sh)에서 bare `python3` 호출이 이 파일로 해석되어 실제 python 을 부른다.
// target 이 "python" 이면 PATH 의존, 절대경로면 그 경로를 직접 실행.
func writePython3Shim(binDir, target string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	var line string
	if target == "python" {
		line = `exec python "$@"`
	} else {
		line = `exec "` + filepath.ToSlash(target) + `" "$@"`
	}
	shim := filepath.Join(binDir, "python3")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\n"+line+"\n"), 0o755); err != nil {
		return err
	}
	fmt.Printf("  ✓ python3 shim 생성: %s\n", shim)
	return nil
}

// downloadAndInstallPython — Windows 한정 공식 설치 프로그램 무인 설치(per-user).
// 다른 OS 는 자동 설치 대신 안내 후 에러(거의 python3 가 기본 제공됨).
func downloadAndInstallPython(binDir string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("이 OS 에선 python 자동 설치 미지원 — 수동 설치 후 재시도하세요")
	}
	url := fmt.Sprintf("https://www.python.org/ftp/python/%s/python-%s-amd64.exe", pythonVersion, pythonVersion)
	installer := filepath.Join(binDir, "python-installer.exe")
	fmt.Printf("  python %s 설치 프로그램 다운로드: %s\n", pythonVersion, url)
	if err := downloadFile(url, installer, 0o755); err != nil {
		return fmt.Errorf("설치 프로그램 다운로드 실패(접속 차단/네트워크 오류일 수 있음): %w", err)
	}
	defer os.Remove(installer)
	fmt.Println("  python 무인 설치 진행(per-user, PATH 추가)…")
	// /quiet 무인, per-user(관리자 불필요), PATH 추가, 테스트 스위트 제외.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, installer,
		"/quiet", "InstallAllUsers=0", "PrependPath=1", "Include_test=0", "Include_launcher=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("설치 실행 실패: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findInstalledWindowsPython — 무인 설치 후 PATH 미갱신 대비, 표준 per-user 설치 경로에서
// python.exe 를 찾아 절대경로를 돌려준다.
func findInstalledWindowsPython() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	local := os.Getenv("LocalAppData")
	if local == "" {
		return ""
	}
	root := filepath.Join(local, "Programs", "Python")
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "Python3") {
			continue
		}
		cand := filepath.Join(root, e.Name(), "python.exe")
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

// ── 공통 유틸 ────────────────────────────────────────────────────────────────

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// runOK — 명령을 실행해 종료코드 0 이면 true (10초 타임아웃).
func runOK(name string, args ...string) bool {
	if name == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run() == nil
}

// runCapture — 명령 stdout 캡처(10초 타임아웃).
func runCapture(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// downloadFile — url 을 dest 로 받는다(임시파일 후 rename, 리다이렉트 자동 추종).
func downloadFile(url, dest string, mode os.FileMode) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".download"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// logDepsStatus — 데몬 기동 시 1회 의존성 상태 로깅(비치명적).
func logDepsStatus() {
	st := DepsHealth()
	log.Printf("  의존성: jq=%v python=%v (미충족 시 `gojira-launcher install` 재실행)", st["jq"], st["python"])
}

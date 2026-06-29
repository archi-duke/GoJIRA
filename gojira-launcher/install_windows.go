//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// killProcessTree — 콘솔(cmd)과 그 자식(claude/opencode)까지 트리 종료. 과제전환 시 옛 에이전트 정리.
// (새 콘솔 spawn 은 spawn_windows.go 의 CreateProcess 가 담당 — 거기서 받은 PID 를 여기 넘긴다.)
func killProcessTree(pid int) error {
	out, err := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		s := strings.ToLower(string(out))
		// 이미 종료/부재면 성공으로 간주(영문 "not found"/한글 "없").
		if strings.Contains(s, "not found") || strings.Contains(s, "no running") || strings.Contains(string(out), "없") {
			return nil
		}
		return fmt.Errorf("taskkill pid=%d: %s (%w)", pid, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Windows 자동시작은 HKCU Run 키 사용 — 관리자 권한 불필요, 로그인 시 자동 실행.
const (
	runKeyPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	runKeyName = "GojiraLauncher"
)

// detached 프로세스 생성 플래그 — 콘솔 분리 + 새 프로세스 그룹 + 창 숨김.
const (
	flagDetachedProcess     = 0x00000008
	flagCreateNewProcGroup  = 0x00000200
	flagCreateNoWindow      = 0x08000000
	detachedCreationFlags   = flagDetachedProcess | flagCreateNewProcGroup | flagCreateNoWindow
)

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "AppData", "Local")
		}
	}
	return filepath.Join(base, "GoJIRA")
}

func exeName() string { return "gojira-launcher.exe" }

// registerAutostart — HKCU Run 키에 등록(reg.exe). 따옴표로 경로 감쌈.
func registerAutostart(exe string) error {
	cmd := exec.Command("reg", "add", runKeyPath, "/v", runKeyName, "/t", "REG_SZ",
		"/d", fmt.Sprintf("\"%s\"", exe), "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reg add: %s (%w)", string(out), err)
	}
	return nil
}

func unregisterAutostart() error {
	cmd := exec.Command("reg", "delete", runKeyPath, "/v", runKeyName, "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("reg delete: %s (%w)", string(out), err)
	}
	return nil
}

// startDetached — 설치 후 데몬을 detached 로 기동(설치 콘솔이 닫혀도 유지).
func startDetached(exe string) error {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedCreationFlags}
	return cmd.Start()
}

// detach — 자동 업데이트 재기동 시 새 프로세스를 부모와 분리(update.go 에서 호출).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flagCreateNewProcGroup}
}

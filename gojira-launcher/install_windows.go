//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

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

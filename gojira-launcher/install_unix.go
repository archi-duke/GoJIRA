//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Unix(Linux/macOS) — 폐쇄망 주 타깃은 Windows 이므로 자동시작은 최소 구현:
// Linux 는 ~/.config/autostart 의 .desktop 엔트리, 그 외(mac)는 수동 안내.

func installDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/usr/local/lib/gojira"
	}
	return filepath.Join(home, ".local", "share", "gojira")
}

func exeName() string { return "gojira-launcher" }

func registerAutostart(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	autostartDir := filepath.Join(home, ".config", "autostart")
	if err := os.MkdirAll(autostartDir, 0o755); err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=GoJIRA Launcher
Exec=%s
X-GNOME-Autostart-enabled=true
NoDisplay=true
`, exe)
	return os.WriteFile(filepath.Join(autostartDir, "gojira-launcher.desktop"), []byte(desktop), 0o644)
}

func unregisterAutostart() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	p := filepath.Join(home, ".config", "autostart", "gojira-launcher.desktop")
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func startDetached(exe string) error {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// detach — 자동 업데이트 재기동 시 새 세션으로 분리(update.go 에서 호출).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

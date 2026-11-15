//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

// Unix(Linux/macOS) — 폐쇄망 주 타깃은 Windows 이므로 자동시작은 최소 구현:
// Linux 는 ~/.config/autostart 의 .desktop 엔트리, macOS 는 ~/Library/LaunchAgents 의
// LaunchAgent plist.
//
// 이전에는 macOS 에서도 .desktop 을 썼다. macOS 에 ~/.config/autostart 를 읽는 것이
// 없으니 **파일만 생기고 자동시작은 되지 않는데 nil 을 반환해 성공으로 보고**됐다.

func installDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/usr/local/lib/gojira"
	}
	return filepath.Join(home, ".local", "share", "gojira")
}

func exeName() string { return "gojira-launcher" }

const launchAgentLabel = "com.gojira.launcher"

// launchAgentPath — macOS 사용자 LaunchAgent plist 경로.
func launchAgentPath(home string) string {
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
}

func registerAutostart(exe string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if runtime.GOOS == "darwin" {
		dir := filepath.Join(home, "Library", "LaunchAgents")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// RunAtLoad 로 로그인 시 기동, KeepAlive 는 두지 않는다 — 사용자가 끈 런처를
		// 되살리면 종료할 방법이 없다.
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
  <key>ProcessType</key><string>Interactive</string>
</dict>
</plist>
`, launchAgentLabel, exe)
		path := launchAgentPath(home)
		if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
			return err
		}
		// 지금 세션에도 등록한다. 실패해도 다음 로그인에는 뜨므로 치명적이지 않다.
		if out, err := exec.Command("launchctl", "load", "-w", path).CombinedOutput(); err != nil {
			// 이미 등록된 경우도 에러로 오므로 로그만 남긴다.
			fmt.Printf("  launchctl load 경고: %v (%s)\n", err, strings.TrimSpace(string(out)))
		}
		return nil
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

	if runtime.GOOS == "darwin" {
		path := launchAgentPath(home)
		// unload 를 먼저 — 파일을 지운 뒤에는 launchctl 이 찾지 못한다.
		_ = exec.Command("launchctl", "unload", "-w", path).Run()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
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

// killProcessTree — pid 종료(과제전환 시 옛 에이전트 정리). dev 한정 경로.
func killProcessTree(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	_ = p.Kill()
	return nil
}

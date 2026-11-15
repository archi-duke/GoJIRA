//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// macOS 자동시작은 LaunchAgent plist 여야 한다.
// 이전에는 Linux 용 .desktop 을 쓰고 nil 을 반환해, 자동시작이 안 되는데도
// 성공으로 보고됐다.
func TestRegisterAutostartDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin 전용 경로")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	exe := filepath.Join(home, "bin", "gojira-launcher")
	if err := registerAutostart(exe); err != nil {
		t.Fatalf("registerAutostart: %v", err)
	}

	// .desktop 을 쓰지 않아야 한다
	if _, err := os.Stat(filepath.Join(home, ".config", "autostart", "gojira-launcher.desktop")); err == nil {
		t.Error("Linux .desktop 을 만들었다 — macOS 에서는 효과가 없다")
	}

	plist := launchAgentPath(home)
	b, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("LaunchAgent plist 가 없다: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		"<key>Label</key><string>com.gojira.launcher</string>",
		"<key>RunAtLoad</key><true/>",
		exe,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist 에 %q 가 없다\n---\n%s", want, got)
		}
	}
	// KeepAlive 는 두지 않는다 — 사용자가 끈 런처를 되살리면 끌 방법이 없다
	if strings.Contains(got, "KeepAlive") {
		t.Error("KeepAlive 가 있다 — 사용자가 런처를 종료할 수 없게 된다")
	}
	// plist 파싱이 되는지 확인
	if out, err := runPlutil(plist); err != nil {
		t.Errorf("plutil 검증 실패: %v\n%s", err, out)
	}

	// 해제가 파일을 지우는지
	if err := unregisterAutostart(); err != nil {
		t.Fatalf("unregisterAutostart: %v", err)
	}
	if _, err := os.Stat(plist); !os.IsNotExist(err) {
		t.Error("plist 가 남아 있다")
	}
}

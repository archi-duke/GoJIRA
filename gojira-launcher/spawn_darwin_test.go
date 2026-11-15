//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeSpawnScript — 값에 따옴표·공백이 섞여도 셸이 한 단어로 받아야 한다.
// 이전 osascript 경로는 AppleScript 안에 셸 명령을 중첩해 이게 깨졌다.
func TestWriteSpawnScript(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin 전용 경로")
	}
	env := []string{
		"SIMPLE=value",
		"WITH_SPACE=a b c",
		"WITH_QUOTE=it's here",
		"TOKEN=abc'def\"ghi",
	}
	path, err := writeSpawnScript("/usr/bin/true", "/tmp/dir with space", "GoJIRA · TEST", env)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)

	if !strings.HasPrefix(got, "#!/bin/sh\n") {
		t.Error("shebang 누락")
	}
	for _, want := range []string{
		`export SIMPLE='value'`,
		`export WITH_SPACE='a b c'`,
		`cd '/tmp/dir with space' || exit 1`,
		`exec '/usr/bin/true'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("스크립트에 %q 가 없다\n---\n%s", want, got)
		}
	}
	// 따옴표가 섞인 값 — '\'' 로 끊어 붙여야 셸이 한 단어로 받는다
	if !strings.Contains(got, `export WITH_QUOTE='it'\''s here'`) {
		t.Errorf("따옴표 이스케이프 실패\n---\n%s", got)
	}

	// 권한 0700 — env 에 토큰이 섞일 수 있다
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := fi.Mode().Perm(); mode != 0o700 {
		t.Errorf("권한 %o, want 700 — 토큰이 다른 사용자에게 읽힌다", mode)
	}

	// 실제로 셸이 파싱할 수 있어야 한다
	if out, err := runSh("-n", path); err != nil {
		t.Errorf("sh -n 문법 검사 실패: %v\n%s", err, out)
	}
}

// terminalCommand — darwin 은 osascript 가 아니라 open 을 써야 한다.
// osascript 는 Apple Event 권한에 막히면 -1743 으로 실패하는데, 그걸 호출자가 알 수 없다.
func TestTerminalCommandUsesOpen(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin 전용 경로")
	}
	c := terminalCommand("/usr/bin/true", "/tmp", "label", []string{"K=V"})
	base := filepath.Base(c.Path)
	if base == "osascript" {
		t.Fatal("osascript 를 쓰고 있다 — Apple Event 권한에 막히면 조용히 실패한다")
	}
	if base != "open" {
		t.Fatalf("실행기가 %q — open 이어야 한다", base)
	}
	joined := strings.Join(c.Args, " ")
	if !strings.Contains(joined, "-a Terminal") {
		t.Errorf("args=%v — -a Terminal 이 없다", c.Args)
	}
	// 마지막 인자는 생성된 스크립트 경로
	last := c.Args[len(c.Args)-1]
	if !strings.HasSuffix(last, ".sh") {
		t.Errorf("마지막 인자가 스크립트가 아니다: %q", last)
	}
	os.Remove(last)
}

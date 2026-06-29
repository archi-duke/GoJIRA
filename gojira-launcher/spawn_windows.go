//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Windows 에이전트 spawn — 새 콘솔을 직접 생성한다.
//
// 왜 exec.Cmd 가 아니라 raw CreateProcess 인가:
//   - exec.Cmd 는 Stdin/Stdout/Stderr 가 nil 이면 NUL 장치로 연결한다(STARTF_USESTDHANDLES).
//     CREATE_NEW_CONSOLE 를 줘도 표준핸들이 NUL 이면 콘솔 창만 뜨고 에이전트(특히 opencode
//     TUI)가 입출력을 못 해 "창은 떠도 도구가 안 도는" 증상이 난다.
//   - 과거 `cmd /c start ... cmd /k` 는 인터랙티브했지만 start 가 콘솔을 띄우자마자 부모가
//     종료돼 우리가 추적하는 PID 가 즉사하는 껍데기였다(→ 과제전환 시 종료 불가).
//   - CreateProcess 를 STARTF_USESTDHANDLES 없이 호출하면 새 콘솔이 표준핸들을 제공해
//     인터랙티브 + 실제 PID 추적이 동시에 된다.
const (
	createNewConsole    = 0x00000010
	createUnicodeEnv    = 0x00000400
	waitInfinite        = 0xFFFFFFFF
)

// startAgent — bin 을 새 콘솔에서 띄운다. (pid, 종료대기함수, err) 반환.
func startAgent(bin, label, dir string, env []string) (int, func(), error) {
	title := sanitizeLabel(label)
	if title == "" {
		title = "GoJIRA"
	}
	// 새 콘솔에서: 제목 설정 → UTF-8 코드페이지(한국어 Windows CP949 의 U+FFFD 깨짐 방지)
	// → 에이전트 실행. 우리가 raw 커맨드라인을 직접 만들므로 cmd 의 따옴표 스트립 함정이 없다.
	inner := fmt.Sprintf("title %s&&chcp 65001>nul&&%s", title, bin)
	cmdline := "cmd /k " + inner

	envBlock, err := makeEnvBlock(dedupEnv(append(os.Environ(), env...)))
	if err != nil {
		return 0, nil, err
	}
	cmdp, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return 0, nil, err
	}
	var dirp *uint16
	if dir != "" {
		if dirp, err = syscall.UTF16PtrFromString(dir); err != nil {
			return 0, nil, err
		}
	}
	si := new(syscall.StartupInfo)
	si.Cb = uint32(unsafe.Sizeof(*si))
	pi := new(syscall.ProcessInformation)
	if err := syscall.CreateProcess(nil, cmdp, nil, nil, false,
		uint32(createNewConsole|createUnicodeEnv), envBlock, dirp, si, pi); err != nil {
		return 0, nil, err
	}
	_ = syscall.CloseHandle(pi.Thread)
	proc := pi.Process
	wait := func() {
		_, _ = syscall.WaitForSingleObject(proc, waitInfinite)
		_ = syscall.CloseHandle(proc)
	}
	return int(pi.ProcessId), wait, nil
}

// makeEnvBlock — []string("K=V") 를 CreateProcess 용 UTF-16 이중널종료 환경 블록으로.
func makeEnvBlock(env []string) (*uint16, error) {
	var b []uint16
	for _, e := range env {
		if e == "" {
			continue
		}
		u, err := syscall.UTF16FromString(e) // 끝에 NUL 포함
		if err != nil {
			return nil, err
		}
		b = append(b, u...)
	}
	b = append(b, 0) // 이중널 종료
	if len(b) < 2 {
		b = []uint16{0, 0}
	}
	return &b[0], nil
}

// dedupEnv — 같은 키(대소문자 무시)는 마지막 값 유지(주입 env 가 os.Environ 을 덮어쓰도록).
func dedupEnv(env []string) []string {
	seen := map[string]int{}
	out := make([]string, 0, len(env))
	for _, e := range env {
		k := e
		if i := strings.IndexByte(e, '='); i >= 0 {
			k = strings.ToUpper(e[:i])
		}
		if idx, ok := seen[k]; ok {
			out[idx] = e
		} else {
			seen[k] = len(out)
			out = append(out, e)
		}
	}
	return out
}

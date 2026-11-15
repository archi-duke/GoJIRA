//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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

const (
	synchronize                    = 0x00100000
	processQueryLimitedInformation = 0x00001000
	terminalPidTimeout             = 20 * time.Second
)

// startAgent — bin 을 새 창에서 띄운다. (pid, 종료대기함수, err) 반환.
// Windows Terminal(wt.exe)이 있으면 그 새 창에서, 없으면(또는 wt 기동 자체가 실패하면)
// 레거시 콘솔(conhost) PowerShell 창으로 폴백한다. conhost 는 에이전트 TUI 의 한글/박스문자가
// 깨지므로 Windows Terminal 을 우선한다.
func startAgent(bin, label, dir string, env []string) (int, func(), error) {
	title := sanitizeLabel(label)
	if title == "" {
		title = "GoJIRA"
	}
	if wt, err := exec.LookPath("wt.exe"); err == nil {
		pid, wait, launched, err := startAgentInTerminal(wt, bin, title, dir, env)
		if launched || err == nil {
			return pid, wait, err
		}
		log.Printf("Windows Terminal 기동 실패 — 콘솔 창으로 폴백: %v", err)
	}
	return startAgentConsole(bin, title, dir, env)
}

// startAgentInTerminal — Windows Terminal 새 창의 PowerShell 에서 에이전트를 띄운다.
//
// wt.exe 는 실행 중인 Windows Terminal 에 요청만 넘기고 즉시 종료하므로
//   - 우리 env 블록이 새 탭으로 전달되지 않는다 → 주입 env 를 임시 .ps1 에 $env: 대입으로 기록.
//   - 실제 셸 PID 를 알 수 없다 → .ps1 이 자기 $PID 를 파일에 쓰고, 그걸 읽어 OpenProcess 로 추적.
//     (세션 레지스트리·과제전환 killProcessTree 가 이 PID 로 동작)
// launched=true 면 wt 는 떴으므로 호출측이 콘솔 폴백(=창 2개)을 하지 않아야 한다.
func startAgentInTerminal(wt, bin, title, dir string, env []string) (int, func(), bool, error) {
	stamp := time.Now().UnixNano()
	pidFile := filepath.Join(os.TempDir(), fmt.Sprintf("gojira-agent-%d.pid", stamp))
	scriptFile := filepath.Join(os.TempDir(), fmt.Sprintf("gojira-agent-%d.ps1", stamp))

	var b strings.Builder
	for _, kv := range dedupEnv(env) {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		fmt.Fprintf(&b, "${env:%s} = %s\r\n", kv[:i], psQuote(kv[i+1:]))
	}
	fmt.Fprintf(&b, "Set-Content -LiteralPath %s -Value $PID -Encoding ascii\r\n", psQuote(pidFile))
	fmt.Fprintf(&b, "Remove-Item -LiteralPath %s -ErrorAction SilentlyContinue\r\n", psQuote(scriptFile))
	b.WriteString("chcp 65001 > $null\r\n")
	b.WriteString("$u = New-Object System.Text.UTF8Encoding $false\r\n")
	b.WriteString("[Console]::InputEncoding = $u; [Console]::OutputEncoding = $u; $OutputEncoding = $u\r\n")
	if dir != "" {
		fmt.Fprintf(&b, "Set-Location -LiteralPath %s\r\n", psQuote(dir))
	}
	fmt.Fprintf(&b, "& %s\r\n", psQuote(bin))

	// UTF-8 BOM — Windows PowerShell 5.1 은 BOM 없는 .ps1 을 ANSI(CP949)로 읽어 한글 경로가 깨진다.
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte(b.String())...)
	if err := os.WriteFile(scriptFile, content, 0o600); err != nil {
		return 0, nil, false, fmt.Errorf("에이전트 스크립트 생성 실패: %w", err)
	}

	args := []string{"-w", "new", "new-tab", "--title", title, "--suppressApplicationTitle"}
	if dir != "" {
		args = append(args, "-d", dir)
	}
	args = append(args, "powershell.exe", "-NoLogo", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", scriptFile)
	cmd := exec.Command(wt, args...)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptFile)
		return 0, nil, false, err
	}
	go func() { _ = cmd.Wait() }()

	deadline := time.Now().Add(terminalPidTimeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				_ = os.Remove(pidFile)
				h, err := syscall.OpenProcess(synchronize|processQueryLimitedInformation, false, uint32(pid))
				if err != nil {
					return 0, nil, true, fmt.Errorf("에이전트 프로세스(pid=%d) 열기 실패: %w", pid, err)
				}
				wait := func() {
					_, _ = syscall.WaitForSingleObject(h, waitInfinite)
					_ = syscall.CloseHandle(h)
				}
				return pid, wait, true, nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	// 스크립트가 끝내 실행되지 않았다면 주입 env(세션 토큰 포함)가 평문으로 남지 않게 정리.
	_ = os.Remove(scriptFile)
	return 0, nil, true, fmt.Errorf("Windows Terminal 에서 에이전트 시작 확인 실패(%s 내 PID 미보고)", terminalPidTimeout)
}

// psQuote — PowerShell 작은따옴표 리터럴(내부 ' 는 '' 로 이스케이프).
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// startAgentConsole — 레거시 콘솔(conhost) 폴백.
func startAgentConsole(bin, title, dir string, env []string) (int, func(), error) {
	// 새 PowerShell 콘솔에서: 제목 설정 → UTF-8 코드페이지·콘솔 인코딩(한국어 Windows CP949 의
	// U+FFFD 깨짐 방지) → 에이전트 실행. -NoExit 로 에이전트 종료 후에도 셸이 남는다(구 cmd /k 동작).
	// -ExecutionPolicy Bypass: npm 전역 설치 CLI(claude)는 PowerShell 에서 claude.ps1 shim 으로
	// 해석되는데, 기본 정책(Restricted)이면 실행이 막힌다.
	// title 은 sanitizeLabel 이 ' " ` 를 제거하므로 작은따옴표 리터럴에 안전하게 들어간다.
	// bin 은 agentBins 화이트리스트 값만 온다.
	script := fmt.Sprintf("$Host.UI.RawUI.WindowTitle='%s'; chcp 65001 > $null; "+
		"$u = New-Object System.Text.UTF8Encoding $false; "+
		"[Console]::InputEncoding = $u; [Console]::OutputEncoding = $u; $OutputEncoding = $u; "+
		"& '%s'", title, bin)
	cmdline := `powershell.exe -NoLogo -NoExit -ExecutionPolicy Bypass -Command "` + script + `"`

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

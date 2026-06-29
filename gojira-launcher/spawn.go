package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// 실행 명령 화이트리스트 (설계 §2.3) — 임의 명령 실행 경로를 만들지 않는다.
//   - 에디터: "code" (VS Code) 고정
//   - 에이전트 CLI: agentBins 의 키만 허용
var agentBins = map[string]string{
	"opencode": "opencode",
	"claude":   "claude",
}

// availableAgents — PATH 에 존재하는 에이전트 CLI 목록(/health 용).
func availableAgents() []string {
	out := []string{}
	for name, bin := range agentBins {
		if _, err := exec.LookPath(bin); err == nil {
			out = append(out, name)
		}
	}
	return out
}

func hasEditor() bool {
	_, err := exec.LookPath("code")
	return err == nil
}

func hasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// resolveAgent — 요청 agent(빈값이면 기본값)를 화이트리스트로 검증.
func (s *Server) resolveAgent(agent string) (string, error) {
	if agent == "" {
		agent = s.cfg.DefaultAgent
	}
	if _, ok := agentBins[agent]; !ok {
		return "", fmt.Errorf("허용되지 않은 agent: %q (opencode|claude)", agent)
	}
	return agent, nil
}

// safeFilePath — file(상대경로)이 repoPath 밖으로 벗어나지 않는지 검증.
func safeFilePath(repoPath, file string) (string, error) {
	if file == "" {
		return "", nil
	}
	clean := filepath.Clean(filepath.Join(repoPath, file))
	rel, err := filepath.Rel(repoPath, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("file 경로가 레포 밖을 가리킵니다: %q", file)
	}
	return clean, nil
}

// OpenInEditor — VS Code 로 폴더(+선택 파일)를 연다. fullFile 은 safeFilePath 검증을 통과한 절대경로.
// 단일 호출로 폴더+파일을 한 번에 전달 — 더블 launch 시 새 창이 maximized 로 떠버리는 현상 회피.
func OpenInEditor(repoPath, fullFile string) error {
	if !hasEditor() {
		return fmt.Errorf("'code' (VS Code) 를 PATH 에서 찾을 수 없습니다")
	}
	args := []string{repoPath}
	if fullFile != "" {
		args = append(args, "-g", fullFile)
	}
	if err := exec.Command("code", args...).Start(); err != nil {
		return fmt.Errorf("VS Code 열기 실패: %w", err)
	}
	return nil
}

// agentEnv — spawn 되는 CLI/에이전트가 로컬 clone 과 백엔드를 찾도록 주입할 env.
// gojira-cli(common.go)·doc-save.sh · .claude/skills/*/SKILL.md 가 이 변수들을 읽는다:
//   GOJIRA_REPO_ROOT — 로컬 YAML/문서 write 대상 clone 루트 (트랙 D/E 로컬화)
//   GOJIRA_USER      — 이슈 write/ingest 의 X-User-Id
//   GOJIRA_API / GOJIRA_API_URL  — gojira-api 주소 (스킬·CLI 둘 다 주입)
//   PLATFORM_API / GOJIRA_PLATFORM_URL — platform-api 주소 (스킬·CLI 둘 다 주입)
func (s *Server) agentEnv(repoPath, user string) []string {
	env := []string{"GOJIRA_REPO_ROOT=" + repoPath}
	// UTF-8 강제 — Windows 콘솔 기본 코드페이지(한국어 환경 CP949 등)에서 한글이
	// U+FFFD 로 깨져 저장되는 문제 방지. 에이전트가 띄우는 bash/jq/python(스킬 스크립트)
	// 가 UTF-8 I/O 를 쓰도록 locale 을 명시한다(콘솔 코드페이지는 terminalCommand 의
	// `chcp 65001` 이 담당). 사용자가 이미 LANG/LC_ALL 을 설정했으면 그 값을 존중.
	if os.Getenv("LANG") == "" {
		env = append(env, "LANG=C.UTF-8")
	}
	if os.Getenv("LC_ALL") == "" {
		env = append(env, "LC_ALL=C.UTF-8")
	}
	env = append(env, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	// Windows: opencode 의 Bash 툴은 기본 PowerShell 을 쓴다 — 스킬은 jq/heredoc 쓰는
	// POSIX bash 라 PowerShell 로 변환되면 깨진다(+ 비영어 locale 인코딩 문제). opencode 는
	// SHELL 환경변수로 셸을 고르므로 Git Bash(bash.exe) 경로를 주입해 강제한다.
	if runtime.GOOS == "windows" {
		if os.Getenv("SHELL") == "" {
			if bp := findGitBash(); bp != "" {
				env = append(env, "SHELL="+bp)
			}
		}
	}
	if user != "" {
		env = append(env, "GOJIRA_USER="+user)
	}
	if s.cfg.Backend.GojiraAPI != "" {
		// GOJIRA_API_URL — gojira-cli 관례, GOJIRA_API — SKILL.md 스크립트 관례.
		env = append(env, "GOJIRA_API_URL="+s.cfg.Backend.GojiraAPI, "GOJIRA_API="+s.cfg.Backend.GojiraAPI)
	}
	if s.cfg.Backend.PlatformAPI != "" {
		// GOJIRA_PLATFORM_URL — gojira-cli 관례, PLATFORM_API — SKILL.md 스크립트 관례.
		env = append(env, "GOJIRA_PLATFORM_URL="+s.cfg.Backend.PlatformAPI, "PLATFORM_API="+s.cfg.Backend.PlatformAPI)
	}
	// 런처가 확보한 외부 CLI(jq, python3 shim)를 에이전트 PATH 맨 앞에 주입 —
	// 스킬의 curl/jq/python3 블록이 시스템 설치 없이도 동작하도록(deps.go 참조).
	if binDir, err := gojiraBinDir(); err == nil {
		env = append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env
}

// findGitBash — Windows 에서 Git for Windows 의 bash.exe 경로를 찾는다.
// System32\bash.exe(WSL 런처)는 의도적으로 후보에서 제외 — 스킬은 Git Bash(MSYS)
// 에서 돌도록 deps 가 준비돼 있다. git 위치(…\Git\cmd\git.exe)에서 …\Git\bin\bash.exe
// 를 유추하고, 표준 설치 경로를 폴백으로 검사한다. 없으면 "" 반환.
func findGitBash() string {
	candidates := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files\Git\usr\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
	}
	if gp, err := exec.LookPath("git"); err == nil {
		// gp 예: C:\Program Files\Git\cmd\git.exe → 루트=…\Git → …\Git\bin\bash.exe
		root := filepath.Dir(filepath.Dir(gp))
		candidates = append([]string{filepath.Join(root, "bin", "bash.exe")}, candidates...)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// SpawnAgent — 검증된 agent 를 repoPath 에서 새 콘솔/터미널 창으로 띄운다.
// CLI 는 대화형이므로 TTY 가 필요 → OS별 startAgent 가 새 콘솔(windows: CreateProcess,
// unix: 터미널 에뮬레이터)을 만든다. sessionId(=pid 문자열) 반환.
// env 는 자식 CLI 가 로컬 clone/백엔드를 찾도록 주입된다(agentEnv).
//
// Design B: label 을 창 제목으로 달아 과제별 에이전트를 구분하고, code 로 세션 레지스트리에
// 등록한다(종료 시 자동 해제 — 과제전환 replace 가 이 PID 를 트리종료). code=="" 는 추적 제외.
func (s *Server) SpawnAgent(agent, repoPath, label, code string, env []string) (string, error) {
	log.Printf("[TEMP-DEBUG] SpawnAgent ENTRY agent=%q repoPath=%q label=%q code=%q envCount=%d goos=%s", agent, repoPath, label, code, len(env), runtime.GOOS)
	bin, ok := agentBins[agent]
	if !ok {
		log.Printf("[TEMP-DEBUG] SpawnAgent rejected agent=%q (not in allowlist)", agent)
		return "", fmt.Errorf("허용되지 않은 agent: %q", agent)
	}
	log.Printf("[TEMP-DEBUG] SpawnAgent bin=%q", bin)
	pid, wait, err := startAgent(bin, label, repoPath, env)
	if err != nil {
		log.Printf("[TEMP-DEBUG] SpawnAgent startAgent FAIL: %v", err)
		return "", fmt.Errorf("%s 실행 실패: %w", agent, err)
	}
	log.Printf("[TEMP-DEBUG] SpawnAgent SUCCESS pid=%d", pid)
	s.sessions.add(code, agent, pid)
	// 좀비 방지 + 종료 시 레지스트리 정리: 백그라운드에서 회수 후 해당 세션 dead 처리.
	go func() {
		wait()
		s.sessions.markDead(code, pid)
	}()
	return fmt.Sprintf("%d", pid), nil
}

// sanitizeLabel — 터미널 창 제목으로 안전한 1줄 라벨. 따옴표/제어문자를 제거해
// start/osascript/터미널 인자 주입을 막는다. 길이도 적당히 자른다.
func sanitizeLabel(label string) string {
	label = strings.Map(func(r rune) rune {
		if r < 0x20 || r == '"' || r == '\'' || r == '`' {
			return -1
		}
		return r
	}, label)
	if len(label) > 60 {
		label = label[:60]
	}
	return label
}

// terminalCommand — unix(mac/linux)에서 대화형 CLI 를 새 터미널에서 실행하는 명령을 만든다.
// windows 는 startAgent(spawn_windows.go)의 CreateProcess 경로를 쓰므로 여기 오지 않는다.
// label 은 새 창의 제목으로 달려 과제별 에이전트를 구분한다(Design B). 빈 문자열이면 미지정.
func terminalCommand(bin, dir, label string, env []string) *exec.Cmd {
	log.Printf("[TEMP-DEBUG] terminalCommand goos=%s bin=%q dir=%q label=%q", runtime.GOOS, bin, dir, label)
	title := sanitizeLabel(label)
	switch runtime.GOOS {
	case "darwin":
		// NOTE: 제목(title) 주입은 AppleScript↔shell 이스케이프가 취약해 생략. mac 은
		// 에이전트 spawn 의 실사용 환경이 아니다(사용자=Windows, 배포=헤드리스 Ubuntu).
		exports := ""
		for _, kv := range env {
			if i := strings.IndexByte(kv, '='); i > 0 {
				exports += fmt.Sprintf("export %s='%s'; ", kv[:i], kv[i+1:])
			}
		}
		script := fmt.Sprintf("tell app \"Terminal\" to do script \"cd %s && %s%s\"", dir, exports, bin)
		return exec.Command("osascript", "-e", script)
	default: // linux/기타 — 흔한 터미널 에뮬레이터를 순서대로 시도.
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
			if _, err := exec.LookPath(term); err == nil {
				// 에뮬레이터별 제목 플래그(best-effort). 미지원이면 -e 만.
				args := []string{}
				if title != "" {
					switch term {
					case "xterm":
						args = append(args, "-T", title)
					case "konsole":
						args = append(args, "-p", "tabtitle="+title)
					default: // gnome-terminal / x-terminal-emulator
						args = append(args, "--title="+title)
					}
				}
				args = append(args, "-e", bin)
				c := exec.Command(term, args...)
				c.Dir = dir
				c.Env = append(os.Environ(), env...)
				return c
			}
		}
		// 폴백: 터미널 없이 detached 실행(PoC — 비대화 환경).
		c := exec.Command(bin)
		c.Dir = dir
		c.Env = append(os.Environ(), env...)
		return c
	}
}

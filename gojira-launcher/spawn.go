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

// SpawnAgent — 검증된 agent 를 repoPath 에서 새 터미널 창으로 띄운다.
// CLI 는 대화형이므로 TTY 가 필요 → OS별 터미널로 감싼다. sessionId(=pid 문자열) 반환.
// env 는 자식 CLI 가 로컬 clone/백엔드를 찾도록 주입된다(agentEnv).
func SpawnAgent(agent, repoPath string, env []string) (string, error) {
	log.Printf("[TEMP-DEBUG] SpawnAgent ENTRY agent=%q repoPath=%q envCount=%d goos=%s", agent, repoPath, len(env), runtime.GOOS)
	bin, ok := agentBins[agent]
	if !ok {
		log.Printf("[TEMP-DEBUG] SpawnAgent rejected agent=%q (not in allowlist)", agent)
		return "", fmt.Errorf("허용되지 않은 agent: %q", agent)
	}
	log.Printf("[TEMP-DEBUG] SpawnAgent bin=%q", bin)
	cmd := terminalCommand(bin, repoPath, env)
	log.Printf("[TEMP-DEBUG] SpawnAgent cmd path=%q args=%v dir=%q", cmd.Path, cmd.Args, cmd.Dir)
	if err := cmd.Start(); err != nil {
		log.Printf("[TEMP-DEBUG] SpawnAgent cmd.Start FAIL: %v", err)
		return "", fmt.Errorf("%s 실행 실패: %w", agent, err)
	}
	pid := cmd.Process.Pid
	log.Printf("[TEMP-DEBUG] SpawnAgent SUCCESS pid=%d", pid)
	// 좀비 방지: 백그라운드에서 회수.
	go func() { _ = cmd.Wait() }()
	return fmt.Sprintf("%d", pid), nil
}

// terminalCommand — OS별로 대화형 CLI 를 새 터미널에서 실행하는 명령을 만든다.
// linux/fallback 은 cmd.Env 로 env 전파, mac/windows 는 셸 스크립트에 export/set 으로 주입.
func terminalCommand(bin, dir string, env []string) *exec.Cmd {
	log.Printf("[TEMP-DEBUG] terminalCommand goos=%s bin=%q dir=%q", runtime.GOOS, bin, dir)
	switch runtime.GOOS {
	case "windows":
		// 새 콘솔 창에서 실행. env 는 자식 프로세스 환경(c.Env)으로 직접 주입한다.
		//
		// 과거에는 `set "K=V"&& ` 프리픽스를 cmd 인자에 끼워 넣었으나, Go 의
		// exec.Command 가 그 인자의 `"` 를 `\"` 로 escape 하는데 cmd.exe 는 `\"` 를
		// 이해하지 못해 모든 set 이 깨졌다(변수 미주입). start 로 뜨는 새 콘솔도
		// 부모 cmd 의 환경 블록을 상속하므로, c.Env 로 넘기면 cmd→start→cmd /k→bin
		// 까지 정상 전파된다(linux 분기와 동일 방식).
		//
		// `chcp 65001` 로 새 콘솔을 UTF-8 코드페이지로 전환한다 — 기본값(한국어 Windows
		// 는 CP949)에서는 에이전트가 출력/전달하는 한글 UTF-8 바이트가 깨져(U+FFFD) 이슈
		// YAML/DB 에 그대로 저장되는 문제가 있었다. chcp+bin 을 한 인자(따옴표 없음)로 묶어
		// 전달하므로 위 `\"` escape 함정에 걸리지 않는다.
		c := exec.Command("cmd", "/c", "start", "", "cmd", "/k", "chcp 65001>nul && "+bin)
		c.Dir = dir
		c.Env = append(os.Environ(), env...)
		log.Printf("[TEMP-DEBUG] terminalCommand windows envCount=%d cmd_args=%v", len(env), c.Args)
		return c
	case "darwin":
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
				c := exec.Command(term, "-e", bin)
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

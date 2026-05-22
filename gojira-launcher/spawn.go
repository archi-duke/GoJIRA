package main

import (
	"fmt"
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
func OpenInEditor(repoPath, fullFile string) error {
	if !hasEditor() {
		return fmt.Errorf("'code' (VS Code) 를 PATH 에서 찾을 수 없습니다")
	}
	// 폴더를 기존 창에 열기.
	if err := exec.Command("code", repoPath).Start(); err != nil {
		return fmt.Errorf("VS Code 폴더 열기 실패: %w", err)
	}
	if fullFile != "" {
		// -r: 같은 창 재사용, -g: 파일로 이동.
		_ = exec.Command("code", "-r", "-g", fullFile).Start()
	}
	return nil
}

// agentEnv — spawn 되는 CLI/에이전트가 로컬 clone 과 백엔드를 찾도록 주입할 env.
// gojira-cli(common.go)·doc-save.sh 가 이 변수들을 읽는다:
//   GOJIRA_REPO_ROOT — 로컬 YAML/문서 write 대상 clone 루트 (트랙 D/E 로컬화)
//   GOJIRA_USER      — 이슈 write/ingest 의 X-User-Id
//   GOJIRA_API_URL / GOJIRA_PLATFORM_URL — 백엔드 주소
func (s *Server) agentEnv(repoPath, user string) []string {
	env := []string{"GOJIRA_REPO_ROOT=" + repoPath}
	if user != "" {
		env = append(env, "GOJIRA_USER="+user)
	}
	if s.cfg.Backend.GojiraAPI != "" {
		env = append(env, "GOJIRA_API_URL="+s.cfg.Backend.GojiraAPI)
	}
	if s.cfg.Backend.PlatformAPI != "" {
		env = append(env, "GOJIRA_PLATFORM_URL="+s.cfg.Backend.PlatformAPI)
	}
	return env
}

// SpawnAgent — 검증된 agent 를 repoPath 에서 새 터미널 창으로 띄운다.
// CLI 는 대화형이므로 TTY 가 필요 → OS별 터미널로 감싼다. sessionId(=pid 문자열) 반환.
// env 는 자식 CLI 가 로컬 clone/백엔드를 찾도록 주입된다(agentEnv).
func SpawnAgent(agent, repoPath string, env []string) (string, error) {
	bin, ok := agentBins[agent]
	if !ok {
		return "", fmt.Errorf("허용되지 않은 agent: %q", agent)
	}
	cmd := terminalCommand(bin, repoPath, env)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%s 실행 실패: %w", agent, err)
	}
	pid := cmd.Process.Pid
	// 좀비 방지: 백그라운드에서 회수.
	go func() { _ = cmd.Wait() }()
	return fmt.Sprintf("%d", pid), nil
}

// terminalCommand — OS별로 대화형 CLI 를 새 터미널에서 실행하는 명령을 만든다.
// linux/fallback 은 cmd.Env 로 env 전파, mac/windows 는 셸 스크립트에 export/set 으로 주입.
func terminalCommand(bin, dir string, env []string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		// 새 콘솔 창에서 실행. env 는 `set "K=V"&&` 로 선주입.
		prefix := ""
		for _, kv := range env {
			prefix += "set \"" + kv + "\"&& "
		}
		c := exec.Command("cmd", "/c", "start", "", "cmd", "/k", prefix+bin)
		c.Dir = dir
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

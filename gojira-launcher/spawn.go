package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// availableAgents — cfg.Agents 중 PATH 에 존재하는 것만 health 응답에 노출.
// 등록은 됐지만 미설치인 CLI 는 클릭해도 spawn 실패하므로 미리 거른다.
func (s *Server) availableAgents() []string {
	out := []string{}
	for _, a := range s.cfg.Agents {
		if _, err := exec.LookPath(a.Command); err == nil {
			out = append(out, a.Name)
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

// resolveAgent — 요청 agent(빈값이면 cfg.DefaultAgent)를 등록 목록에서 조회.
// launcher.json 의 agents 가 화이트리스트.
func (s *Server) resolveAgent(name string) (*AgentSpec, error) {
	if name == "" {
		name = s.cfg.DefaultAgent
	}
	a := findAgentSpec(s.cfg.Agents, name)
	if a == nil {
		known := make([]string, 0, len(s.cfg.Agents))
		for _, sp := range s.cfg.Agents {
			known = append(known, sp.Name)
		}
		return nil, fmt.Errorf("등록되지 않은 agent: %q (등록 목록: %s) — launcher.json 의 agents 에 추가하세요", name, strings.Join(known, ", "))
	}
	return a, nil
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
		// GOJIRA_API_URL — gojira-cli 관례, GOJIRA_API — 이슈 스킬 스크립트 관례. 둘 다 주입.
		env = append(env, "GOJIRA_API_URL="+s.cfg.Backend.GojiraAPI, "GOJIRA_API="+s.cfg.Backend.GojiraAPI)
	}
	if s.cfg.Backend.PlatformAPI != "" {
		env = append(env, "GOJIRA_PLATFORM_URL="+s.cfg.Backend.PlatformAPI)
	}
	return env
}

// SpawnAgent — 등록된 agent 를 repoPath 에서 새 터미널 창으로 띄운다.
// CLI 는 대화형이므로 TTY 가 필요 → OS별 터미널로 감싼다. sessionId(=pid 문자열) 반환.
// callerEnv 는 spawn 컨텍스트(repoRoot/user/backend) — AgentSpec.Env 와 병합돼 자식에게 주입.
func (s *Server) SpawnAgent(spec *AgentSpec, repoPath string, callerEnv []string) (string, error) {
	env := append([]string{}, spec.Env...)
	env = append(env, callerEnv...)
	cmd := terminalCommand(spec.Command, spec.Args, repoPath, env)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%s 실행 실패: %w", spec.Name, err)
	}
	pid := cmd.Process.Pid
	// 좀비 방지: 백그라운드에서 회수.
	go func() { _ = cmd.Wait() }()
	return fmt.Sprintf("%d", pid), nil
}

// terminalCommand — OS별로 대화형 CLI 를 새 터미널에서 실행하는 명령을 만든다.
// linux/fallback 은 cmd.Env 로 env 전파, mac/windows 는 셸 스크립트에 export/set 으로 주입.
func terminalCommand(bin string, extraArgs []string, dir string, env []string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		// 새 콘솔 창에서 실행. env 는 `set "K=V"&&` 로 선주입.
		prefix := ""
		for _, kv := range env {
			prefix += "set \"" + kv + "\"&& "
		}
		full := bin
		for _, a := range extraArgs {
			full += " " + a
		}
		c := exec.Command("cmd", "/c", "start", "", "cmd", "/k", prefix+full)
		c.Dir = dir
		return c
	case "darwin":
		exports := ""
		for _, kv := range env {
			if i := strings.IndexByte(kv, '='); i > 0 {
				exports += fmt.Sprintf("export %s='%s'; ", kv[:i], kv[i+1:])
			}
		}
		argStr := bin
		for _, a := range extraArgs {
			argStr += " " + a
		}
		script := fmt.Sprintf("tell app \"Terminal\" to do script \"cd %s && %s%s\"", dir, exports, argStr)
		return exec.Command("osascript", "-e", script)
	default: // linux/기타 — 흔한 터미널 에뮬레이터를 순서대로 시도.
		termArgs := append([]string{"-e", bin}, extraArgs...)
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
			if _, err := exec.LookPath(term); err == nil {
				c := exec.Command(term, termArgs...)
				c.Dir = dir
				c.Env = append(os.Environ(), env...)
				return c
			}
		}
		// 폴백: 터미널 없이 detached 실행(PoC — 비대화 환경).
		c := exec.Command(bin, extraArgs...)
		c.Dir = dir
		c.Env = append(os.Environ(), env...)
		return c
	}
}

package main

import (
	"fmt"
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

// SpawnAgent — 검증된 agent 를 repoPath 에서 새 터미널 창으로 띄운다.
// CLI 는 대화형이므로 TTY 가 필요 → OS별 터미널로 감싼다. sessionId(=pid 문자열) 반환.
func SpawnAgent(agent, repoPath string) (string, error) {
	bin, ok := agentBins[agent]
	if !ok {
		return "", fmt.Errorf("허용되지 않은 agent: %q", agent)
	}
	cmd := terminalCommand(bin, repoPath)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%s 실행 실패: %w", agent, err)
	}
	pid := cmd.Process.Pid
	// 좀비 방지: 백그라운드에서 회수.
	go func() { _ = cmd.Wait() }()
	return fmt.Sprintf("%d", pid), nil
}

// terminalCommand — OS별로 대화형 CLI 를 새 터미널에서 실행하는 명령을 만든다.
func terminalCommand(bin, dir string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		// 새 콘솔 창에서 실행.
		c := exec.Command("cmd", "/c", "start", "", "cmd", "/k", bin)
		c.Dir = dir
		return c
	case "darwin":
		script := fmt.Sprintf("tell app \"Terminal\" to do script \"cd %s && %s\"", dir, bin)
		return exec.Command("osascript", "-e", script)
	default: // linux/기타 — 흔한 터미널 에뮬레이터를 순서대로 시도.
		for _, term := range []string{"x-terminal-emulator", "gnome-terminal", "konsole", "xterm"} {
			if _, err := exec.LookPath(term); err == nil {
				c := exec.Command(term, "-e", bin)
				c.Dir = dir
				return c
			}
		}
		// 폴백: 터미널 없이 detached 실행(PoC — 비대화 환경).
		c := exec.Command(bin)
		c.Dir = dir
		return c
	}
}

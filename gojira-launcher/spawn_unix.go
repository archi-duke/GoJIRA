//go:build !windows

package main

// startAgent — unix(mac/linux)는 터미널 에뮬레이터 자체가 새 창+TTY 를 제공하므로
// 기존 exec.Cmd 경로(terminalCommand)를 그대로 쓴다. (pid, 종료대기함수, err) 반환.
// 배포 타깃 Ubuntu 는 헤드리스 서버라 GUI 터미널 spawn 은 dev 한정.
func startAgent(bin, label, dir string, env []string) (int, func(), error) {
	c := terminalCommand(bin, dir, label, env)
	if err := c.Start(); err != nil {
		return 0, nil, err
	}
	pid := c.Process.Pid
	wait := func() { _ = c.Wait() }
	return pid, wait, nil
}

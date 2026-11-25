//go:build !windows

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// startAgent — unix(mac/linux)는 터미널 에뮬레이터 자체가 새 창+TTY 를 제공하므로
// 기존 exec.Cmd 경로(terminalCommand)를 그대로 쓴다. (pid, 종료대기함수, err) 반환.
// 배포 타깃 Ubuntu 는 헤드리스 서버라 GUI 터미널 spawn 은 dev 한정.
//
// darwin 은 `open` 이 **Terminal 에 요청만 전달하고 즉시 끝난다.** 그래서
// Start() 만 보면 터미널이 실제로 열렸는지 알 수 없다 — 이전에 osascript 가
// Apple Event 권한(-1743)에 막혀 실패했는데도 "SUCCESS" 로 보고된 원인이다.
// 종료 코드를 확인해 실패를 드러낸다.
func startAgent(bin string, args []string, label, dir string, env []string) (int, func(), error) {
	c := terminalCommand(bin, args, dir, label, env)
	if err := c.Start(); err != nil {
		return 0, nil, err
	}
	pid := c.Process.Pid

	if runtime.GOOS == "darwin" && filepath.Base(c.Path) == "open" {
		// open 은 바로 끝난다. 실패면 여기서 드러난다.
		if err := c.Wait(); err != nil {
			return 0, nil, fmt.Errorf("Terminal 실행 실패(%v) — 터미널이 열리지 않았습니다", err)
		}
		// 성공. 다만 **런처가 아는 pid 는 open 의 것이고 이미 종료됐다.**
		// 에이전트 pid 는 Terminal 이 띄운 셸의 자식이라 여기서 알 수 없다.
		// 세션 레지스트리는 이 pid 로 생존을 추적할 수 없으므로, 종료 대기를
		// 걸지 않고(= 바로 dead 로 마킹되지 않게) 그대로 둔다.
		// mac 은 에이전트 spawn 의 실사용 환경이 아니라 여기까지로 둔다.
		return pid, func() {}, nil
	}

	wait := func() { _ = c.Wait() }
	return pid, wait, nil
}

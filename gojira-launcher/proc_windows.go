//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideConsole — Windows GUI 데몬(콘솔 없이 기동)이 git.exe 등 콘솔 앱을 실행할 때
// 매번 새 콘솔 창이 번쩍이는 것을 막는다. CREATE_NO_WINDOW 를 SysProcAttr 에 OR 한다.
// flagCreateNoWindow 는 install_windows.go 에 정의됨(같은 windows 빌드, package main).
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= flagCreateNoWindow
}

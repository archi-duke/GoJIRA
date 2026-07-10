//go:build !windows

package main

import "os/exec"

// hideConsole — 비 Windows(Linux/macOS)는 콘솔 창 개념이 없어 no-op.
func hideConsole(cmd *exec.Cmd) {}

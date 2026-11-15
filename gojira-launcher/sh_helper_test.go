//go:build !windows

package main

import "os/exec"

func runSh(args ...string) (string, error) {
	out, err := exec.Command("/bin/sh", args...).CombinedOutput()
	return string(out), err
}

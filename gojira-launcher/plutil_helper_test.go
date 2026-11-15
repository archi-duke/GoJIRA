//go:build !windows

package main

import "os/exec"

func runPlutil(path string) (string, error) {
	out, err := exec.Command("plutil", "-lint", path).CombinedOutput()
	return string(out), err
}

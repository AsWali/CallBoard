//go:build !darwin && !linux && !windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

func parentOf(pid int) (int, string, bool) {
	b, err := exec.Command("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
	f := strings.Fields(string(b))
	if err != nil || len(f) < 1 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(f[0])
	return ppid, strings.Join(f[1:], " "), err == nil
}

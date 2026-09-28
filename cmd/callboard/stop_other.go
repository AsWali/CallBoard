//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func stopProcess(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

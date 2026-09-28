package main

import (
	"os"
	"os/exec"
)

func stopProcess(p *os.Process) error { return p.Kill() }

func detach(c *exec.Cmd) {}

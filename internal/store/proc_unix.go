//go:build !windows

package store

import "syscall"

func alive(pid int) bool {
	if pid <= 1 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func Alive(pid int) bool { return alive(pid) }

//go:build windows

package store

import "os"

func alive(pid int) bool {
	if pid <= 1 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	p.Release()
	return true
}

func Alive(pid int) bool { return alive(pid) }

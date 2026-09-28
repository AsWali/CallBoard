package main

import "golang.org/x/sys/unix"

func parentOf(pid int) (int, string, bool) {
	k, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || k.Proc.P_pid == 0 {
		return 0, "", false
	}
	return int(k.Eproc.Ppid), unix.ByteSliceToString(k.Proc.P_comm[:]), true
}

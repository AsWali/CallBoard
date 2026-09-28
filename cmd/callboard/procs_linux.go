package main

import (
	"os"
	"strconv"
	"strings"
)

func parentOf(pid int) (int, string, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", false
	}
	s := string(b)
	open, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return 0, "", false
	}
	f := strings.Fields(s[end+1:])
	if len(f) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(f[1])
	return ppid, s[open+1 : end], err == nil
}

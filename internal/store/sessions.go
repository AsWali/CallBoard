package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Store) pidPath(pid int) string {
	return filepath.Join(s.Shared(), "pids", strconv.Itoa(pid))
}

func (s *Store) LinkProcess(pid int, session string) {
	if pid <= 1 || session == "" || s.Off() || s.SessionOfProcess(pid) == session {
		return
	}
	p := s.pidPath(pid)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(session+"\n"), 0o644)
}

func (s *Store) SessionOfProcess(pid int) string {
	b, err := os.ReadFile(s.pidPath(pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func sandboxed(dir string, err error) error {
	if !os.IsPermission(err) {
		return err
	}
	return fmt.Errorf("can't write %s here (the agent's sandbox keeps commands out of .git); make this change with the callboard MCP tools instead", dir)
}

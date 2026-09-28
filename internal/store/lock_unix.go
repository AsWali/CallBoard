//go:build !windows

package store

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"syscall"
)

func (s *Store) lock() (func(), error) { return s.lockNamed("") }

func (s *Store) lockNamed(name string) (func(), error) {
	if s.Off() {
		return nil, ErrOff
	}
	dir := s.Shared()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, sandboxed(dir, err)
	}
	h := fnv.New32a()
	h.Write([]byte(s.Root))
	if name != "" {
		h.Reset()
		h.Write([]byte(name))
	}
	lf, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("lock-%08x", h.Sum32())), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, sandboxed(dir, err)
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		lf.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(lf.Fd()), syscall.LOCK_UN); lf.Close() }, nil
}

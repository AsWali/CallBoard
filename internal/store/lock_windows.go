//go:build windows

package store

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"
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
	p := filepath.Join(dir, fmt.Sprintf("lock-%08x", h.Sum32()))
	deadline := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if st, serr := os.Stat(p); serr == nil && time.Since(st.ModTime()) > 30*time.Second {
			os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the list is locked by another callboard (%s); if none is running, delete that file", p)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			m[p] = string(b)
		}
		return nil
	})
	return m
}

func TestOffReadsButNeverWrites(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "Alpha", Section: "Now"}, claude)
	s.ClaimItem(a.Key, claude.By, claude.Session, false)
	s.Git("config", "callboard.off", "true")
	before := snapshot(t, s.Root)

	if l, err := s.List("", "all", ""); err != nil || len(l.Items) != 1 {
		t.Fatalf("off lists not readable: %+v %v", l, err)
	}
	if _, err := s.Add(Backlog, board.New{Title: "Bravo"}, claude); !errors.Is(err, ErrOff) {
		t.Fatalf("added while off: %v", err)
	}
	if _, err := s.Tick(a.Key, true, claude); !errors.Is(err, ErrOff) {
		t.Fatalf("ticked while off: %v", err)
	}
	if err := s.Release(a.Key, claude.By, claude.Session, false); !errors.Is(err, ErrOff) {
		t.Fatalf("released while off: %v", err)
	}
	if _, _, err := s.AddList("ideas", claude); err == nil {
		t.Fatal("made a list while off")
	}
	s.Reconcile()
	s.News("s9")
	s.Touch(claude.Session)
	s.LinkProcess(os.Getpid(), "s9")

	after := snapshot(t, s.Root)
	for p, b := range after {
		if before[p] != b {
			t.Errorf("%s written while off", p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			t.Errorf("%s removed while off", p)
		}
	}
}

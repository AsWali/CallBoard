package store

import (
	"os"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func TestSomeoneElsesListFileIsLeftAlone(t *testing.T) {
	s := repo(t)
	faq := "# Questions\n\n## How do I install it?\n\nRun make.\n"
	checklist := "# Backlog\n\n- [ ] Write the docs\n- [x] Ship it\n"
	os.WriteFile(s.File(Questions), []byte(faq), 0o644)
	os.WriteFile(s.File(Backlog), []byte(checklist), 0o644)

	if !s.Foreign(Questions) || !s.Foreign(Backlog) || s.Foreign(Requests) {
		t.Fatal("foreign files not told apart")
	}
	if _, ok := s.ListFile(s.File(Questions)); ok {
		t.Fatal("reading the FAQ would be blocked")
	}
	if _, err := s.Add(Questions, board.New{Title: "Which one?"}, claude); err == nil || !strings.Contains(err.Error(), "isn't a Callboard list") {
		t.Fatalf("added to the FAQ: %v", err)
	}
	if _, err := s.Add(Backlog, board.New{Title: "Mine"}, claude); err == nil {
		t.Fatal("added to someone else's backlog")
	}
	if l, err := s.List("", "all", ""); err != nil || len(l.Items) != 0 {
		t.Fatalf("their lines listed as items: %+v %v", l, err)
	}
	if b, _ := os.ReadFile(s.File(Questions)); string(b) != faq {
		t.Fatalf("FAQ changed:\n%s", b)
	}
	if b, _ := os.ReadFile(s.File(Backlog)); string(b) != checklist {
		t.Fatalf("backlog changed:\n%s", b)
	}
	if _, err := s.Add(Requests, board.New{Title: "A key"}, claude); err != nil {
		t.Fatal(err)
	}

	if n, err := s.Adopt(Questions); err != nil || n != 0 || !s.Foreign(Questions) {
		t.Fatalf("an FAQ with no tasks was taken over: %d %v", n, err)
	}
	if n, err := s.Adopt(Backlog); err != nil || n != 2 || s.Foreign(Backlog) {
		t.Fatalf("adopt: %d %v", n, err)
	}
	if _, err := s.Add(Backlog, board.New{Title: "Mine"}, claude); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.List("backlog", "all", ""); len(l.Items) != 3 {
		t.Fatalf("after adopting: %+v", l.Items)
	}
}

func TestEmptiedListStaysCallboards(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "Only", Section: "Now"}, claude)
	if _, _, err := s.Delete(a.Key, claude); err != nil {
		t.Fatal(err)
	}
	if s.Foreign(Backlog) {
		t.Fatal("a backlog emptied by Callboard counts as someone else's")
	}
	if _, err := s.Add(Backlog, board.New{Title: "Next"}, claude); err != nil {
		t.Fatal(err)
	}
}

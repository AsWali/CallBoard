package store

import (
	"os"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func TestLogIsTrimmed(t *testing.T) {
	defer func(m, k int) { maxLog, keepLog = m, k }(maxLog, keepLog)
	maxLog, keepLog = 20<<10, 10<<10
	s := repo(t)
	gone, _ := s.Add(Backlog, board.New{Title: "Deleted long ago", Section: "Now"}, claude)
	s.Add(Backlog, board.New{Title: "Stays", Section: "Now"}, claude)
	back, _ := s.Add(Backlog, board.New{Title: "Deleted and brought back", Section: "Now"}, claude)
	s.Delete(gone.Key, claude)
	s.Delete(back.Key, claude)
	s.Restore(back.Key, claude)
	s.MarkRead("old")
	for i := 0; i < 150; i++ {
		s.Log(Event{By: "claude", What: "set", Key: "B-zz00", Title: strings.Repeat("x", 100), Session: "busy"})
	}
	s.MarkRead("recent")
	s.Log(Event{By: "you", What: "set", Key: "B-zz01", Title: "after recent looked"})
	s.Log(Event{By: "you", What: "set", Key: "B-zz02", Title: "and one more"})

	st, err := os.Stat(s.logPath())
	if err != nil || st.Size() > int64(maxLog) {
		t.Fatalf("log not trimmed: %v %v", st.Size(), err)
	}
	if news := s.News("recent"); len(news) != 2 || news[0].Title != "after recent looked" {
		t.Fatalf("news after the trim: %+v", news)
	}
	if news := s.News("old"); len(news) != 0 {
		t.Fatalf("a session from before the trim got %d events", len(news))
	}
	all, _ := s.List("", "all", "")
	if l := s.Search(all, "deleted"); len(l.Deleted) != 1 || l.Deleted[0].Key != gone.Key {
		t.Fatalf("deleted items after the trim: %+v", l.Deleted)
	}
	if got, err := s.Restore(gone.Key, you); err != nil || got.Title != "Deleted long ago" {
		t.Fatal(got, err)
	}
}

func TestTrimWithBigDeletions(t *testing.T) {
	defer func(m, k int) { maxLog, keepLog = m, k }(maxLog, keepLog)
	maxLog, keepLog = 20<<10, 10<<10
	s := repo(t)
	for i := 0; i < 40; i++ {
		it, _ := s.Add(Backlog, board.New{Title: "Big", Section: "Now", Body: []string{"  " + strings.Repeat("n", 2000)}}, claude)
		s.Delete(it.Key, claude)
	}
	for i := 0; i < 100; i++ {
		s.Log(Event{By: "claude", What: "set", Key: "B-zz00", Title: strings.Repeat("x", 100)})
	}
	if st, _ := os.Stat(s.logPath()); st.Size() > int64(maxLog) {
		t.Fatalf("deletions kept the log at %d bytes, over %d", st.Size(), maxLog)
	}
}

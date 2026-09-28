package store

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func TestDeleteRestore(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "First", Section: "Now", Body: []string{"  a note"}}, claude)
	b, _ := s.Add(Backlog, board.New{Title: "Second", Section: "Now"}, claude)
	c, _ := s.Add(Backlog, board.New{Title: "Third", Section: "Now", Needs: []string{b.Key}}, claude)
	before, _ := os.ReadFile(s.File(Backlog))
	if _, err := s.ClaimItem(b.Key, claude.By, claude.Session, false); err != nil {
		t.Fatal(err)
	}
	gone, freed, err := s.Delete(b.Key, claude)
	if err != nil || gone.Title != "Second" || strings.Join(freed, ",") != c.Key {
		t.Fatal(gone, freed, err)
	}
	if _, ok := s.Claims()[strings.ToUpper(b.Key)]; ok {
		t.Fatal("the claim outlived the item")
	}
	all, _ := s.List("", "all", "")
	if l := s.Search(all, "second"); len(l.Items) != 0 || len(l.Deleted) != 1 || l.Deleted[0].Section != "Now" || !strings.Contains(FormatList(l), "restore") {
		t.Fatalf("find after delete: %+v", l)
	}
	if _, err := s.Restore(a.Key, you); err == nil {
		t.Fatal("restored an item that was never deleted")
	}
	back, err := s.Restore(b.Key, you)
	if err != nil || back.Title != "Second" {
		t.Fatal(back, err)
	}
	if after, _ := os.ReadFile(s.File(Backlog)); string(after) != string(before) {
		t.Fatalf("not put back where it was:\n%s\nwant\n%s", after, before)
	}
	if _, err := s.Restore(b.Key, you); err == nil {
		t.Fatal("restored twice")
	}
	if l := s.Search(all, "second"); len(l.Deleted) != 0 {
		t.Fatalf("still listed as deleted: %+v", l.Deleted)
	}

	s.Delete(a.Key, claude)
	raw, _ := os.ReadFile(s.File(Backlog))
	os.WriteFile(s.File(Backlog), []byte(strings.Replace(string(raw), "- [ ] Third", "- [ ] Third, edited by hand", 1)), 0o644)
	if _, err := s.Restore(a.Key, you); err != nil {
		t.Fatal(err)
	}
	f, _ := s.Load(Backlog)
	if f.Tasks[0].Key != a.Key || len(f.Tasks[0].Body) != 1 {
		t.Fatalf("first item not first again:\n%s", f.String())
	}

	lines := strings.Split(f.String(), "\n")
	var kept []string
	for _, l := range lines {
		if !strings.Contains(l, c.Key) {
			kept = append(kept, l)
		}
	}
	os.WriteFile(s.File(Backlog), []byte(strings.Join(kept, "\n")), 0o644)
	if _, err := s.Restore(c.Key, you); err != nil {
		t.Fatalf("an item deleted by hand can't come back: %v", err)
	}
}

func TestRestoreInAnotherClone(t *testing.T) {
	s := repo(t)
	s.Add(Backlog, board.New{Title: "First", Section: "Now"}, claude)
	b, _ := s.Add(Backlog, board.New{Title: "Second", Section: "Now", Body: []string{"  its note"}}, claude)
	s.Add(Backlog, board.New{Title: "Third", Section: "Now"}, claude)
	gitIn(t, s.Root, "add", "-A")
	gitIn(t, s.Root, "commit", "-qm", "three")
	before, _ := os.ReadFile(s.File(Backlog))
	if _, _, err := s.Delete(b.Key, claude); err != nil {
		t.Fatal(err)
	}
	gitIn(t, s.Root, "commit", "-qam", "drop second")

	dir := t.TempDir()
	gitIn(t, dir, "clone", "-q", s.Root, "c")
	o := Open(dir + "/c")
	all, _ := o.List("", "all", "")
	l := o.Search(all, "second")
	if len(l.Deleted) != 1 || l.Deleted[0].Key != b.Key || l.Deleted[0].By != "t" {
		t.Fatalf("find in the clone: %+v", l.Deleted)
	}
	back, err := o.Restore(b.Key, you)
	if err != nil || back.Title != "Second" {
		t.Fatal(back, err)
	}
	if after, _ := os.ReadFile(o.File(Backlog)); strings.ReplaceAll(string(after), "\r\n", "\n") != string(before) {
		t.Fatalf("not put back where it was:\n%s\nwant\n%s", after, before)
	}
	if l := o.Search(all, "second"); len(l.Deleted) != 0 {
		t.Fatalf("still listed as deleted: %+v", l.Deleted)
	}
}

func TestDoneByTravelsAndBlame(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "By tool"}, claude)
	b, _ := s.Add(Backlog, board.New{Title: "By hand"}, claude)
	if _, err := s.Tick(a.Key, true, claude); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(s.File(Backlog))
	if !strings.Contains(string(text), "done-by:claude") {
		t.Fatalf("done-by not on the item:\n%s", text)
	}
	os.WriteFile(s.File(Backlog), []byte(strings.Replace(string(text), "- [ ] By hand", "- [x] By hand", 1)), 0o644)
	gitIn(t, s.Root, "add", "-A")
	cmd := exec.Command("git", "-c", "user.email=sam@x", "-c", "user.name=Sam", "commit", "-qm", "ticked")
	cmd.Dir = s.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	dir := t.TempDir()
	gitIn(t, dir, "clone", "-q", s.Root, "c")
	o := Open(dir + "/c")
	l, _ := o.List("", "done", "")
	got := map[string]string{}
	for _, it := range l.Items {
		got[it.Key] = it.DoneBy
	}
	if got[a.Key] != "claude" || got[b.Key] != "Sam" {
		t.Fatalf("done by in a fresh clone: %v", got)
	}
	if _, err := o.Tick(a.Key, false, you); err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(o.File(Backlog)); strings.Contains(string(text), "done-by:claude") {
		t.Fatal("reopening kept done-by")
	}
}

func TestFindPagesDeletedItems(t *testing.T) {
	s := repo(t)
	s.Add(Backlog, board.New{Title: "Kept alpha", Section: "Now"}, claude)
	for i := 0; i < 70; i++ {
		it, _ := s.Add(Backlog, board.New{Title: "Gone alpha", Section: "Now"}, claude)
		s.Delete(it.Key, claude)
	}
	all, _ := s.List("", "all", "")
	l := s.Search(all, "alpha").Page(0, 50)
	if len(l.Items) != 1 || len(l.Deleted) != 49 || l.Total != 71 {
		t.Fatalf("first page: %d items, %d deleted, total %d", len(l.Items), len(l.Deleted), l.Total)
	}
	l = s.Search(all, "alpha").Page(50, 50)
	if len(l.Items) != 0 || len(l.Deleted) != 21 {
		t.Fatalf("second page: %d items, %d deleted", len(l.Items), len(l.Deleted))
	}
	if out := FormatList(l); !strings.Contains(out, "Showing items 51–71 of 71, the last ones.") {
		t.Fatal(out)
	}
}

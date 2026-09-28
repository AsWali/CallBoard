package show

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/store"
)

func repo(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return store.Open(dir)
}

func TestRender(t *testing.T) {
	s := repo(t)
	a := store.Actor{By: "claude", Session: "s1"}
	r, _ := s.Add(store.Requests, board.New{Title: "Make a store account"}, a)
	q, _ := s.Add(store.Questions, board.New{Title: "Which port?", Body: []string{"- 4700 (recommended)", "- 8080"}}, a)
	pub, _ := s.Add(store.Backlog, board.New{Title: "Publish it", Section: "Now", Needs: []string{r.Key, q.Key}}, a)
	s.Add(store.Backlog, board.New{Title: "Tell people", Section: "Now", Needs: []string{pub.Key}}, a)
	docs, _ := s.Add(store.Backlog, board.New{Title: "Write the docs", Section: "Now"}, a)
	s.SetFields(docs.Key, [][2]string{{"prio", "high"}}, a)
	s.Tick(docs.Key, true, a)

	draw := func(words ...string) string {
		return Render(s, words, Opts{Width: 100})
	}
	if out := draw(); !strings.Contains(out, "Waiting on you") || !strings.Contains(out, "unblocks 2") {
		t.Fatalf("overview:\n%s", out)
	}
	if out := draw("todo"); !strings.Contains(out, "Publish it") || strings.Contains(out, "Write the docs") || strings.Contains(out, "store account") {
		t.Fatalf("todo:\n%s", out)
	}
	if out := draw("done"); !strings.Contains(out, "Write the docs") || strings.Contains(out, "Publish it") {
		t.Fatalf("done:\n%s", out)
	}
	if out := draw("all", "prio=high"); !strings.Contains(out, "Write the docs") || strings.Contains(out, "Publish it") {
		t.Fatalf("prio=high:\n%s", out)
	}
	out := draw("graph")
	if !strings.Contains(out, "unblocks 2") || !strings.Contains(out, "└─ ⊘ "+pub.Key) || !strings.Contains(out, "also waits on") {
		t.Fatalf("graph:\n%s", out)
	}
	if out := draw(strings.ToLower(pub.Key)); !strings.Contains(out, "Waits on") || !strings.Contains(out, "Unblocks") || !strings.Contains(out, "History") {
		t.Fatalf("detail:\n%s", out)
	}
	if out := draw("board"); !strings.Contains(out, "Tasks") || !strings.Contains(out, "Requests") {
		t.Fatalf("board:\n%s", out)
	}
	if out := draw("you"); !strings.Contains(out, "Waiting on you") || !strings.Contains(out, "unblocks 2") || !strings.Contains(out, "frees "+pub.Key) || !strings.Contains(out, "recommended: 4700") {
		t.Fatalf("you:\n%s", out)
	}
	s.ClaimItem(pub.Key, a.By, a.Session, false)
	if out := draw("now"); !strings.Contains(out, "claude on main") || !strings.Contains(out, "Publish it") {
		t.Fatalf("now:\n%s", out)
	}
	if out := draw("quiet"); !strings.Contains(out, "Nothing has gone quiet") {
		t.Fatalf("quiet today:\n%s", out)
	}
	later := Render(s, []string{"quiet"}, Opts{Width: 100, Now: time.Now().Add(10 * 24 * time.Hour)})
	if !strings.Contains(later, "Tell people") || !strings.Contains(later, "untouched for 10 days") {
		t.Fatalf("quiet in ten days:\n%s", later)
	}
	if out := draw("recent"); !strings.Contains(out, "Today") || !strings.Contains(out, "Write the docs") {
		t.Fatalf("recent:\n%s", out)
	}
	if out := draw(); !strings.Contains(out, "Waiting on you") || !strings.Contains(out, "Map") || !strings.Contains(out, "Recently done") {
		t.Fatalf("overview:\n%s", out)
	}
	keys := map[string]string{"you": "2", "now": "3", "graph": "4"}
	for _, words := range [][]string{nil, {"you"}, {"graph"}, {"now"}} {
		out := Render(s, words, Opts{Width: 30, Wrap: true, Keys: keys})
		for _, l := range strings.Split(out, "\n") {
			if Width(l) > 30 {
				t.Fatalf("%v: a line wider than 30:\n%s", words, out)
			}
		}
		if strings.Contains(out, "/callboard") {
			t.Fatalf("%v: a /callboard hint in watch:\n%s", words, out)
		}
	}
	if out := Render(s, nil, Opts{Width: 30, Wrap: true, Keys: keys}); !strings.Contains(out, "press 4") || !strings.Contains(out, "Publish it") {
		t.Fatalf("narrow overview:\n%s", out)
	}
	if out := Render(s, Split(`"tell"`), Opts{Color: true}); !strings.Contains(out, "Tell people") || !strings.HasPrefix(out, "\x1b[0m") {
		t.Fatalf("search with colour:\n%q", out)
	}
}

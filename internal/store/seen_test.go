package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func whats(evs []Event) string {
	var out []string
	for _, e := range evs {
		out = append(out, e.By+" "+e.What+" "+e.Title+" ("+e.Via+")")
	}
	return strings.Join(out, "; ")
}

func TestGitChangesBecomeEvents(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "Alpha"}, claude)
	s.Reconcile()
	gitIn(t, s.Root, "add", "-A")
	gitIn(t, s.Root, "commit", "-qm", "seed")
	if evs := s.Reconcile(); len(evs) != 0 {
		t.Fatalf("own writes told again: %s", whats(evs))
	}

	gitIn(t, s.Root, "switch", "-qc", "feat")
	s.Tick(a.Key, true, claude)
	b, _ := s.Add(Backlog, board.New{Title: "Bravo"}, claude)
	gitIn(t, s.Root, "commit", "-qam", "feat work")
	gitIn(t, s.Root, "switch", "-q", "main")
	if got := whats(s.Reconcile()); got != "git reopened Alpha (switching from feat to main); git removed Bravo (switching from feat to main)" {
		t.Fatalf("switch: %s", got)
	}
	gitIn(t, s.Root, "merge", "-q", "feat")
	if got := whats(s.Reconcile()); got != "git ticked Alpha (merge feat); git added Bravo (merge feat)" {
		t.Fatalf("merge: %s", got)
	}
	gitIn(t, s.Root, "reset", "-q", "--hard", "HEAD~1")
	if got := whats(s.Reconcile()); !strings.Contains(got, "git removed Bravo (reset to HEAD~1)") {
		t.Fatalf("reset: %s", got)
	}

	gitIn(t, s.Root, "commit", "-q", "--allow-empty", "-m", "unrelated")
	p := s.File(Backlog)
	txt, _ := os.ReadFile(p)
	os.WriteFile(p, []byte(strings.Replace(string(txt), "- [ ] Alpha", "- [ ] Alpha by hand", 1)), 0o644)
	if got := whats(s.Reconcile()); got != "someone renamed Alpha by hand (edited backlog.md)" {
		t.Fatalf("hand edit: %s", got)
	}

	gitIn(t, s.Root, "commit", "-qam", "hand edit")
	gitIn(t, s.Root, "switch", "-qc", "feat2")
	c, _ := s.Add(Backlog, board.New{Title: "Charlie"}, claude)
	gitIn(t, s.Root, "commit", "-qam", "c")
	gitIn(t, s.Root, "switch", "-q", "main")
	s.Prime(Actor{Session: "s2"})
	gitIn(t, s.Root, "merge", "-q", "feat2")
	n := FormatNews(s.News("s2"))
	if !strings.Contains(n, `Git (merge feat2) added `+c.Key+` "Charlie"`) || strings.Contains(n, b.Key) {
		t.Fatalf("news:\n%s", n)
	}
}

func TestSettledMergeIsTold(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "Alpha"}, claude)
	s.Reconcile()
	gitIn(t, s.Root, "add", "-A")
	gitIn(t, s.Root, "commit", "-qm", "seed")
	gitIn(t, s.Root, "switch", "-qc", "feat")
	s.Rename(a.Key, "Alpha feat", claude)
	s.Add(Backlog, board.New{Title: "From feat"}, claude)
	gitIn(t, s.Root, "commit", "-qam", "f")
	gitIn(t, s.Root, "switch", "-q", "main")
	s.Rename(a.Key, "Alpha main", claude)
	gitIn(t, s.Root, "commit", "-qam", "m")
	s.Reconcile()
	cmd := exec.Command("git", "merge", "feat")
	cmd.Dir = s.Root
	cmd.Run()
	if evs := s.Reconcile(); len(evs) != 0 {
		t.Fatalf("told mid-merge: %s", whats(evs))
	}
	f, _ := s.Load(Backlog)
	if len(f.Conflicts()) == 0 {
		t.Fatalf("expected a conflict:\n%s", f.String())
	}
	if _, err := s.Resolve(a.Key, 2, you); err != nil {
		t.Fatal(err)
	}
	gitIn(t, s.Root, "commit", "-qam", "Merge branch 'feat'")
	got := whats(s.Reconcile())
	if !strings.Contains(got, "git added From feat (merge feat)") || !strings.Contains(got, "git renamed Alpha feat (merge feat)") {
		t.Fatalf("after settling: %s", got)
	}
}

func TestStepWords(t *testing.T) {
	for in, want := range map[string]string{
		"merge feat: Merge made by the 'ort' strategy.":        "merge feat",
		"commit (merge): Merge branch 'feat' into main":        "merge feat",
		"commit: fix the parser":                               "",
		"checkout: moving from main to feat":                   "switching from main to feat",
		"reset: moving to HEAD~2":                              "reset to HEAD~2",
		"pull -q --no-rebase: Fast-forward":                    "pull",
		"pull --rebase (finish): returning to refs/heads/main": "pull --rebase",
		"rebase (finish): returning to refs/heads/feat":        "rebase",
		"cherry-pick: tick C":                                  "cherry-pick “tick C”",
		`revert: Revert "tick C"`:                              "revert “tick C”",
	} {
		if got := stepWords(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestClaimsFollowTheirWorktree(t *testing.T) {
	s := repo(t)
	a, _ := s.Add(Backlog, board.New{Title: "Alpha"}, claude)
	gitIn(t, s.Root, "add", "-A")
	gitIn(t, s.Root, "commit", "-qm", "seed")
	wt := filepath.Join(t.TempDir(), "wt")
	gitIn(t, s.Root, "worktree", "add", "-q", "-b", "feat", wt)
	if _, err := Open(wt).ClaimItem(a.Key, "claude", "s1", false); err != nil {
		t.Fatal(err)
	}
	branch := func() string { return s.Claims()[strings.ToUpper(a.Key)].Branch }
	if branch() != "feat" {
		t.Fatalf("claim on %q", branch())
	}
	gitIn(t, s.Root, "branch", "-m", "feat", "feature")
	if branch() != "feature" {
		t.Fatalf("after rename: %q", branch())
	}
	gitIn(t, s.Root, "worktree", "remove", wt)
	if len(s.Claims()) != 0 {
		t.Fatalf("claim outlived its worktree: %+v", s.Claims())
	}
}

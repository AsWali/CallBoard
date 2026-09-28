package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeLabelsAndBranch(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	write := func(s string) { os.WriteFile(filepath.Join(dir, "f"), []byte(s), 0o644) }
	git("init", "-q", "-b", "main")
	write("a\n")
	git("add", "-A")
	git("commit", "-qm", "a")
	git("switch", "-qc", "feat")
	write("b\n")
	git("commit", "-qam", "on feat")
	sha := git("rev-parse", "--short", "HEAD")
	git("switch", "-q", "main")
	write("c\n")
	git("commit", "-qam", "on main")
	r := Find(dir)
	if r.Branch() != "main" {
		t.Fatalf("branch %q", r.Branch())
	}

	if o, th := r.MergeLabels("HEAD", "feat"); o != "main" || th != "feat" {
		t.Fatalf("merge: %q %q", o, th)
	}

	if _, th := r.MergeLabels("HEAD", sha+" (on feat)"); th != "feat" {
		t.Fatalf("cherry-pick: %q", th)
	}

	if _, th := r.MergeLabels("HEAD", "parent of "+sha+" (on feat)"); th != "undoing “on feat”" {
		t.Fatalf("revert: %q", th)
	}

	git("switch", "-q", "feat")
	git("rebase", "main")
	if o, th := r.MergeLabels("HEAD", sha+" (on feat)"); o != "main" || th != "feat" {
		t.Fatalf("rebase: %q %q", o, th)
	}
	if r.Branch() != "" {
		t.Fatalf("mid-rebase HEAD is detached, got %q", r.Branch())
	}

	if o, th := r.MergeLabels("ours", "theirs"); o != "main" || th != "feat" {
		t.Fatalf("redone rebase: %q %q", o, th)
	}
	git("rebase", "--abort")
	git("switch", "-q", "main")
	git("merge", "feat")
	if o, th := r.MergeLabels("ours", "theirs"); o != "main" || th != "feat" {
		t.Fatalf("redone merge: %q %q", o, th)
	}
	git("merge", "--abort")
	git("cherry-pick", sha)
	if _, th := r.MergeLabels("ours", "theirs"); th != "feat" {
		t.Fatalf("redone cherry-pick: %q", th)
	}
	git("cherry-pick", "--abort")
	git("switch", "-q", "feat")
	if r.Branch() != "feat" || BranchAt(dir) != "feat" {
		t.Fatalf("after abort: %q %q", r.Branch(), BranchAt(dir))
	}

	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "-q", "-b", "side", wt)
	if BranchAt(wt) != "side" || Find(wt).Branch() != "side" {
		t.Fatalf("worktree: %q", BranchAt(wt))
	}
}

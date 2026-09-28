package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBranchesFromFiles(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "trunk")
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "one")
	run("branch", "feature/a")
	run("branch", "zeta")
	run("pack-refs", "--all")
	run("branch", "master")
	run("branch", "feature/b")
	r := Find(dir)
	want := strings.Split(run("for-each-ref", "--format=%(refname:short)", "refs/heads"), "\n")
	if got := r.Branches(); !slices.Equal(got, want) {
		t.Fatalf("branches %v, git says %v", got, want)
	}
	if d := r.DefaultBranch(); d != "master" {
		t.Fatalf("default %q", d)
	}
	run("branch", "-D", "zeta")
	if got := r.Branches(); slices.Contains(got, "zeta") {
		t.Fatalf("a deleted packed branch is still listed: %v", got)
	}
}

func TestFindAgreesWithGit(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "proj")
	os.Mkdir(dir, 0o755)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "one")
	run("worktree", "add", "-q", filepath.Join(base, "wt"), "-b", "side")
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755)
	os.Symlink(dir, filepath.Join(base, "link"))
	for _, d := range []string{dir, filepath.Join(dir, "a", "b"), filepath.Join(base, "wt"), filepath.Join(base, "link"), base} {
		got, want := Find(d), findWithGit(Repo{Root: d, Name: filepath.Base(d)})
		if got != want {
			t.Errorf("%s:\n got  %+v\n want %+v", d, got, want)
		}
	}
}

func TestOff(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q").Run(); err != nil {
		t.Skip("no git")
	}
	r := Find(dir)
	if r.Off() {
		t.Fatal("off by default")
	}
	exec.Command("git", "-C", dir, "config", "callboard.off", "true").Run()
	if !r.Off() {
		t.Fatal("callboard.off=true not seen")
	}
	exec.Command("git", "-C", dir, "config", "--unset", "callboard.off").Run()
	if r.Off() {
		t.Fatal("still off after unset")
	}
}

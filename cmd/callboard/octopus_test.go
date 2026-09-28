package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOctopusMergesListsPerItem(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip()
	}
	tmp := t.TempDir()
	bin, home, dir := filepath.Join(tmp, "bin"), filepath.Join(tmp, "home"), filepath.Join(tmp, "repo")
	if out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "callboard"), ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	os.MkdirAll(home, 0o755)
	os.MkdirAll(dir, 0o755)
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git")
	}
	env := []string{"HOME=" + home, "PATH=" + strings.Join([]string{bin, filepath.Join(home, ".local", "bin"), filepath.Dir(gitBin), "/usr/bin", "/bin"}, ":"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		"CALLBOARD_BY=you", "CLAUDECODE=", "CODEX_THREAD_ID="}
	run := func(name string, args ...string) (string, error) {
		t.Helper()
		if name == "callboard" {
			name = filepath.Join(bin, name)
		}
		c := exec.Command(name, args...)
		c.Dir, c.Env = dir, append(os.Environ(), env...)
		out, err := c.CombinedOutput()
		return string(out), err
	}
	must := func(name string, args ...string) string {
		t.Helper()
		out, err := run(name, args...)
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return out
	}
	must("git", "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("backlog.md merge=callboard\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("## Now\n\n- [ ] Shared <!-- id:B-aaaa -->\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("one\ntwo\nthree\n"), 0o644)
	must("git", "add", "-A")
	must("git", "commit", "-qm", "init")
	if out := must("callboard", "list"); !strings.Contains(out, "merges of several branches at once") {
		t.Fatalf("no word about octopus merges:\n%s", out)
	}
	if got := strings.TrimSpace(must("git", "config", "--get", "pull.octopus")); got != "callboard" {
		t.Fatalf("pull.octopus = %q", got)
	}
	for _, b := range []string{"a", "b", "c"} {
		must("git", "checkout", "-q", "-b", b, "main")
		must("callboard", "add", "From "+b)
		switch b {
		case "a":
			must("callboard", "tick", "B-aaaa")
		case "c":
			os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("one\ntwo\nthree, from c\n"), 0o644)
		}
		must("git", "commit", "-qam", b)
	}
	must("git", "checkout", "-q", "main")
	must("callboard", "add", "From main")
	must("git", "commit", "-qam", "main")
	if out, err := run("git", "merge", "--no-edit", "a", "b", "c"); err != nil {
		t.Fatalf("octopus merge failed: %v\n%s", err, out)
	}
	parents := strings.Fields(must("git", "log", "-1", "--format=%p"))
	backlog, _ := os.ReadFile(filepath.Join(dir, "backlog.md"))
	notes, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if len(parents) != 4 || !strings.Contains(string(backlog), "- [x] Shared") || !strings.Contains(string(notes), "three, from c") {
		t.Fatalf("parents %v\n%s\n%s", parents, backlog, notes)
	}
	for _, want := range []string{"From main", "From a", "From b", "From c"} {
		if !strings.Contains(string(backlog), want) {
			t.Fatalf("%q lost:\n%s", want, backlog)
		}
	}

	os.Remove(filepath.Join(bin, "callboard"))
	must("git", "checkout", "-q", "-b", "d", "HEAD~1")
	if out, _ := run("git", "merge", "--no-edit", "a", "b"); strings.Contains(out, "Could not find merge strategy") {
		t.Fatalf("without callboard, octopus merges broke:\n%s", out)
	}
}

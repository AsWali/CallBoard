package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/gitx"
	"github.com/AsWali/CallBoard/internal/merge"
)

func TestCloneWithoutDriverIsRepaired(t *testing.T) {
	if testing.Short() {
		t.Skip("builds callboard")
	}

	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "callboard"), "github.com/AsWali/CallBoard/cmd/callboard")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	git("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(strings.Join(attrLines, "\n")+"\n"), 0o644)
	list := "## Now\n- [ ] Alpha <!-- id:B-aaaa -->\n\n## Later\n"
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte(list), 0o644)
	git("add", "-A")
	git("commit", "-qm", "seed")
	add := func(line string) {
		os.WriteFile(filepath.Join(dir, "backlog.md"), []byte(strings.Replace(list, "\n\n## Later", "\n"+line+"\n\n## Later", 1)), 0o644)
		git("commit", "-qam", line)
	}
	git("switch", "-qc", "feat")
	add("- [ ] From feat <!-- id:B-ffff -->")
	git("switch", "-q", "main")
	add("- [ ] From main <!-- id:B-mmmm -->")
	git("merge", "feat")
	if b, _ := os.ReadFile(filepath.Join(dir, "backlog.md")); !strings.Contains(string(b), "<<<<<<<") {
		t.Fatalf("expected git's line conflict:\n%s", b)
	}

	msgs := strings.Join(EnsureDriver(gitx.Find(dir)), "\n")
	if !strings.Contains(msgs, "set the merge driver") || !strings.Contains(msgs, "no conflicts left, and it's staged") {
		t.Fatalf("messages:\n%s", msgs)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if strings.Contains(string(b), "<<<<<<<") || !strings.Contains(string(b), "From main") || !strings.Contains(string(b), "From feat") {
		t.Fatalf("not redone per item:\n%s", b)
	}
	if st := git("status", "--short"); strings.Contains(st, "UU") {
		t.Fatalf("still unmerged: %s", st)
	}
	if again := EnsureDriver(gitx.Find(dir)); len(again) != 0 {
		t.Fatalf("second run should be quiet: %v", again)
	}

	git("config", "merge.callboard.driver", "callboard merge %O %A %B %P")
	if m := EnsureDriver(gitx.Find(dir)); len(m) != 1 || !strings.Contains(m[0], "updated the merge driver") {
		t.Fatalf("upgrade: %v", m)
	}
}

func TestDriverAsksForTheseRules(t *testing.T) {
	if rulesVer != strconv.Itoa(merge.Rules) {
		t.Fatalf("the driver asks for merge rules %s, but this callboard has %d", rulesVer, merge.Rules)
	}
}

func TestLineMergeWithoutCallboardIsRedone(t *testing.T) {
	if testing.Short() {
		t.Skip("builds callboard")
	}
	if runtime.GOOS == "windows" {
		t.Skip("its stand-in for an older callboard is a shell script, and it trims PATH to Unix folders")
	}
	bin, old := t.TempDir(), t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "callboard"), "github.com/AsWali/CallBoard/cmd/callboard")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	os.WriteFile(filepath.Join(old, "callboard"), []byte("#!/bin/sh\n[ \"$2\" = --rules ] && exit 2\ntouch \"$0.ran\"\nexit 0\n"), 0o755)
	gitBin, _ := exec.LookPath("git")
	sysPath := filepath.Dir(gitBin) + string(os.PathListSeparator) + "/usr/bin:/bin"
	for _, c := range []struct{ name, path string }{{"missing", sysPath}, {"older", old + string(os.PathListSeparator) + sysPath}} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "PATH="+c.path)
				out, _ := cmd.CombinedOutput()
				return string(out)
			}
			git("init", "-q", "-b", "main")
			os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(strings.Join(attrLines, "\n")+"\n"), 0o644)
			if !setDriver(gitx.Find(dir)) {
				t.Fatal("set driver")
			}
			list := "## Now\n- [ ] Alpha <!-- id:B-aaaa -->\n\n## Later\n"
			os.WriteFile(filepath.Join(dir, "backlog.md"), []byte(list), 0o644)
			git("add", "-A")
			git("commit", "-qm", "seed")
			add := func(line string) {
				os.WriteFile(filepath.Join(dir, "backlog.md"), []byte(strings.Replace(list, "\n\n## Later", "\n"+line+"\n\n## Later", 1)), 0o644)
				git("commit", "-qam", line)
			}
			git("switch", "-qc", "feat")
			add("- [ ] From feat <!-- id:B-ffff -->")
			git("switch", "-q", "main")
			add("- [ ] From main <!-- id:B-mmmm -->")
			git("merge", "feat")
			b, _ := os.ReadFile(filepath.Join(dir, "backlog.md"))
			if !strings.Contains(string(b), "<<<<<<< HEAD"+lineMark) || !strings.Contains(string(b), "From feat") {
				t.Fatalf("expected a marked line merge with both sides:\n%s", b)
			}
			if _, err := os.Stat(filepath.Join(old, "callboard.ran")); err == nil {
				t.Fatal("the older callboard merged")
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+sysPath)
			msgs := strings.Join(EnsureDriver(gitx.Find(dir)), "\n")
			if !strings.Contains(msgs, "line by line without callboard") || !strings.Contains(msgs, "no conflicts left, and it's staged") {
				t.Fatalf("messages:\n%s", msgs)
			}
			b, _ = os.ReadFile(filepath.Join(dir, "backlog.md"))
			if strings.Contains(string(b), "<<<<<<<") || !strings.Contains(string(b), "From main") || !strings.Contains(string(b), "From feat") {
				t.Fatalf("not redone per item:\n%s", b)
			}
		})
	}
}

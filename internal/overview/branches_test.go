package overview

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/gitx"
)

func TestBranchesPage(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t", "-c", "merge.callboard.driver=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(name, s string) { os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644) }
	list := func(lines ...string) string { return "# Backlog\n\n## Now\n" + strings.Join(lines, "\n") + "\n" }
	alpha, beta := "- [ ] Alpha <!-- id:B-aaaa -->", "- [ ] Beta <!-- id:B-bbbb -->"
	git("init", "-q", "-b", "main")
	write("backlog.md", list(alpha, beta))
	write("code.go", "package x\n")
	git("add", "-A")
	git("commit", "-qm", "seed")

	git("switch", "-qc", "feat")
	write("backlog.md", list("- [ ] Alpha <!-- id:B-aaaa prio:low -->", beta, "- [ ] Gamma <!-- id:B-gggg -->"))
	git("commit", "-qam", "feat: prio and Gamma")

	git("switch", "-qc", "fix", "main")
	write("backlog.md", list(alpha, "- [x] Beta <!-- id:B-bbbb done:2026-09-27T10:00 -->"))
	git("commit", "-qam", "fix: tick Beta")
	git("switch", "-q", "main")
	write("code.go", "package x // main\n")
	git("commit", "-qam", "code only")
	git("merge", "-q", "--no-ff", "--no-edit", "fix")
	write("backlog.md", list("- [ ] Alpha <!-- id:B-aaaa prio:high -->", "- [x] Beta <!-- id:B-bbbb done:2026-09-27T10:00 -->"))
	git("commit", "-qam", "main: prio high")

	p := BuildBranches(gitx.Find(dir), "feat")
	if p.Main != "main" || len(p.Branches) != 2 {
		t.Fatalf("page: %+v", p)
	}
	c := p.Branches[0]
	if c.Name != "feat" || !c.Current || c.Fork == nil || c.Fork.Subject != "seed" || c.InMain {
		t.Fatalf("feat card: %+v", c)
	}

	if len(c.Since) != 2 || c.Since[0].Subject != "main: prio high" || c.Since[1].Merged != "fix" || c.Since[1].Items[0].What != "ticked" {
		t.Fatalf("since: %+v", c.Since)
	}
	if len(c.Brings) != 2 {
		t.Fatalf("brings: %+v", c.Brings)
	}

	if len(c.Conflicts) != 1 || c.Conflicts[0].Key != "B-aaaa" || c.Conflicts[0].Versions[0].Label != "main" || c.Conflicts[0].Versions[1].Differs != "prio low" {
		t.Fatalf("conflicts: %+v", c.Conflicts)
	}
	if f := p.Branches[1]; f.Name != "fix" || !f.InMain || len(f.Conflicts) != 0 {
		t.Fatalf("fix card: %+v", f)
	}
}

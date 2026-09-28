package gitx

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type Repo struct {
	Root   string
	Common string
	GitDir string
	Name   string
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func Find(dir string) Repo {
	abs, _ := filepath.Abs(dir)
	r := Repo{Root: abs, Name: filepath.Base(abs)}
	if os.Getenv("GIT_DIR") != "" || os.Getenv("GIT_WORK_TREE") != "" {
		return findWithGit(r)
	}
	for d := real(abs); ; d = filepath.Dir(d) {
		dot := filepath.Join(d, ".git")
		st, err := os.Stat(dot)
		if err == nil {
			gd := dot
			if !st.IsDir() {
				b, err := os.ReadFile(dot)
				p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
				if err != nil || !ok {
					return findWithGit(r)
				}
				if !filepath.IsAbs(p) {
					p = filepath.Join(d, p)
				}
				gd = real(p)
			}
			r.Root, r.GitDir, r.Common = d, gd, gd
			if b, err := os.ReadFile(filepath.Join(gd, "commondir")); err == nil {
				c := strings.TrimSpace(string(b))
				if !filepath.IsAbs(c) {
					c = filepath.Join(gd, c)
				}
				r.Common = real(c)
			}
			r.Name = filepath.Base(d)
			if filepath.Base(r.Common) == ".git" {
				r.Name = filepath.Base(filepath.Dir(r.Common))
			}
			return r
		}
		if filepath.Dir(d) == d {
			return r
		}
	}
}

func real(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func findWithGit(r Repo) Repo {
	abs := r.Root
	top, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return r
	}
	r.Root = filepath.FromSlash(top)
	r.Name = filepath.Base(r.Root)
	if c, err := git(abs, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		r.Common = filepath.FromSlash(c)
		if filepath.Base(r.Common) == ".git" {
			r.Name = filepath.Base(filepath.Dir(r.Common))
		}
	}
	if d, err := git(abs, "rev-parse", "--absolute-git-dir"); err == nil {
		r.GitDir = filepath.FromSlash(d)
	}
	return r
}

func (r Repo) Branch() string {
	if r.GitDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(r.GitDir, "HEAD"))
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/heads/")
	if !ok {
		return ""
	}
	return ref
}

func (r Repo) Shared() string {
	if r.Common != "" {
		return filepath.Join(r.Common, "callboard")
	}
	return filepath.Join(r.Root, ".callboard", "local")
}

func (r Repo) Worktrees() []string {
	out, err := r.Git("worktree", "list", "--porcelain")
	if err != nil {
		return []string{r.Root}
	}
	var dirs []string
	for _, l := range strings.Split(out, "\n") {
		if d, ok := strings.CutPrefix(l, "worktree "); ok {
			if d = filepath.FromSlash(d); Exists(d) {
				dirs = append(dirs, d)
			}
		}
	}
	if len(dirs) == 0 {
		dirs = []string{r.Root}
	}
	return dirs
}

func (r Repo) Git(args ...string) (string, error) { return git(r.Root, args...) }

func HasGit() bool { _, err := exec.LookPath("git"); return err == nil }

func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

func (r Repo) WorktreeBranches() map[string]string {
	out := map[string]string{}
	s, err := r.Git("worktree", "list", "--porcelain")
	if err != nil {
		return out
	}
	dir := ""
	for _, l := range strings.Split(s, "\n") {
		if d, ok := strings.CutPrefix(l, "worktree "); ok {
			dir = filepath.FromSlash(d)
		}
		if b, ok := strings.CutPrefix(l, "branch refs/heads/"); ok && Exists(dir) {
			out[b] = dir
		}
	}
	return out
}

func (r Repo) Branches() []string {
	if r.Common == "" {
		return nil
	}
	if Exists(filepath.Join(r.Common, "reftable")) {
		s, err := r.Git("for-each-ref", "--format=%(refname:short)", "refs/heads")
		if err != nil || s == "" {
			return nil
		}
		return strings.Split(s, "\n")
	}
	seen := map[string]bool{}
	heads := filepath.Join(r.Common, "refs", "heads")
	filepath.WalkDir(heads, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !strings.HasSuffix(p, ".lock") {
			rel, _ := filepath.Rel(heads, p)
			seen[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if b, err := os.ReadFile(filepath.Join(r.Common, "packed-refs")); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if _, ref, ok := strings.Cut(l, " refs/heads/"); ok && !strings.HasPrefix(l, "#") {
				seen[strings.TrimSpace(ref)] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}

func (r Repo) DefaultBranch() string {
	have := r.Branches()
	if b, err := os.ReadFile(filepath.Join(r.Common, "refs", "remotes", "origin", "HEAD")); err == nil && r.Common != "" {
		if o, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "ref: refs/remotes/origin/"); ok && slices.Contains(have, o) {
			return o
		}
	}
	for _, b := range []string{"main", "master"} {
		if slices.Contains(have, b) {
			return b
		}
	}
	return r.Branch()
}

func (r Repo) Show(ref, path string) (string, bool) {
	cmd := exec.Command("git", "show", ref+":"+path)
	cmd.Dir = r.Root
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func (r Repo) MergeBase(a, b string) string {
	s, _ := r.Git("merge-base", a, b)
	return s
}

var commitLabel = regexp.MustCompile(`^(?:parent of )?([0-9a-f]{7,40}) \((.*)\)$`)

func (r Repo) MergeLabels(ours, theirs string) (string, string) {
	gd, _ := r.Git("rev-parse", "--absolute-git-dir")
	gd = filepath.FromSlash(gd)
	read := func(name string) string {
		for _, d := range []string{"rebase-merge", "rebase-apply"} {
			if b, err := os.ReadFile(filepath.Join(gd, d, name)); err == nil {
				return strings.TrimSpace(string(b))
			}
		}
		return ""
	}
	branchOf := func(sha string) string {
		n, err := r.Git("name-rev", "--name-only", "--no-undefined", "--refs=refs/heads/*", sha)
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.FieldsFunc(n, func(c rune) bool { return c == '~' || c == '^' })[0], "refs/heads/")
	}
	rebasing := gd != "" && read("head-name") != ""

	if ours == "ours" {
		ours = "HEAD"
	}
	if theirs == "theirs" && gd != "" {
		theirs = r.merging(gd, rebasing, read("head-name"), branchOf)
	}
	if ours == "HEAD" {
		switch b := r.Branch(); {
		case rebasing:
			if n := branchOf(read("onto")); n != "" {
				ours = n
			}
		case b != "":
			ours = b
		}
	}
	if m := commitLabel.FindStringSubmatch(theirs); m != nil {
		switch {
		case strings.HasPrefix(theirs, "parent of "):
			theirs = "undoing “" + m[2] + "”"
		case rebasing:
			theirs = strings.TrimPrefix(read("head-name"), "refs/heads/")
		default:
			if n := branchOf(m[1]); n != "" {
				theirs = n
			}
		}
	}
	return ours, theirs
}

var mergeMsg = regexp.MustCompile(`^Merge (?:remote-tracking )?branch '([^']+)'`)

func (r Repo) merging(gd string, rebasing bool, headName string, branchOf func(string) string) string {
	if rebasing {
		return strings.TrimPrefix(headName, "refs/heads/")
	}
	head := func(name string) string {
		b, _ := os.ReadFile(filepath.Join(gd, name))
		f := strings.Fields(string(b))
		if len(f) == 0 {
			return ""
		}
		return f[0]
	}
	if sha := head("MERGE_HEAD"); sha != "" {
		if b, err := os.ReadFile(filepath.Join(gd, "MERGE_MSG")); err == nil {
			if m := mergeMsg.FindStringSubmatch(string(b)); m != nil {
				return m[1]
			}
		}
		if n := branchOf(sha); n != "" {
			return n
		}
	}
	if sha := head("CHERRY_PICK_HEAD"); sha != "" {
		if n := branchOf(sha); n != "" {
			return n
		}
	}
	if sha := head("REVERT_HEAD"); sha != "" {
		if subj, err := r.Git("log", "-1", "--format=%s", sha); err == nil {
			return "undoing “" + subj + "”"
		}
	}
	return "theirs"
}

func BranchAt(dir string) string {
	dot := filepath.Join(dir, ".git")
	st, err := os.Stat(dot)
	if err != nil {
		return ""
	}
	gd := dot
	if !st.IsDir() {
		b, err := os.ReadFile(dot)
		if err != nil {
			return ""
		}
		p, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
		if !ok {
			return ""
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		gd = p
	}
	return Repo{GitDir: gd}.Branch()
}

func (r Repo) Off() bool {
	if r.Common == "" {
		return false
	}
	b, err := os.ReadFile(filepath.Join(r.Common, "config"))
	if err != nil {
		return false
	}
	in := false
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			in = strings.EqualFold(strings.Trim(l, "[] \t"), "callboard")
			continue
		}
		if k, v, ok := strings.Cut(l, "="); in && ok && strings.EqualFold(strings.TrimSpace(k), "off") {
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "true", "yes", "on", "1":
				return true
			}
			return false
		}
	}
	return false
}

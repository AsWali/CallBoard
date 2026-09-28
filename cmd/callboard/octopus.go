package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/merge"
)

const octopusEnv = "CALLBOARD_OCTOPUS_THEIRS"

func gitOut(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func gitRun(env []string, args ...string) error {
	c := exec.Command("git", args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	c.Env = append(os.Environ(), env...)
	return c.Run()
}

func octopusCmd(args []string) int {
	if len(args) == 1 && args[0] == "--check" {
		return 0
	}
	var remotes []string
	head, sep := "", false
	for _, a := range args {
		switch {
		case !sep && a == "--":
			sep = true
		case !sep:
		case head == "":
			head = a
		default:
			remotes = append(remotes, a)
		}
	}
	if len(remotes) < 2 {
		return 2
	}
	if exec.Command("git", "diff-index", "--quiet", "--cached", "HEAD", "--").Run() != nil {
		fmt.Println("Error: Your local changes to the following files would be overwritten by merge")
		names, _ := gitOut("diff-index", "--cached", "--name-only", "HEAD", "--")
		for _, n := range strings.Split(names, "\n") {
			fmt.Println("    " + n)
		}
		return 2
	}
	h, err := gitOut("rev-parse", "--verify", "-q", head)
	if err != nil {
		return 2
	}
	mrc := []string{h}
	mrt, _ := gitOut("write-tree")
	nonFF, failed := false, false
	for _, sha := range remotes {
		if failed {
			fmt.Println("Automated merge did not work.")
			fmt.Println("Should not be doing an octopus.")
			return 2
		}
		name := sha
		if v := os.Getenv("GITHEAD_" + sha); v != "" {
			name = v
		} else if v := os.Getenv("GITHEAD_" + strings.ToUpper(sha)); v != "" {
			name = v
		}
		out, err := gitOut(append([]string{"merge-base", "--all", sha}, mrc...)...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to find common commit with %s\n", name)
			return 2
		}
		common := strings.Fields(out)
		if slices.Contains(common, sha) {
			fmt.Printf("Already up to date with %s\n", name)
			continue
		}
		if !nonFF && len(mrc) == 1 && len(common) == 1 && common[0] == mrc[0] {
			fmt.Printf("Fast-forwarding to: %s\n", name)
			if gitRun(nil, "read-tree", "-u", "-m", head, sha) != nil {
				return 2
			}
			mrc = []string{sha}
			mrt, _ = gitOut("write-tree")
			continue
		}
		nonFF = true
		fmt.Printf("Trying simple merge with %s\n", name)
		if gitRun(nil, append(append([]string{"read-tree", "-u", "-m", "--aggressive"}, common...), mrt, sha)...) != nil {
			return 2
		}
		next, err := gitOut("write-tree")
		if err != nil {
			fmt.Println("Simple merge did not work, trying automatic merge.")
			self, err := os.Executable()
			if err != nil {
				return 2
			}
			if gitRun([]string{octopusEnv + "=" + name}, "merge-index", "-o", self, "-a") != nil {
				failed = true
			}
			next, _ = gitOut("write-tree")
		}
		mrc = append(mrc, sha)
		mrt = next
	}
	if failed {
		return 1
	}
	return 0
}

func oneFile(args []string) int {
	if err := gitRun(nil, append([]string{"merge-one-file"}, args...)...); err != nil {
		if x, ok := err.(*exec.ExitError); ok {
			return x.ExitCode()
		}
		return 2
	}
	return 0
}

func octopusFile(args []string, theirs string) int {
	orig, ours, their, path := args[0], args[1], args[2], args[3]
	attr, _ := gitOut("check-attr", "merge", "--", path)
	if !strings.HasSuffix(attr, ": merge: callboard") || ours == "" || their == "" {
		return oneFile(args)
	}
	blob := func(sha string) (string, bool) {
		if sha == "" {
			return "", true
		}
		out, err := exec.Command("git", "cat-file", "blob", sha).Output()
		return string(out), err == nil
	}
	base, ok1 := blob(orig)
	a, ok2 := blob(ours)
	b, ok3 := blob(their)
	if !ok1 || !ok2 || !ok3 {
		fmt.Fprintf(os.Stderr, "callboard: couldn't read the versions of %s\n", path)
		return 2
	}
	if !board.HasKeys(base) && !board.HasKeys(a) && !board.HasKeys(b) {
		return oneFile(args)
	}
	r := merge.MergeAs(merge.PrefixFor(path), base, a, b, "HEAD", theirs)
	mode := os.FileMode(0o644)
	if args[5] == "100755" {
		mode = 0o755
	}
	if err := os.WriteFile(path, []byte(r.Text), mode); err != nil {
		fmt.Fprintln(os.Stderr, "callboard:", err)
		return 2
	}
	if r.Conflicts > 0 {
		fmt.Fprintf(os.Stderr, "callboard: %d task(s) in %s changed on both sides; pick one version between the markers\n", r.Conflicts, path)
		return 1
	}
	if gitRun(nil, "update-index", "--add", "--", path) != nil {
		return 2
	}
	return 0
}

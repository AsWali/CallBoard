package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/gitx"
	"github.com/AsWali/CallBoard/internal/merge"
	"github.com/AsWali/CallBoard/internal/server"
	"github.com/AsWali/CallBoard/internal/store"
)

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	st := store.Open(wd())
	port := fs.Int("port", 0, "port (default: this worktree's own, 4700–4999)")
	open := fs.Bool("open", false, "open the page in the browser")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	want := *port
	if want == 0 {
		want = server.DefaultPort(st.Root)
	}

	p := want
	for tries := 0; ; tries++ {
		switch server.Probe(p, st.Root) {
		case "ours":
			url := fmt.Sprintf("http://localhost:%d", p)
			fmt.Printf("✓ callboard already running for %s at %s\n", st.Root, url)
			if *open {
				openURL(url)
			}
			return nil
		case "free":
			goto listen
		}
		if *port != 0 {
			return fmt.Errorf("port %d is taken by something else; pick another: callboard serve --port %d", p, p+1)
		}
		if tries == 20 {
			return fmt.Errorf("ports %d–%d are all taken; pick one: callboard serve --port N", want, p)
		}
		if tries == 0 {
			fmt.Printf("· port %d is taken by another program; trying the next one\n", p)
		}
		p++
	}
listen:
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url := fmt.Sprintf("http://localhost:%d", p)
	branch := st.Branch()
	if branch == "" {
		branch = "no branch"
	}
	var files []string
	for _, k := range st.Lists() {
		files = append(files, k.File)
	}
	agents := "Claude Code and Codex hear the lists (hooks, AGENTS.md) and change them with the MCP tools or the callboard command"
	if st.Off() {
		agents = "Callboard is switched off here, so the lists can be read but not changed (callboard on turns it back on)"
	}
	fmt.Printf("✓ Callboard for %s (%s) at %s\n  lists:  %s in %s\n  agents: %s\n  stop:   Ctrl-C\n", st.Name, branch, url, strings.Join(files, ", "), store.Tilde(st.Root), agents)
	if *open {
		go func() { time.Sleep(300 * time.Millisecond); openURL(url) }()
	}
	return server.New(wd()).Serve(ctx, p)
}
func openURL(url string) {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	exec.Command(name, url).Start()
}
func mergeCmd(args []string) int {
	if len(args) == 2 && args[0] == "--rules" {
		if n, err := strconv.Atoi(args[1]); err == nil && n <= merge.Rules {
			return 0
		}
		return 1
	}
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "callboard merge BASE OURS THEIRS [PATH]: git calls this; see callboard setup")
		return 2
	}
	read := func(p string) (string, bool) {
		b, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "callboard merge:", err)
			return "", false
		}
		return string(b), true
	}
	base, ok1 := read(args[0])
	ours, ok2 := read(args[1])
	theirs, ok3 := read(args[2])
	if !ok1 || !ok2 || !ok3 {
		return 2
	}

	var lo, lt string
	if len(args) > 5 {
		lo, lt = gitx.Find(".").MergeLabels(args[4], args[5])
	}
	path := ""
	if len(args) > 3 {
		path = args[3]
	}
	if !board.HasKeys(base) && !board.HasKeys(ours) && !board.HasKeys(theirs) {
		return lineMerge(args[1], args[0], args[2], lo, lt)
	}
	r := merge.MergeAs(merge.PrefixFor(path), base, ours, theirs, lo, lt)
	if err := os.WriteFile(args[1], []byte(r.Text), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "callboard merge:", err)
		return 2
	}
	if r.Conflicts > 0 {
		path := "backlog.md"
		if len(args) > 3 {
			path = args[3]
		}
		fmt.Fprintf(os.Stderr, "callboard: %d task(s) in %s changed on both sides; pick one version between the markers\n", r.Conflicts, path)
		return 1
	}
	return 0
}

func lineMerge(ours, base, theirs, lo, lt string) int {
	args := []string{"merge-file"}
	for _, l := range []string{lo, "base", lt} {
		if l != "" {
			args = append(args, "-L", l)
		}
	}
	c := exec.Command("git", append(args, ours, base, theirs)...)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		if x, ok := err.(*exec.ExitError); ok && x.ExitCode() > 0 && x.ExitCode() < 128 {
			return 1
		}
		fmt.Fprintln(os.Stderr, "callboard merge:", err)
		return 2
	}
	return 0
}

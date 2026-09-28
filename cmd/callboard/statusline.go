package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/setup"
	"github.com/AsWali/CallBoard/internal/store"
)

func statusline(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "on":
			return setup.StatusLineOn(os.Stdout, slices.Contains(args[1:], "--below"))
		case "off":
			return setup.StatusLineOff(os.Stdout)
		case "status":
			fmt.Println("Callboard in Claude Code's status line: " + setup.StatusLineState())
			return nil
		case "help", "--help", "-h":
			fmt.Println(statusHelp)
			return nil
		}
	}
	in, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	var info struct {
		SessionID string `json:"session_id"`
		Cwd       string `json:"cwd"`
		Workspace struct {
			CurrentDir string `json:"current_dir"`
		} `json:"workspace"`
	}
	json.Unmarshal(in, &info)
	if len(args) == 2 && args[0] == "--below" {
		sh := exec.Command("sh", "-c", args[1])
		if runtime.GOOS == "windows" && !has("sh") {
			sh = exec.Command("cmd", "/c", args[1])
		}
		sh.Stdin = bytes.NewReader(in)
		sh.Stderr = os.Stderr
		out, _ := sh.Output()
		if s := strings.TrimRight(string(out), "\n"); s != "" {
			fmt.Println(s)
		}
	}
	dir := info.Workspace.CurrentDir
	if dir == "" {
		dir = info.Cwd
	}
	if dir == "" {
		dir = wd()
	}
	if line := statusText(store.Open(dir), info.SessionID); line != "" {
		fmt.Println(line)
	}
	return nil
}

const statusHelp = `callboard statusline on [--below]   show a Callboard line in Claude Code's status line (opt-in; setup never does it)
                                    --below keeps your own status line and puts Callboard's line under it
callboard statusline off            take it out again (your own status line comes back as it was)
callboard statusline status         whether it's on
callboard statusline                print the line (Claude Code runs this, with the session on stdin)

Codex's status line only takes its own built-in items, so it can't show this; use callboard panel there.`

func statusText(st *store.Store, session string) string {
	if !st.InUse() {
		return ""
	}
	v, err := st.View()
	if err != nil {
		return ""
	}
	merges := v.Merges()
	me := store.Actor{By: "claude", Session: session}
	match := st.Matcher(me)
	var mine []store.Item
	you, ready := 0, 0
	for _, k := range st.Lists() {
		for _, t := range v.Files[k.Name].Tasks {
			if t.Done || store.InMerge(merges, t.Key) {
				continue
			}
			it := v.Item(k, t)
			switch {
			case it.Kind != "task" || store.IsForYou(it) && it.Claim == nil:
				you++
			case it.Claim != nil && session != "" && it.Claim.Session == session:
				mine = append(mine, it)
			case !it.Blocked && it.Claim == nil:
				if _, other := match(it.Fields[store.ForField]); !other {
					ready++
				}
			}
		}
	}
	parts := []string{"\x1b[1mCallboard\x1b[0m"}
	for _, it := range mine {
		parts = append(parts, "\x1b[33m◐\x1b[0m "+it.Key+" "+clip(it.Title, 40))
	}
	var counts []string
	if you > 0 {
		counts = append(counts, fmt.Sprintf("\x1b[35m%d for you\x1b[0m", you))
	}
	counts = append(counts, fmt.Sprintf("\x1b[2m%d ready\x1b[0m", ready))
	if len(mine) == 0 {
		if next := v.NextUp(me); next != nil {
			counts = append(counts, "\x1b[2mnext "+next.Key+" "+clip(next.Title, 40)+"\x1b[0m")
		}
	}
	return strings.Join(parts, "  ") + "  " + strings.Join(counts, "\x1b[2m · \x1b[0m")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

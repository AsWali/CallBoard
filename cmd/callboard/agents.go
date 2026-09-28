package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/AsWali/CallBoard/internal/store"
)

func agents(args []string) error {
	fs := flag.NewFlagSet("agents", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	as := store.Open(wd()).Agents(20)
	if *asJSON {
		if as == nil {
			as = []store.Agent{}
		}
		return printJSON(as)
	}
	if len(as) == 0 {
		fmt.Println("No agents here yet.")
	}
	for _, a := range as {
		tool := map[string]string{"codex": "Codex"}[a.Tool]
		if tool == "" {
			tool = "Claude Code"
		}
		name := a.Name
		if name == "" {
			name = a.Title
		}
		if name == "" {
			name = strings.SplitN(a.Prompt, "\n", 2)[0]
			if len(name) > 70 {
				name = name[:69] + "…"
			}
		}
		state := "ended"
		switch {
		case a.Live && a.Status == "busy":
			state = "working"
		case a.Live:
			state = "idle"
		case a.Tool == "codex":
			state = "last active " + a.Active
		}
		extra := ""
		if a.Color != "" {
			extra += " " + a.Color
		}
		if a.Subagents > 0 {
			extra += fmt.Sprintf(" · %d subagents", a.Subagents)
		}
		fmt.Printf("%-8s  %-11s %-8s %s%s\n", a.Session[:min(8, len(a.Session))], tool, state, name, extra)
	}
	return nil
}

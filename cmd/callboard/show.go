package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AsWali/CallBoard/internal/show"
	"github.com/AsWali/CallBoard/internal/store"
)

func showCmd(args []string) error {
	color := false
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == "" {
		color = true
	}
	var rest []string
	for _, a := range args {
		switch a {
		case "--plain":
			color = false
		case "--color":
			color = true
		default:
			rest = append(rest, a)
		}
	}
	fmt.Print(show.Render(store.Open(wd()), rest, show.Opts{Color: color, Width: termWidth()}))
	return nil
}

func slashCommand(prompt string) bool {
	rest, ok := strings.CutPrefix(strings.TrimSpace(prompt), "/callboard")
	if !ok || rest != "" && rest[0] != ' ' && rest[0] != '\t' && rest[0] != '\n' {
		return false
	}
	st := store.Open(wd())
	var text string
	switch {
	case !st.HasLists():
		text = "Callboard isn't used here: this folder isn't a git repo and has no backlog.md, requests.md or questions.md."
	case len(show.Split(rest)) > 0 && strings.EqualFold(show.Split(rest)[0], "panel"):
		var err error
		if text, err = openPanel(show.Split(rest)[1:]); err != nil {
			text = "✗ " + err.Error()
		}
	default:
		text = show.Render(st, show.Split(rest), show.Opts{Color: true, Width: termWidth() - 14})
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"continue": false, "stopReason": "\n" + strings.TrimRight(text, "\n")})
	return true
}

func termWidth() int {
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 20 {
		return n
	}
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer tty.Close()
		cmd := exec.Command("stty", "size")
		cmd.Stdin = tty
		if out, err := cmd.Output(); err == nil {
			if f := strings.Fields(string(out)); len(f) == 2 {
				if n, err := strconv.Atoi(f[1]); err == nil && n > 20 {
					return n
				}
			}
		}
	}
	return 100
}

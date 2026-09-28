package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/gitx"
)

const (
	agentsStart = "<!-- callboard: added by callboard setup; callboard disconnect removes it -->"
	agentsEnd   = "<!-- /callboard -->"
	tomlHead    = "[mcp_servers.callboard]"
)

var agentsBlock = []string{
	agentsStart,
	"## Callboard",
	"",
	"This repo's work lives in backlog.md (tasks), requests.md (things only the human can do) and questions.md (the human's decisions), kept by Callboard. Don't edit those files by hand. Don't read or grep them either: answer questions about them with the list tool (its find searches, its key shows one item) or `callboard list --find WORDS`.",
	"",
	"- Start with `callboard prime`: the open items, what to do next and how to keep the lists.",
	"- Change them with the callboard MCP tools (list, add, edit, tick, claim, assume, view and the rest); they work inside a sandbox. Claim a task before you start it, tick it when it's done, add what you find, assume an answer when you can't wait. The `callboard` command reads anywhere, but writing needs `.git`, which a sandbox may guard; use the MCP tools then.",
	"- When the human asks to see the lists a certain way (a board, most urgent first, by area, what's blocked), make that view with the view tool, so it stays on their page, and say what it shows.",
	agentsEnd,
}

var tomlBlock = []string{
	tomlHead,
	`command = "callboard"`,
	`args = ["mcp"]`,
}

func CodexInstalled() bool {
	_, err := exec.LookPath("codex")
	return err == nil
}

var snakeRe = regexp.MustCompile(`([a-z])([A-Z])`)

func CodexHooksTrusted() bool {
	dir := codexDir()
	conf := readString(filepath.Join(dir, "config.toml"))
	for _, h := range hooks {
		event := strings.ToLower(snakeRe.ReplaceAllString(h[0], "${1}_${2}"))
		if !strings.Contains(conf, filepath.Join(dir, "hooks.json")+":"+event+":") {
			return false
		}
	}
	return true
}

func Codex(w io.Writer, r gitx.Repo) error {
	s := &Step{W: w}
	connectCodex(s, filepath.Join(r.Root, "AGENTS.md"), filepath.Join(r.Root, ".codex"), agentsBlock, relTo(r.Root))
	fmt.Fprintln(w, "  Codex asks you once, by design: trust this folder when it asks (for the MCP server), and approve Callboard's hooks with /hooks.")
	if s.Err {
		return fmt.Errorf("setup didn't finish; see ✗ above")
	}
	return nil
}

func connectCodex(s *Step, agents, dir string, block []string, name func(string) string) {
	changed, err := editLines(agents, func(lines []string) []string {
		lines = cutBlock(lines, agentsStart, agentsEnd)
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		return append(lines, block...)
	})
	report(s, changed, err, name(agents)+": how to keep the lists, for Codex and other agents")
	changed, err = editLines(filepath.Join(dir, "config.toml"), func(lines []string) []string {
		if slices.Contains(lines, tomlHead) {
			return lines
		}
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		return append(lines, tomlBlock...)
	})
	report(s, changed, err, name(filepath.Join(dir, "config.toml"))+": MCP server \"callboard\" (callboard mcp)")
	changed, err = editJSON(filepath.Join(dir, "hooks.json"), addHooks)
	report(s, changed, err, name(filepath.Join(dir, "hooks.json"))+": hooks for session start, prompts, tools, reads of the lists and subagents")
}

func relTo(root string) func(string) string {
	return func(p string) string {
		if r, err := filepath.Rel(root, p); err == nil {
			return r
		}
		return p
	}
}

func disconnectCodex(s *Step, agents, dir string, name func(string) string) {
	changed, err := editLines(agents, func(lines []string) []string {
		out := cutBlock(lines, agentsStart, agentsEnd)
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		return out
	})
	reportOff(s, changed, err, name(agents)+": removed the Callboard part")
	changed, err = editLines(filepath.Join(dir, "config.toml"), func(lines []string) []string {
		i := slices.Index(lines, tomlHead)
		if i < 0 {
			return lines
		}
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "[") {
			j++
		}
		out := append(slices.Clone(lines[:i]), lines[j:]...)
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		return out
	})
	reportOff(s, changed, err, name(filepath.Join(dir, "config.toml"))+": removed the callboard server")
	changed, err = editJSON(filepath.Join(dir, "hooks.json"), dropHooks)
	reportOff(s, changed, err, name(filepath.Join(dir, "hooks.json"))+": removed the hooks")
	os.Remove(dir)
}

func cutBlock(lines []string, start, end string) []string {
	i := slices.Index(lines, start)
	if i < 0 {
		return lines
	}
	j := i
	for j < len(lines) && lines[j] != end {
		j++
	}
	if j < len(lines) {
		j++
	}
	if i > 0 && lines[i-1] == "" && (j == len(lines) || lines[j] == "") {
		i--
	}
	return append(slices.Clone(lines[:i]), lines[j:]...)
}

func CodexStatus(agents, dir string) (set bool, missing []string) {
	b, _ := os.ReadFile(agents)
	t, _ := os.ReadFile(filepath.Join(dir, "config.toml"))
	m, _ := readJSON(filepath.Join(dir, "hooks.json"))
	has := []bool{strings.Contains(string(b), agentsStart), strings.Contains(string(t), tomlHead), m != nil && hasHook(m, hooks[0][0], hooks[0][1])}
	names := []string{filepath.Base(agents), "config.toml", "hooks.json"}
	for i, h := range has {
		set = set || h
		if !h {
			missing = append(missing, names[i])
		}
	}
	return set, missing
}

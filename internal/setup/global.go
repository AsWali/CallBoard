package setup

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

var globalAgentsBlock = []string{
	agentsStart,
	"## Callboard",
	"",
	"Every project keeps its work in Callboard: backlog.md (tasks), requests.md (things only the human can do) and questions.md (the human's decisions). Don't edit those files by hand, and don't keep todo lists or notes of open work anywhere else. Don't read or grep them either: answer questions about them with the list tool (its find searches, its key shows one item) or `callboard list --find WORDS`.",
	"",
	"- Start with `callboard prime` in a project: the open items, what to do next and how to keep the lists.",
	"- Change them with the callboard MCP tools (list, add, edit, tick, claim, assume, view and the rest); they work inside a sandbox. Claim a task before you start it, tick it when it's done, add what you find, assume an answer when you can't wait.",
	agentsEnd,
}

const slashFile = `---
description: Show Callboard's lists here (todo, done, requests, questions, graph, board, an item); /callboard help lists the rest
argument-hint: "[todo|done|requests|questions|ready|waiting|graph|board|table|KEY|view:NAME|prio=high|help]"
disable-model-invocation: true
allowed-tools: Bash(callboard show:*)
---
<!-- ` + slashMark + ` -->
!` + "`callboard show --plain $ARGUMENTS`" + `

Show the output above to the user as it is, in a code block, and add nothing.
`

const slashMark = "made by callboard setup --global"

func slashPath() string { return filepath.Join(claudeDir(), "commands", "callboard.md") }

func ourSlash() (there, ours bool) {
	b, err := os.ReadFile(slashPath())
	return err == nil, strings.Contains(string(b), slashMark)
}

func claudeDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func codexDir() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

func attributesFile() string {
	if out, err := exec.Command("git", "config", "--global", "--get", "core.attributesFile").Output(); err == nil {
		if p := strings.TrimSpace(string(out)); p != "" {
			if strings.HasPrefix(p, "~/") {
				home, _ := os.UserHomeDir()
				p = filepath.Join(home, p[2:])
			}
			return p
		}
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "git", "attributes")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "git", "attributes")
}

func readString(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

func gitGlobal(args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"config", "--global"}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func ClaudeInstalled() bool {
	if _, err := exec.LookPath("claude"); err == nil {
		return true
	}
	_, err := os.Stat(claudeDir())
	return err == nil
}

func claudeMCP() bool {
	p := filepath.Join(claudeDir(), ".claude.json")
	if os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, ".claude.json")
	}
	m, err := readJSON(p)
	return err == nil && m.Child("mcpServers").ChildIf(serverName) != nil
}

func Global(w io.Writer, exe string) error {
	s := &Step{W: w}
	linkBinary(s, exe)
	pathHint(s, filepath.Dir(LinkPath()))

	if ClaudeInstalled() {
		if _, err := exec.LookPath("claude"); err != nil {
			s.bad("claude isn't on your PATH, so Claude Code's MCP server wasn't added; install Claude Code, then run this again")
		} else if claudeMCP() {
			s.same("Claude Code: MCP server \"callboard\" for every project (already there)")
		} else if out, err := exec.Command("claude", "mcp", "add", "--scope", "user", serverName, "--", "callboard", "mcp").CombinedOutput(); err != nil {
			s.bad("Claude Code's MCP server: %s", strings.TrimSpace(string(out)))
		} else {
			s.ok("Claude Code: MCP server \"callboard\" for every project (claude mcp add --scope user)")
		}
		settings := filepath.Join(claudeDir(), "settings.json")
		changed, err := editJSON(settings, addHooks)
		report(s, changed, err, tilde(settings)+": hooks for session start, prompts, tools, reads of the lists and subagents")
		switch there, ours := ourSlash(); {
		case there && !ours:
			s.same("%s is your own, so /callboard was left alone", tilde(slashPath()))
		case ours && readString(slashPath()) == slashFile:
			s.same("Claude Code: the /callboard command (already there)")
		default:
			os.MkdirAll(filepath.Dir(slashPath()), 0o755)
			if err := os.WriteFile(slashPath(), []byte(slashFile), 0o644); err != nil {
				s.bad("couldn't write %s: %v", tilde(slashPath()), err)
			} else {
				s.ok("Claude Code: the /callboard command, which shows the lists without asking the model (%s)", tilde(slashPath()))
			}
		}
	}

	if cur, _ := gitGlobal("--get", "merge.callboard.driver"); cur == driverCmd {
		s.same("git: merge driver \"callboard\" in your global config (already there)")
	} else {
		_, err1 := gitGlobal("merge.callboard.name", "Callboard: merge the lists per item")
		_, err2 := gitGlobal("merge.callboard.driver", driverCmd)
		if err1 != nil || err2 != nil {
			s.bad("couldn't set the merge driver: git config --global merge.callboard.driver %q", driverCmd)
		} else {
			s.ok("git: merge driver \"callboard\" in your global config")
		}
	}
	setOctopus(s, gitGlobal, "your global config")
	attrs := attributesFile()
	changed, err := editLines(attrs, func(lines []string) []string {
		for _, a := range attrLines {
			if !slices.Contains(lines, a) {
				lines = append(lines, a)
			}
		}
		return lines
	})
	report(s, changed, err, tilde(attrs)+": the lists and views merge per item in every repo")

	if CodexInstalled() {
		dir := codexDir()
		connectCodex(s, filepath.Join(dir, "AGENTS.md"), dir, globalAgentsBlock, tilde)
		if !CodexHooksTrusted() {
			fmt.Fprintln(w, "  Codex asks you once, by design: approve Callboard's hooks with /hooks.")
		}
	}
	fmt.Fprintln(w, "  Undo: callboard disconnect --global. The page doesn't run; open it in a project with: callboard serve --open")
	if s.Err {
		return fmt.Errorf("setup didn't finish; see ✗ above")
	}
	return nil
}

func DisconnectGlobal(w io.Writer) error {
	s := &Step{W: w}
	if _, err := exec.LookPath("claude"); err == nil && claudeMCP() {
		if out, err := exec.Command("claude", "mcp", "remove", "--scope", "user", serverName).CombinedOutput(); err != nil {
			s.bad("Claude Code's MCP server: %s", strings.TrimSpace(string(out)))
		} else {
			s.ok("Claude Code: removed the MCP server")
		}
	}
	settings := filepath.Join(claudeDir(), "settings.json")
	changed, err := editJSON(settings, dropHooks)
	reportOff(s, changed, err, tilde(settings)+": removed the hooks")
	if _, ours := ourSlash(); ours {
		os.Remove(slashPath())
		s.ok("Claude Code: removed the /callboard command")
	}
	if cur, _ := gitGlobal("--get", "merge.callboard.driver"); cur != "" {
		gitGlobal("--remove-section", "merge.callboard")
		s.ok("git: removed the merge driver from your global config")
	}
	dropOctopus(s, gitGlobal, "your global config")
	attrs := attributesFile()
	changed, err = editLines(attrs, func(lines []string) []string {
		return slices.DeleteFunc(lines, func(l string) bool { return slices.Contains(attrLines, l) })
	})
	reportOff(s, changed, err, tilde(attrs)+": removed the Callboard merge lines")
	if StatusLineState() != "off" {
		StatusLineOff(w)
	}
	disconnectCodex(s, filepath.Join(codexDir(), "AGENTS.md"), codexDir(), tilde)
	fmt.Fprintln(w, "  The lists and their history stay in each project. Undo this: callboard setup --global")
	return nil
}

func GlobalStatus() (set bool, missing []string) {
	has := func(ok bool, name string) {
		set = set || ok
		if !ok {
			missing = append(missing, name)
		}
	}
	if ClaudeInstalled() {
		has(claudeMCP(), "Claude Code's MCP server")
		m, _ := readJSON(filepath.Join(claudeDir(), "settings.json"))
		has(m != nil && hasHook(m, hooks[0][0], hooks[0][1]), "Claude Code's hooks")
		_, ours := ourSlash()
		has(ours && readString(slashPath()) == slashFile, "the /callboard command")
	}
	cur, _ := gitGlobal("--get", "merge.callboard.driver")
	has(cur == driverCmd, "git's merge driver")
	if runtime.GOOS != "windows" {
		has(octopusReady(gitGlobal), "git's merges of several branches")
	}
	b, _ := os.ReadFile(attributesFile())
	has(strings.Contains(string(b), attrLines[0]), "git's attributes")
	if CodexInstalled() {
		codexSet, codexMissing := CodexStatus(filepath.Join(codexDir(), "AGENTS.md"), codexDir())
		set = set || codexSet
		for _, x := range codexMissing {
			missing = append(missing, "Codex's "+x)
		}
	}
	return set, missing
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/AsWali/CallBoard/internal/mcpserver"
	"github.com/AsWali/CallBoard/internal/setup"
	"github.com/AsWali/CallBoard/internal/store"
)

const version = "0.4.0"

const usage = `callboard: the tasks, requests and questions of this worktree, in
backlog.md, requests.md and questions.md.

Items:
  callboard show [WORDS]                the lists drawn for you: todo, done, requests, questions, ready, waiting,
                                        graph, board, table, a KEY, view:NAME, prio=high, by:prio, sort:-prio
                                        (callboard show help); in Claude Code, /callboard WORDS shows the same
  callboard watch [WORDS]               the same, kept on screen and drawn again when the lists change;
                                        Tab or 1-9 switch views, arrows scroll, q quits
  callboard panel [WORDS] | off         open watch beside the agent: a split when the terminal can
                                        (tmux, Zellij, iTerm2, WezTerm, Kitty, Windows Terminal), else a
                                        window beside it; /callboard panel in Claude Code, !callboard panel in Codex
  callboard list [--kind task|request|question|NAME|all] [--ready | --done | --all] [--section NAME] [--find WORDS] [--from N] [--limit N] [--json]
  callboard add "title" [--kind K] [--section NAME] [--under KEY] [--needs KEY,KEY] [--body "line"]...
                        [--option "text"]... [--recommended N]   options are for questions
  callboard tick KEY [--undo]           mark a task or request done (--undo opens it again)
  callboard rename KEY "new title"
  callboard delete KEY...                delete items (callboard restore KEY brings one back, where it was)
  callboard note KEY "text" [--replace] add a note under it (--replace swaps all its notes; --clear removes them)
  callboard set KEY field=value...      facts on an item: status=doing prio=high area="page ui" (field= removes)
  callboard move KEY "Section"          put it under another heading
  callboard move KEY --before|--after KEY2   put it just before or after another item in its list
  callboard section NAME --to NEW | --before S | --after S | --remove [--into S] [--kind K]
                                        rename, reorder or remove a heading (--into: where its items go)
  callboard lists [add NAME]            the lists here; add makes NAME.md, a task list with its own key letter
  callboard lists rename NAME NEW       rename one of your lists (its keys stay)
  callboard lists remove NAME [--force] remove one of your lists, file and all (--force if it has open items)
  callboard views [--json]              the page's saved views
  callboard view [V-KEY] [--name N] [--list L] [--layout list|board|table] [--group FIELD]
                 [--order a,b] [--sort f,-g] [--filter f=v]... [--show f,g] [--delete]
                 [--suggest "why"] [--keep]
                                        make or change a view; everyone sees the same views.
                                        --suggest proposes it: the page offers Keep and Dismiss
  callboard resolve KEY 1|2             after a merge where two branches changed an item differently:
                                        keep the first or the second version
  callboard needs KEY [KEY...]          what it waits on (--none: nothing)
  callboard claim KEY [--force]         you're working on it; release KEY drops it
  callboard assume QKEY N               an agent picks option N itself; the question stays open
  callboard answer QKEY ANSWER [--why "reason"]   answer: an option number or your words
  callboard reopen QKEY                 ask an answered question again
  callboard list --kind question --done   answered questions, with their answers
  callboard news                        what the human changed since this session last looked
  callboard agents [--json]             the Claude Code and Codex sessions here: name, colour, working or not
  callboard branches [--json]           each branch against main, in the lists: where it forked, what main
                                        got since, what it changes, and which items would conflict if merged now
  callboard keys                        give items typed by hand a key
  KEY may carry the version you read, B-k3f9@7c; the change is refused if it has changed since.

Agents and git:
  callboard prime                       what the SessionStart hook prints
  callboard hook prompt|tool|subagent   the other hooks (Claude Code calls these)
  callboard mcp                         the MCP server on stdio
  callboard merge BASE OURS THEIRS [PATH]   git merge driver (git calls this)
  callboard octopus BASES -- HEAD REMOTES   merging several branches at once (git calls this through git-merge-callboard)

Page and setup:
  callboard serve [--port N] [--open]   the live page for this worktree (only runs when you start it)
                                        for your phone or teammates (--read-only: they can't change anything)
  callboard setup --global              connect Claude Code, Codex and git for every project, in your own settings
  callboard setup [claude|codex]        connect them in this repo only, in files you commit (for a team)
  callboard disconnect [--global]       undo setup (leaves the callboard command for other projects)
  callboard install                     copy this binary to ~/.local/bin, so it works without a checkout
  callboard uninstall                   remove it from ~/.local/bin
  callboard off | on                    switch Callboard off in this repo (hooks quiet, lists read-only), or back on
  callboard status [--global]           what setup has done (--global: only the setup for every project)
  callboard statusline on [--below] | off   a Callboard line in Claude Code's status line; only when you ask,
                                        and --below keeps your own status line above it
  callboard version

Env:
  CALLBOARD_BY        who changes are by (default: "codex" inside Codex, "claude" inside Claude Code, else "you")
  CALLBOARD_SESSION   session id for claims and news from the command line (Claude Code
                      and Codex set theirs: CLAUDE_CODE_SESSION_ID, CODEX_THREAD_ID)

Examples:
  callboard add "Parse the three files" --section v1
  callboard add "Write the docs" --under B-k3f9
  callboard add "Which port?" --kind question --option "From the path" --option "Always 4700" --recommended 1
  callboard add "Make a GitHub repo" --kind request --body "1. Go to github.com/new" --body "2. Name it callboard"
  callboard tick B-k3f9@7c
  callboard answer Q-7x1c 1 --why "bookmarks keep working"
`

func main() {
	if theirs := os.Getenv(octopusEnv); theirs != "" && len(os.Args) == 8 {
		os.Exit(octopusFile(os.Args[1:], theirs))
	}
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	if cmd == "hook" && len(args) == 1 && args[0] == "prompt" && slashCommand(hookInput().Prompt) {
		return
	}
	if (cmd == "prime" || cmd == "hook") && !store.Open(wd()).InUse() {
		return
	}
	switch cmd {
	case "merge", "octopus", "setup", "disconnect", "off", "on", "status", "statusline", "panel", "install", "uninstall", "version", "--version", "-v", "help", "--help", "-h":
	case "hook":
		if len(args) > 0 && args[0] == "tool" {
			break
		}
		fallthrough
	default:
		st := store.Open(wd())
		if st.Off() {
			break
		}
		for _, l := range setup.EnsureDriver(st.Repo) {
			fmt.Fprintln(os.Stderr, l)
		}
		st.Reconcile()
	}
	switch cmd {
	case "show":
		err = showCmd(args)
	case "watch":
		err = watch(args)
	case "panel":
		err = panelCmd(args)
	case "statusline":
		err = statusline(args)
	case "list", "ls":
		err = list(args)
	case "add":
		err = add(args)
	case "tick", "done":
		err = tick(args)
	case "set":
		err = setFields(args)
	case "move":
		err = move(args)
	case "delete", "rm":
		err = deleteItem(args)
	case "restore", "undelete":
		err = restore(args)
	case "section", "sections":
		err = section(args)
	case "lists":
		err = lists(args)
	case "views":
		err = views(args)
	case "agents":
		err = agents(args)
	case "branches":
		err = branches(args)
	case "view":
		err = view(args)
	case "resolve":
		err = resolve(args)
	case "rename":
		err = rename(args)
	case "note":
		err = note(args)
	case "needs":
		err = needs(args)
	case "claim":
		err = claim(args)
	case "release":
		err = release(args)
	case "assume":
		err = assume(args)
	case "answer":
		err = answer(args)
	case "reopen":
		err = reopen(args)
	case "news":
		err = news()
	case "off", "on":
		err = switchOff(cmd == "off")
	case "keys":
		err = keys()
	case "prime":
		err = prime()
	case "hook":
		err = hook(args)
	case "serve":
		err = serve(args)
	case "mcp":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = mcpserver.Run(ctx, wd(), version)
	case "merge":
		os.Exit(mergeCmd(args))
	case "octopus":
		os.Exit(octopusCmd(args))
	case "setup":
		err = setupCmd(args)
	case "install":
		err = setup.Install(os.Stdout, exe())
	case "uninstall":
		err = setup.Uninstall(os.Stdout)
	case "disconnect":
		if slices.Contains(args, "--global") {
			err = setup.DisconnectGlobal(os.Stdout)
		} else {
			err = setup.Disconnect(os.Stdout, store.Open(wd()).Repo, exe())
		}
	case "status":
		err = status(slices.Contains(args, "--global"))
	case "version", "--version", "-v":
		fmt.Println("callboard", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "callboard: no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "callboard:", err)
		os.Exit(1)
	}
}

func wd() string { d, _ := os.Getwd(); return d }

func exe() string {
	p, err := os.Executable()
	if err != nil {
		return "callboard"
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func actor() store.Actor {
	a := store.Actor{By: store.By(), Session: os.Getenv("CALLBOARD_SESSION")}
	if a.Session == "" {
		a.Session = store.AgentSession()
	}
	if a.Session == "" && os.Getenv("CLAUDECODE") != "" {
		st := store.Open(wd())
		for _, p := range ancestors(6) {
			if isAgent(p.comm) {
				a.Session = st.SessionOfProcess(p.pid)
				break
			}
		}
	}
	return a
}

func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		a := args[0]
		name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "=")
		switch {
		case a == "--":
			return append(pos, args[1:]...), nil
		case !strings.HasPrefix(a, "-") || a == "-" || strings.ContainsAny(name, " \t") || name == "":
			pos = append(pos, a)
			args = args[1:]
			continue
		}
		n := 1
		if f := fs.Lookup(name); f != nil && !strings.Contains(a, "=") && len(args) > 1 {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				n = 2
			}
		}
		if err := fs.Parse(args[:n]); err != nil {
			return nil, err
		}
		args = args[n:]
	}
	return pos, nil
}

type multi []string

func (m *multi) String() string     { return fmt.Sprint(*m) }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func setupCmd(args []string) error {
	if slices.Contains(args, "--global") {
		fmt.Println("Connecting Callboard for every project")
		return setup.Global(os.Stdout, exe())
	}
	which := ""
	if len(args) > 0 {
		which = args[0]
	}
	if which != "" && which != "claude" && which != "codex" {
		return fmt.Errorf("setup connects claude or codex: callboard setup [claude|codex]")
	}
	st := store.Open(wd())
	fmt.Printf("Connecting Callboard in %s\n", st.Root)
	if which != "codex" {
		if err := setup.Claude(os.Stdout, st.Repo, exe()); err != nil {
			return err
		}
	}
	if which == "codex" || which == "" && setup.CodexInstalled() {
		fmt.Println("Codex:")
		return setup.Codex(os.Stdout, st.Repo)
	}
	return nil
}

func status(global bool) error {
	st := store.Open(wd())
	if !global && st.Off() {
		defer fmt.Println("· Callboard is switched off in this repo: the hooks say nothing and the lists can be read but not changed. callboard on switches it back on")
	}
	for _, k := range store.Kinds {
		if !global && st.Foreign(k) {
			defer fmt.Printf("· %s here has no Callboard items, so Callboard leaves it alone as someone else's file, and %ss can't be added until it is renamed or moved\n", k.File, k.Noun)
		}
	}
	gset, gmissing := setup.GlobalStatus()
	if gset && len(gmissing) == 0 {
		codex := ""
		if setup.CodexInstalled() {
			codex = ", Codex"
			if !setup.CodexHooksTrusted() {
				codex += " (approve its hooks with /hooks)"
			}
		}
		fmt.Println("✓ set up for every project: MCP server, hooks, merge driver" + codex)
		return nil
	}
	if global {
		fmt.Printf("✗ not set up for every project; missing: %s. Run: callboard setup --global\n", joinComma(gmissing))
		os.Exit(1)
	}
	done, missing := setup.Status(st.Repo)
	partly := !slices.Contains(missing, ".mcp.json")
	if set, cx := setup.CodexStatus(filepath.Join(st.Root, "AGENTS.md"), filepath.Join(st.Root, ".codex")); (set || setup.CodexInstalled()) && len(cx) > 0 {
		done, missing = false, append(missing, cx...)
	} else if set {
		defer fmt.Println("✓ Codex: AGENTS.md, MCP server, hooks (trust the folder and /hooks in Codex)")
	}
	switch {
	case done:
		fmt.Println("✓ set up here: MCP server, hooks, merge driver, link")
		return nil
	case gset:
		fmt.Printf("✗ set up for every project, but missing: %s. Run: callboard setup --global\n", joinComma(gmissing))
	case partly:
		fmt.Printf("✗ set up here, but missing: %s. Run: callboard setup\n", joinComma(missing))
	default:
		fmt.Println("✗ not set up. Run: callboard setup --global (every project), or callboard setup (this repo only)")
	}
	os.Exit(1)
	return nil
}

func joinComma(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

func switchOff(off bool) error {
	r := store.Open(wd()).Repo
	if r.Common == "" {
		return fmt.Errorf("not a git repo; Callboard is only switched off in one")
	}
	if off {
		if _, err := r.Git("config", "callboard.off", "true"); err != nil {
			return err
		}
		fmt.Printf("✓ Callboard is off in %s: the hooks say nothing and the lists can be read but not changed. Undo: callboard on\n", r.Name)
		return nil
	}
	r.Git("config", "--unset", "callboard.off")
	fmt.Printf("✓ Callboard is on in %s\n", r.Name)
	return nil
}

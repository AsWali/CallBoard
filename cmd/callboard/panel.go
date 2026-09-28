package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/store"
)

func panelFile(st *store.Store) string {
	h := sha1.Sum([]byte(st.Root))
	return filepath.Join(st.Shared(), "panels", hex.EncodeToString(h[:6])+".pid")
}

func panelPid(st *store.Store) int {
	b, err := os.ReadFile(panelFile(st))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid == 0 || !store.Alive(pid) {
		return 0
	}
	return pid
}

func markPanel(st *store.Store) func() {
	p := panelFile(st)
	os.MkdirAll(filepath.Dir(p), 0o755)
	me := strconv.Itoa(os.Getpid())
	os.WriteFile(p, []byte(me+"\n"), 0o644)
	return func() {
		if b, _ := os.ReadFile(p); strings.TrimSpace(string(b)) == me {
			os.Remove(p)
			closeWindow(p)
		}
	}
}

func panelCmd(args []string) error {
	text, err := openPanel(args)
	if err != nil {
		return err
	}
	fmt.Println(text)
	return nil
}

func openPanel(args []string) (string, error) {
	st := store.Open(wd())
	if len(args) > 0 && (args[0] == "off" || args[0] == "close") {
		pid := panelPid(st)
		if pid == 0 {
			return "✓ No panel is open for " + st.Name + ".", nil
		}
		p, err := os.FindProcess(pid)
		if err == nil {
			err = stopProcess(p)
		}
		if err != nil {
			return "", fmt.Errorf("couldn't close the panel (pid %d): %v", pid, err)
		}
		return "✓ Closed the panel for " + st.Name + ".", nil
	}
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return panelHelp, nil
	}
	if panelPid(st) != 0 {
		return "✓ The panel for " + st.Name + " is already open. Close it with: callboard panel off", nil
	}
	argv := append([]string{exe(), "watch", "--panel"}, args...)
	where, err := splitBeside(panelFile(st), st.Root, argv)
	if err != nil {
		return "", err
	}
	for i := 0; i < 40 && panelPid(st) == 0; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if panelPid(st) == 0 {
		return "", fmt.Errorf("asked %s to open the panel, but it didn't start. Run it yourself in a split or a new tab: callboard watch", where)
	}
	return "✓ Opened the panel " + where + ". q in it, or callboard panel off, closes it.", nil
}

const panelHelp = `callboard panel [WORDS]   open callboard watch beside the agent, in the same window when the terminal can split
callboard panel off       close it

In Claude Code: /callboard panel. In Codex, and in Claude Code too: !callboard panel.
WORDS pick the first tab, as in callboard show: todo, board, you, now, graph, view:NAME ...

It splits the window in tmux, Zellij, iTerm2, WezTerm, Kitty (with allow_remote_control on) and Windows
Terminal. Elsewhere it opens a window beside this one: Terminal on macOS, Ghostty, or your terminal on Linux.`

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func shellLine(dir string, argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = shq(a)
	}
	return "cd " + shq(dir) + " && exec " + strings.Join(q, " ")
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}

func has(name string) bool { _, err := exec.LookPath(name); return err == nil }

func splitBeside(pidFile, dir string, argv []string) (string, error) {
	env := os.Getenv
	switch {
	case env("TMUX") != "" && has("tmux"):
		a := []string{"split-window", "-h", "-d", "-l", "40%", "-c", dir}
		if p := env("TMUX_PANE"); p != "" {
			a = append(a, "-t", p)
		}
		return "on the right (tmux)", run("tmux", append(a, shellLine(dir, argv))...)
	case env("ZELLIJ") != "" && has("zellij"):
		return "on the right (Zellij)", run("zellij", append([]string{"run", "--direction", "right", "--cwd", dir, "--"}, argv...)...)
	case env("WEZTERM_PANE") != "" && has("wezterm"):
		pane := env("WEZTERM_PANE")
		err := run("wezterm", append([]string{"cli", "split-pane", "--pane-id", pane, "--right", "--percent", "40", "--cwd", dir, "--"}, argv...)...)
		if err == nil {
			run("wezterm", "cli", "activate-pane", "--pane-id", pane)
		}
		return "on the right (WezTerm)", err
	case env("KITTY_WINDOW_ID") != "":
		kitten := []string{"kitten", "@"}
		if !has("kitten") {
			kitten = []string{"kitty", "@"}
		}
		a := append(kitten[1:], "launch", "--location=vsplit", "--keep-focus", "--cwd", dir)
		if err := run(kitten[0], append(a, argv...)...); err != nil {
			return "", fmt.Errorf("couldn't open a split in Kitty (%v). Turn on its remote control: add allow_remote_control yes to kitty.conf, restart Kitty, then try again", err)
		}
		return "on the right (Kitty)", nil
	case env("TERM_PROGRAM") == "iTerm.app" && runtime.GOOS == "darwin":
		return "on the right (iTerm2)", iterm(env("ITERM_SESSION_ID"), dir, argv)
	case env("WT_SESSION") != "" && has("wt.exe"):
		err := run("wt.exe", append([]string{"-w", "0", "split-pane", "-V", "-s", "0.4", "-d", dir}, argv...)...)
		if err == nil {
			run("wt.exe", "-w", "0", "move-focus", "left")
		}
		return "on the right (Windows Terminal)", err
	case strings.EqualFold(env("TERM_PROGRAM"), "ghostty") && runtime.GOOS == "darwin":
		return "in a new Ghostty window", run("open", append([]string{"-na", "Ghostty", "--args", "--working-directory=" + dir, "-e"}, argv...)...)
	case runtime.GOOS == "darwin":
		return "in a Terminal window beside this one", appleTerminal(pidFile, dir, argv)
	case runtime.GOOS == "windows":
		c := exec.Command("cmd", append([]string{"/c", "start", ""}, argv...)...)
		c.Dir = dir
		return "in a new window", c.Run()
	}
	for _, t := range [][]string{
		{"ghostty", "--working-directory=" + dir, "-e"},
		{"x-terminal-emulator", "-e"},
		{"gnome-terminal", "--working-directory=" + dir, "--"},
		{"konsole", "--workdir", dir, "-e"},
		{"xfce4-terminal", "--working-directory=" + dir, "-x"},
		{"alacritty", "--working-directory", dir, "-e"},
		{"foot", "-D", dir},
		{"xterm", "-e"},
	} {
		if (env("DISPLAY") != "" || env("WAYLAND_DISPLAY") != "") && has(t[0]) {
			c := exec.Command(t[0], append(t[1:], argv...)...)
			c.Dir = dir
			if err := c.Start(); err != nil {
				return "", err
			}
			go c.Wait()
			return "in a new " + t[0] + " window", nil
		}
	}
	return "", errors.New("this terminal can't be split from outside. Run the panel yourself in a split or a new tab: callboard watch")
}

func osa(lang, script string) error {
	return run("osascript", "-l", lang, "-e", script)
}

func iterm(session, dir string, argv []string) error {
	_, id, _ := strings.Cut(session, ":")
	cmd := strings.ReplaceAll(strings.ReplaceAll("/bin/sh -c "+shq(shellLine(dir, argv)), `\`, `\\`), `"`, `\"`)
	return osa("AppleScript", `tell application "iTerm2"
	set target to missing value
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique id of s is "`+id+`" then set target to s
			end repeat
		end repeat
	end repeat
	if target is missing value then set target to current session of current window
	tell target to split vertically with default profile command "`+cmd+`"
	tell target to select
end tell`)
}

func appleTerminal(pidFile, dir string, argv []string) error {
	b, _ := json.Marshal(shellLine(dir, argv))
	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", `ObjC.import('AppKit');
const T = Application('Terminal');
const agent = T.windows.length ? T.windows[0].id() : null;
const a = agent ? T.windows.byId(agent).bounds() : null;
T.doScript(`+string(b)+`);
const panel = T.windows[0].id();
let undo = '';
if (a) {
	const screen = $.NSScreen.mainScreen.frame.size.width;
	const width = 560, gap = 6;
	let x = a.x + a.width + gap;
	if (x + width > screen) {
		const narrower = screen - width - gap - a.x;
		if (narrower >= 480) {
			T.windows.byId(agent).bounds = {x: a.x, y: a.y, width: narrower, height: a.height};
			undo = JSON.stringify({id: agent, bounds: a});
			x = a.x + narrower + gap;
		} else {
			x = Math.max(screen - width, 0);
		}
	}
	T.windows.byId(panel).bounds = {x: x, y: a.y, width: width, height: a.height};
	T.windows.byId(agent).index = 1;
}
JSON.stringify({panel: panel, agent: undo ? JSON.parse(undo) : null});`).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return os.WriteFile(strings.TrimSuffix(pidFile, ".pid")+".window", out, 0o644)
}

func closeWindow(pidFile string) {
	side := strings.TrimSuffix(pidFile, ".pid") + ".window"
	b, err := os.ReadFile(side)
	if err != nil {
		return
	}
	os.Remove(side)
	script := `const T = Application('Terminal'); const s = ` + strings.TrimSpace(string(b)) + `;
delay(0.3);
try { if (s.agent) T.windows.byId(s.agent.id).bounds = s.agent.bounds; } catch (e) {}
try { T.windows.byId(s.panel).close(); } catch (e) {}`
	c := exec.Command("osascript", "-l", "JavaScript", "-e", script)
	detach(c)
	c.Start()
}

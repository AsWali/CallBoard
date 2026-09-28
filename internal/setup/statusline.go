package setup

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const statusCmd = "callboard statusline"

func statusSettings() string { return filepath.Join(claudeDir(), "settings.json") }

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func unshq(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], `'\''`, `'`)
	}
	return s
}

func ourStatus(cmd string) (ours bool, theirs string) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(cmd), statusCmd)
	if !ok || rest != "" && rest[0] != ' ' {
		return false, ""
	}
	if w, ok := strings.CutPrefix(strings.TrimSpace(rest), "--below "); ok {
		return true, unshq(w)
	}
	return true, ""
}

func statusCommand(o *Obj) string {
	if sl := o.ChildIf("statusLine"); sl != nil {
		c, _ := sl.Get("command").(string)
		return c
	}
	return ""
}

func StatusLineState() string {
	m, _ := readJSON(statusSettings())
	if m == nil {
		return "off"
	}
	cur := statusCommand(m)
	switch ours, theirs := ourStatus(cur); {
	case ours && theirs != "":
		return "on, under your own"
	case ours:
		return "on"
	}
	return "off"
}

func StatusLineOn(w io.Writer, below bool) error {
	s := &Step{W: w}
	path := statusSettings()
	var cur string
	changed, err := editJSON(path, func(o *Obj) {
		cur = statusCommand(o)
		ours, _ := ourStatus(cur)
		switch {
		case ours:
		case cur == "":
			sl := o.Child("statusLine")
			sl.Set("type", "command")
			sl.Set("command", statusCmd)
		case below:
			o.Child("statusLine").Set("command", statusCmd+" --below "+shq(cur))
		}
	})
	ours, _ := ourStatus(cur)
	switch {
	case err != nil:
		s.bad("%s: %v", tilde(path), err)
	case ours || !changed && cur == "":
		s.same("Claude Code's status line already shows Callboard (%s)", tilde(path))
	case cur != "" && !below:
		s.same("You have your own status line (%s), so Callboard left it alone.", cur)
		fmt.Fprintln(w, "    To show Callboard's line under yours: callboard statusline on --below")
		return nil
	case cur != "":
		s.ok("Claude Code's status line: yours first, then Callboard's line (%s)", tilde(path))
	default:
		s.ok("Claude Code's status line shows Callboard (%s)", tilde(path))
	}
	if s.Err {
		return fmt.Errorf("the status line wasn't changed; see ✗ above")
	}
	fmt.Fprintln(w, "    It shows in new sessions and after the next message. Undo: callboard statusline off")
	fmt.Fprintln(w, "    Codex's status line only takes its own built-in items, so Codex can't show it; use callboard panel there.")
	return nil
}

func StatusLineOff(w io.Writer) error {
	s := &Step{W: w}
	path := statusSettings()
	var cur, theirs string
	var ours bool
	changed, err := editJSON(path, func(o *Obj) {
		cur = statusCommand(o)
		if ours, theirs = ourStatus(cur); !ours {
			return
		}
		if theirs != "" {
			o.Child("statusLine").Set("command", theirs)
		} else {
			o.Delete("statusLine")
		}
	})
	switch {
	case err != nil:
		s.bad("%s: %v", tilde(path), err)
	case !ours && cur != "":
		s.same("The status line is your own, not Callboard's; left alone")
	case !changed:
		s.same("Callboard isn't in Claude Code's status line")
	case theirs != "":
		s.ok("Claude Code's status line is your own again (%s)", tilde(path))
	default:
		s.ok("Removed Callboard's status line (%s)", tilde(path))
	}
	if s.Err {
		return fmt.Errorf("the status line wasn't changed; see ✗ above")
	}
	return nil
}

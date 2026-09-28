package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AsWali/CallBoard/internal/store"
)

var readers = map[string]bool{
	"cat": true, "head": true, "tail": true, "less": true, "more": true, "bat": true, "nl": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "ack": true,
	"sed": true, "awk": true, "gawk": true, "cut": true, "sort": true, "wc": true, "tac": true,
	"type": true, "get-content": true, "gc": true, "select-string": true, "findstr": true,
}

var plainWords = regexp.MustCompile(`^[\pL\pN _.:@/-]+$`)

func mentionsList(raw []byte) bool {
	return bytes.Contains(raw, []byte(".md"))
}

func guardRead(st *store.Store, in hookIn) error {
	if st.Off() || !st.InUse() || len(in.ToolInput) == 0 {
		return nil
	}
	var args map[string]any
	if json.Unmarshal(in.ToolInput, &args) != nil {
		return nil
	}
	dir := in.Cwd
	if dir == "" {
		dir = wd()
	}
	k, find, ok := readsList(st, dir, args)
	if !ok {
		return nil
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"hookSpecificOutput": map[string]string{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": st.ReadInstead(k, find),
		},
	})
}

func readsList(st *store.Store, dir string, args map[string]any) (store.Kind, string, bool) {
	str := func(name string) string { s, _ := args[name].(string); return s }
	find := ""
	if p := str("pattern"); plainWords.MatchString(p) {
		find = p
	}
	for _, name := range []string{"file_path", "path", "notebook_path"} {
		if p := str(name); p != "" {
			if k, ok := st.ListFile(abs(dir, p)); ok {
				return k, find, true
			}
		}
	}
	if g := str("glob"); g != "" {
		if k, ok := st.ListFile(abs(dir, filepath.Join(str("path"), g))); ok {
			return k, find, true
		}
	}
	cmd := str("command")
	if parts, ok := args["command"].([]any); ok {
		var words []string
		for _, p := range parts {
			if s, ok := p.(string); ok {
				words = append(words, s)
			}
		}
		if n := len(words); n >= 3 && (words[n-2] == "-c" || words[n-2] == "-lc") {
			cmd = words[n-1]
		} else {
			cmd = strings.Join(words, " ")
		}
	}
	if cmd == "" {
		return store.Kind{}, "", false
	}
	return shellReadsList(st, dir, cmd)
}

func shellReadsList(st *store.Store, dir, cmd string) (store.Kind, string, bool) {
	for _, words := range shellSegments(cmd) {
		for len(words) > 0 && (strings.Contains(words[0], "=") || words[0] == "sudo" || words[0] == "command" || words[0] == "xargs") {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		if words[0] == "cd" && len(words) > 1 {
			dir = abs(dir, words[1])
			continue
		}
		if !readers[strings.ToLower(filepath.Base(words[0]))] {
			continue
		}
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-") {
				continue
			}
			if k, ok := st.ListFile(abs(dir, w)); ok {
				find := ""
				if b := filepath.Base(words[0]); strings.Contains(b, "grep") || b == "rg" || b == "ag" || b == "ack" {
					for _, x := range words[1:] {
						if !strings.HasPrefix(x, "-") && x != w {
							if plainWords.MatchString(x) {
								find = x
							}
							break
						}
					}
				}
				return k, find, true
			}
		}
	}
	return store.Kind{}, "", false
}

func shellSegments(s string) [][]string {
	var segs [][]string
	var words []string
	var cur strings.Builder
	quote := rune(0)
	word := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	seg := func() {
		word()
		if len(words) > 0 {
			segs = append(segs, words)
			words = nil
		}
	}
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
		case r == '|' || r == ';' || r == '&' || r == '\n':
			seg()
		case r == ' ' || r == '\t' || r == '<' || r == '>' || r == '(' || r == ')':
			word()
		default:
			cur.WriteRune(r)
		}
	}
	seg()
	return segs
}

func abs(dir, p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(h, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return store.RealPath(p)
}

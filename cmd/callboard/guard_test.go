package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/store"
)

func TestReadsList(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("# B\n\n- [ ] Parse it <!-- id:B-aaaa by:you at:2026-09-01 -->\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	st := store.Open(dir)
	cases := []struct {
		args map[string]any
		want bool
		find string
	}{
		{map[string]any{"file_path": filepath.Join(dir, "backlog.md")}, true, ""},
		{map[string]any{"file_path": filepath.Join(dir, "main.go")}, false, ""},
		{map[string]any{"pattern": "Parse", "path": filepath.Join(dir, "backlog.md")}, true, "Parse"},
		{map[string]any{"pattern": "Parse", "path": dir}, false, ""},
		{map[string]any{"pattern": "Parse", "path": dir, "glob": "backlog.md"}, true, "Parse"},
		{map[string]any{"command": "cat backlog.md"}, true, ""},
		{map[string]any{"command": "cd sub && cat ../backlog.md | head -5"}, true, ""},
		{map[string]any{"command": "grep -n \"Parse it\" backlog.md"}, true, "Parse it"},
		{map[string]any{"command": "rg -n 'T(ODO|BD)' backlog.md"}, true, ""},
		{map[string]any{"command": []any{"bash", "-lc", "sed -n 1,20p backlog.md"}}, true, ""},
		{map[string]any{"command": "git diff backlog.md"}, false, ""},
		{map[string]any{"command": "callboard show B-aaaa"}, false, ""},
		{map[string]any{"command": "cat main.go"}, false, ""},
		{map[string]any{"command": "echo backlog.md"}, false, ""},
		{map[string]any{"command": "cat requests.md"}, true, ""},
	}
	for _, c := range cases {
		k, find, ok := readsList(st, dir, c.args)
		if ok != c.want || find != c.find {
			t.Errorf("%v: got %v %q (%s), want %v %q", c.args, ok, find, k.File, c.want, c.find)
		}
	}
	text := st.ReadInstead(store.Backlog, "")
	for _, want := range []string{"backlog.md is kept by Callboard", "B-aaaa", "Parse it"} {
		if !strings.Contains(text, want) {
			t.Errorf("ReadInstead lacks %q:\n%s", want, text)
		}
	}
}

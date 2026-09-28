package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusLine(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "settings.json")
	read := func() string { b, _ := os.ReadFile(path); return string(b) }

	StatusLineOn(io.Discard, false)
	if !strings.Contains(read(), `"command": "callboard statusline"`) || StatusLineState() != "on" {
		t.Fatalf("on with no status line: %s", read())
	}
	StatusLineOff(io.Discard)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("off left %s", read())
	}

	mine := `{"model": "opus", "statusLine": {"type": "command", "command": "echo \"it's mine\"", "padding": 0}}`
	os.WriteFile(path, []byte(mine), 0o644)
	StatusLineOn(io.Discard, false)
	if read() != mine {
		t.Fatalf("on without --below changed the user's own status line: %s", read())
	}
	StatusLineOn(io.Discard, true)
	if StatusLineState() != "on, under your own" {
		t.Fatalf("state after --below: %s", read())
	}
	StatusLineOn(io.Discard, true)
	if strings.Count(read(), "--below") != 1 {
		t.Fatalf("on --below twice wrapped it twice: %s", read())
	}
	StatusLineOff(io.Discard)
	m, _ := readJSON(path)
	if got := statusCommand(m); got != `echo "it's mine"` || !strings.Contains(read(), `"padding": 0`) || !strings.Contains(read(), `"model": "opus"`) {
		t.Fatalf("off didn't give back the user's own status line: %s", read())
	}
	StatusLineOff(io.Discard)
	if statusCommand(m) != `echo "it's mine"` {
		t.Fatal("off touched the user's own status line")
	}
}

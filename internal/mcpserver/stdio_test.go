package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStdioSurvivesBadLines(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "-C", dir, "init", "-q").Run()
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("# Backlog\n"), 0o644)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go New(dir, "test").Run(ctx, Stdio(inR, outW))

	replies := make(chan map[string]any, 10)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("not JSON on stdout: %q", sc.Text())
			}
			replies <- m
		}
	}()
	send := func(l string) { io.WriteString(inW, l+"\n") }
	next := func() map[string]any {
		select {
		case m := <-replies:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("no reply")
			return nil
		}
	}
	code := func(m map[string]any) float64 {
		e, _ := m["error"].(map[string]any)
		c, _ := e["code"].(float64)
		return c
	}
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if m := next(); m["id"] != 1.0 || m["result"] == nil {
		t.Fatalf("initialize: %v", m)
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	send(`{not json`)
	if m := next(); code(m) != -32700 || m["id"] != nil {
		t.Fatalf("bad JSON: %v", m)
	}
	send(`{"id":7,"method":"tools/list"}`)
	if m := next(); code(m) != -32600 || m["id"] != 7.0 {
		t.Fatalf("no jsonrpc: %v", m)
	}
	send(``)
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if m := next(); m["id"] != 2.0 || m["result"] == nil {
		t.Fatalf("tools/list after bad lines: %v", m)
	}
}

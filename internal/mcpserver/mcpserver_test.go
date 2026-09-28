package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTools(t *testing.T) {
	dir := t.TempDir()
	if err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").Run(); err != nil {
		t.Skip("no git")
	}
	os.WriteFile(filepath.Join(dir, "backlog.md"), []byte("## v1\n"), 0o644)

	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	srv := New(dir, "test")
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools.Tools {
		names = append(names, tl.Name)
		if tl.OutputSchema != nil {
			t.Errorf("%s has an output schema; replies are text only", tl.Name)
		}
	}
	if strings.Join(names, ",") != "add,answer,assume,claim,edit,list,lists,news,resolve,section,tick,view" {
		t.Fatalf("tools %v", names)
	}

	call := func(name string, args map[string]any) (string, bool) {
		t.Helper()
		r, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if r.StructuredContent != nil {
			t.Errorf("%s gave structured content", name)
		}
		var b strings.Builder
		for _, c := range r.Content {
			b.WriteString(c.(*mcp.TextContent).Text + "\n")
		}
		return b.String(), r.IsError
	}
	keyOf := func(s string) string { return strings.SplitN(strings.Fields(s)[0], "@", 2)[0] }
	has := func(got string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Fatalf("want %q in:\n%s", w, got)
			}
		}
	}

	added, _ := call("add", map[string]any{"title": "Merge driver"})
	key := keyOf(added)
	if !strings.HasPrefix(key, "B-") {
		t.Fatalf("add gave %q", added)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "backlog.md"))
	if !strings.Contains(string(b), "- [ ] Merge driver <!-- id:"+key+" by:claude") {
		t.Fatalf("file:\n%s", b)
	}

	got, bad := call("edit", map[string]any{"key": key, "title": "Merge driver v2", "section": "Now", "fields": map[string]string{"prio": "high"}})
	if bad {
		t.Fatalf("edit: %s", got)
	}
	has(got, key+"@", "Merge driver v2", "prio:high")
	if got, bad := call("edit", map[string]any{"key": key}); !bad || !strings.Contains(got, "nothing to change") {
		t.Fatalf("empty edit: %s", got)
	}
	if got, bad := call("edit", map[string]any{"key": key, "add_note": "Plan: reuse the board merge"}); bad {
		t.Fatalf("add_note: %s", got)
	}
	if got, bad := call("edit", map[string]any{"key": key, "add_note": "Then the attributes file"}); bad {
		t.Fatalf("second add_note: %s", got)
	}
	got, _ = call("list", map[string]any{"key": key})
	has(got, "Plan: reuse the board merge", "Then the attributes file", "added a note")
	call("edit", map[string]any{"key": key, "notes": "Only this"})
	got, _ = call("list", map[string]any{"key": key})
	if notes, _, _ := strings.Cut(got, "History"); !strings.Contains(notes, "Only this") || strings.Contains(notes, "Plan: reuse") {
		t.Fatalf("notes didn't replace: %s", got)
	}

	got, _ = call("tick", map[string]any{"key": key})
	has(got, "[x] "+key)

	got, _ = call("list", map[string]any{"status": "all"})
	has(got, "1 done", "## Now", "[x] "+key)
	got, _ = call("list", nil)
	has(got, "nothing to show")

	if _, bad := call("tick", map[string]any{"key": "B-nope"}); !bad {
		t.Fatal("unknown key should be an error")
	}

	q, _ := call("add", map[string]any{"title": "Which port?", "kind": "question", "options": []string{"From the path", "Always 4700"}, "recommended": 1})
	has(q, "1) From the path (recommended)")
	qk := keyOf(q)
	wt, _ := call("add", map[string]any{"title": "Serve it", "needs": []string{qk}, "body": []string{"use the flag"}})
	has(wt, "waits on "+qk)
	got, _ = call("list", map[string]any{"kind": "task", "status": "ready"})
	has(got, "nothing to show")
	got, _ = call("list", map[string]any{"key": keyOf(wt)})
	has(got, "Serve it", "Waits on", "Which port?", "use the flag", "History")
	if got, bad := call("assume", map[string]any{"key": qk, "option": 1}); bad {
		t.Fatalf("assume: %s", got)
	}
	got, _ = call("list", map[string]any{"kind": "task", "status": "ready"})
	has(got, "Serve it", "use the flag")
	got, _ = call("answer", map[string]any{"key": qk, "answer": "2", "why": "simpler"})
	has(got, "answered: Always 4700", "no longer waiting on it: "+keyOf(wt))

	got, _ = call("claim", map[string]any{"key": keyOf(wt)})
	has(got, "claimed by claude")
	got, _ = call("claim", map[string]any{"key": keyOf(wt), "release": true})
	has(got, "released")

	got, bad = call("view", map[string]any{"name": "Most important", "layout": "table", "sort": []string{"-prio"}})
	if bad {
		t.Fatalf("view: %s", got)
	}
	has(got, "Most important: table of backlog", "shows: Table of backlog.md")
	got, _ = call("view", nil)
	has(got, "Most important")
	if got, bad := call("view", map[string]any{"name": "x", "group": "priority"}); !bad || !strings.Contains(got, `did you mean "prio"`) {
		t.Fatalf("typo view: %s", got)
	}

	got, _ = call("news", nil)
	if got == "" {
		t.Fatal("no news text")
	}

	for i := 0; i < 60; i++ {
		call("add", map[string]any{"title": fmt.Sprintf("Paged item %02d", i), "section": "Paging"})
	}
	got, _ = call("list", map[string]any{"section": "Paging"})
	has(got, "Paged item 00", "Paged item 49", "Showing items 1–50 of 60. The next ones: from 50.")
	if strings.Contains(got, "Paged item 50") {
		t.Fatalf("first page went past 50:\n%s", got)
	}
	got, _ = call("list", map[string]any{"section": "Paging", "from": 50})
	has(got, "## Paging", "Paged item 50", "Paged item 59", "Showing items 51–60 of 60, the last ones.")
	got, _ = call("list", map[string]any{"section": "Paging", "limit": 100})
	if strings.Contains(got, "Showing") || !strings.Contains(got, "Paged item 59") {
		t.Fatalf("limit 100 should show all 60 without a footer:\n%s", got)
	}
}

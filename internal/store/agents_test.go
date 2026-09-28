package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentsFromClaudeCodeAndCodex(t *testing.T) {
	s := repo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	write := func(p, text string) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cc := "11111111-aaaa-bbbb-cccc-000000000001"
	write(filepath.Join(home, ".claude", "sessions", fmt.Sprint(os.Getpid())+".json"),
		fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":%q,"name":"x-1f","nameSource":"derived","status":"busy","startedAt":%d}`, os.Getpid(), cc, s.Root, time.Now().Add(-time.Hour).UnixMilli()))
	write(filepath.Join(home, ".claude", "projects", "-x", cc+".jsonl"), strings.Join([]string{
		`{"type":"user","message":{"content":"` + strings.Repeat("long ", 30000) + `"}}`,
		`{"type":"custom-title","customTitle":"page work","sessionId":"x"}`,
		`{"type":"agent-color","agentColor":"cyan","sessionId":"x"}`,
		`{"type":"ai-title","aiTitle":"Build the page","sessionId":"x"}`,
		`{"type":"last-prompt","lastPrompt":"make it blue","sessionId":"x"}`,
	}, "\n")+"\n")

	day := filepath.Join(home, ".codex", "sessions", filepath.FromSlash(time.Now().Format("2006/01/02")))
	cx := "01a0e09a-647d-78a0-9c33-7b90e9a760f6"
	meta := func(id, cwd, extra string) string {
		return fmt.Sprintf(`{"timestamp":"2026-09-27T02:03:30Z","type":"session_meta","payload":{"id":%q,"timestamp":"2026-09-27T02:03:29Z","cwd":%q,"originator":"codex_exec"%s}}`, id, cwd, extra) + "\n"
	}
	write(filepath.Join(day, "rollout-a-"+cx+".jsonl"), meta(cx, s.Root, `,"thread_source":"user"`)+
		`{"type":"event_msg","payload":{"type":"user_message","message":"fix the tabs"}}`+"\n")
	write(filepath.Join(day, "rollout-b-r.jsonl"), meta("01a0-review", s.Root, `,"thread_source":"guardian_review","parent_thread_id":"`+cx+`"`))
	write(filepath.Join(day, "rollout-c-o.jsonl"), meta("01a0-other", "/somewhere/else", ""))
	old := time.Now().Add(-10 * time.Minute)
	os.Chtimes(filepath.Join(day, "rollout-a-"+cx+".jsonl"), old, old)
	write(filepath.Join(home, ".codex", "session_index.jsonl"), `{"id":"`+cx+`","thread_name":"Fix word splitting"}`+"\n")

	as := s.Agents(8)
	if len(as) != 2 {
		t.Fatalf("agents: %+v", as)
	}
	c, x := as[0], as[1]
	if c.Tool != "claude" || !c.Live || c.Status != "busy" || c.Name != "page work" || c.Color != "cyan" || c.Title != "Build the page" || c.Prompt != "make it blue" {
		t.Fatalf("claude: %+v", c)
	}
	if x.Tool != "codex" || x.Live || x.Title != "Fix word splitting" || x.Prompt != "fix the tabs" || x.How != "exec" || x.Subagents != 0 {
		t.Fatalf("codex: %+v", x)
	}
}

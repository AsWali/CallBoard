package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AsWali/CallBoard/internal/overview"
	"github.com/AsWali/CallBoard/internal/store"
)

type hookIn struct {
	SessionID  string          `json:"session_id"`
	Source     string          `json:"source"`
	AgentID    string          `json:"agent_id"`
	AgentType  string          `json:"agent_type"`
	ToolName   string          `json:"tool_name"`
	Model      string          `json:"model"`
	Transcript string          `json:"transcript_path"`
	Prompt     string          `json:"prompt"`
	Cwd        string          `json:"cwd"`
	ToolInput  json.RawMessage `json:"tool_input"`
}

func (in hookIn) session() string {
	if in.AgentID != "" {
		return in.SessionID + ":" + in.AgentID
	}
	return in.SessionID
}

var hookInput = sync.OnceValue(func() (in hookIn) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	ch := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(io.LimitReader(os.Stdin, 4<<20)); ch <- b }()
	select {
	case b := <-ch:
		json.Unmarshal(b, &in)
	case <-time.After(500 * time.Millisecond):
	}
	return
})

func prime() error {
	st := store.Open(wd())
	in := hookInput()
	if in.SessionID != "" {
		linkAgent(st, in.SessionID)
		by := agentName(in)
		if in.Source != "" && in.Source != "startup" {
			by += " (" + in.Source + ")"
		}
		st.Log(store.Event{By: by, What: "session", Session: in.SessionID, Title: in.Source})
	}
	out, err := st.Prime(store.Actor{By: agentName(in), Session: in.session()})
	if err != nil {
		return err
	}
	fmt.Print(out)

	if b := st.Branch(); st.Common != "" && b != "" && b != st.DefaultBranch() {
		if p := overview.BuildBranches(st.Repo, b, b); len(p.Branches) == 1 && len(p.Branches[0].Conflicts) > 0 {
			fmt.Printf("Merging %s into %s now: %s. Whoever merges picks one version of each; if it matters, ask the human. Since %s forked, %s got %s.\n", b, p.Main, overview.Clashes(p.Branches[0]), b, p.Main, overview.Since(p.Branches[0]))
		}
	}
	return nil
}

func hook(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("hook prompt|tool|read|subagent (Claude Code and Codex call these)")
	}
	in := hookInput()
	if args[0] == "read" && !mentionsList(in.ToolInput) {
		return nil
	}
	st := store.Open(wd())
	switch args[0] {
	case "prompt":
		linkAgent(st, in.SessionID)
		fmt.Print(store.FormatNews(st.News(in.session())))
	case "tool":
		touchClaims(st, in.SessionID)
		if text := store.FormatNews(st.News(in.session())); text != "" {
			return hookContext("PostToolUse", text)
		}
	case "read":
		return guardRead(st, in)
	case "subagent":
		st.Log(store.Event{By: agentName(in) + " (" + orDefault(in.AgentType, "subagent") + ")", What: "session", Session: in.session(), Title: "subagent"})
		out, err := st.Prime(store.Actor{By: agentName(in), Session: in.session()})
		if err != nil {
			return err
		}
		return hookContext("SubagentStart", out)
	default:
		return fmt.Errorf("no hook %q: prompt, tool, read or subagent", args[0])
	}
	return nil
}

func hookContext(event, text string) error {
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": text},
	})
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func touchClaims(st *store.Store, session string) {
	claims := st.Claims()
	if len(claims) == 0 {
		return
	}
	sessions := map[string]bool{}
	for _, c := range claims {
		sessions[c.Session] = true
	}
	if session != "" && sessions[session] {
		st.Touch(session)
		return
	}
	for _, p := range ancestors(6) {
		if sess := "mcp-" + strconv.Itoa(p.pid); sessions[sess] {
			st.Touch(sess)
			return
		}
	}
}

func linkAgent(st *store.Store, session string) {
	if session == "" {
		return
	}
	for _, p := range ancestors(6) {
		if isAgent(p.comm) {
			st.LinkProcess(p.pid, session)
			return
		}
	}
}

func isAgent(comm string) bool {
	n := strings.ToLower(filepath.Base(comm))
	return strings.Contains(n, "claude") || strings.Contains(n, "codex")
}

func agentName(in hookIn) string {
	if b := os.Getenv("CALLBOARD_BY"); b != "" {
		return b
	}
	if t := in.Transcript; t != "" {
		if strings.HasPrefix(filepath.Base(t), "rollout-") || strings.Contains(filepath.ToSlash(t), "/.codex/") {
			return "codex"
		}
		return "claude"
	}
	if m := strings.ToLower(in.Model); m != "" && !strings.HasPrefix(m, "claude") && !strings.Contains(m, "opus") && !strings.Contains(m, "sonnet") && !strings.Contains(m, "haiku") {
		return "codex"
	}
	if b := store.By(); b != "you" {
		return b
	}
	return "claude"
}

func ancestors(n int) []proc {
	var out []proc
	pid := os.Getppid()
	for i := 0; i < n && pid > 1; i++ {
		ppid, comm, ok := parentOf(pid)
		if !ok {
			break
		}
		out = append(out, proc{pid, comm})
		pid = ppid
	}
	return out
}

type proc struct {
	pid  int
	comm string
}

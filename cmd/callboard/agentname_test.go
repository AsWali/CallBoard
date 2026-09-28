package main

import "testing"

func TestAgentName(t *testing.T) {
	t.Setenv("CALLBOARD_BY", "")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CODEX_THREAD_ID", "")
	for _, c := range []struct {
		in   hookIn
		want string
	}{
		{hookIn{Transcript: "/Users/x/.codex/sessions/2026/09/28/rollout-2026-09-28T21-59-57-abc.jsonl", Model: "gpt-5.6-luna"}, "codex"},
		{hookIn{Transcript: "/srv/codex-home/sessions/rollout-1.jsonl"}, "codex"},
		{hookIn{Transcript: "/Users/x/.claude/projects/-Users-x-app/d5b7.jsonl"}, "claude"},
		{hookIn{Transcript: "/Users/x/.claude/projects/-Users-x-app/d5b7.jsonl", Model: "arn:aws:bedrock:eu-west-1:1:application-inference-profile/abc"}, "claude"},
		{hookIn{Model: "gpt-5.6-luna"}, "codex"},
		{hookIn{}, "claude"},
	} {
		if got := agentName(c.in); got != c.want {
			t.Errorf("%+v: got %s, want %s", c.in, got, c.want)
		}
	}
}

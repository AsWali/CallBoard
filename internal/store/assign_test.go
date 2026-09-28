package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
)

func TestClaimRefusesItemsForOthers(t *testing.T) {
	s := repo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	mine, _ := s.Add(Backlog, board.New{Title: "Deploy it"}, claude)
	s.SetFields(mine.Key, [][2]string{{ForField, "you"}}, claude)
	other, _ := s.Add(Backlog, board.New{Title: "Port it"}, claude)
	s.SetFields(other.Key, [][2]string{{ForField, "codex"}}, claude)
	free, _ := s.Add(Backlog, board.New{Title: "Test it"}, claude)
	s.SetFields(free.Key, [][2]string{{ForField, "anyone"}}, claude)

	if _, err := s.ClaimItem(mine.Key, "claude", "s1", false); err == nil || !strings.Contains(err.Error(), "for the human") {
		t.Fatalf("claimed the human's task: %v", err)
	}
	if _, err := s.ClaimItem(other.Key, "claude", "s1", false); err == nil || !strings.Contains(err.Error(), "for codex") {
		t.Fatalf("claimed codex's task: %v", err)
	}
	if _, err := s.ClaimItem(other.Key, "codex", "s2", false); err != nil {
		t.Fatalf("codex can't claim its own task: %v", err)
	}
	if _, err := s.ClaimItem(free.Key, "claude", "s1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimItem(mine.Key, "claude", "s1", true); err != nil {
		t.Fatalf("force: %v", err)
	}
	if _, err := s.ClaimItem(mine.Key, "you", "", true); err != nil {
		t.Fatalf("the human: %v", err)
	}
	v, _ := s.View()
	if next := v.NextUp(claude); next != nil {
		t.Fatalf("next up offers %s", next.Key)
	}
}

func TestItemsForSeveralAndNextUpForMe(t *testing.T) {
	s := repo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	s.Add(Backlog, board.New{Title: "Anyone's"}, claude)
	both, _ := s.Add(Backlog, board.New{Title: "Either agent"}, claude)
	s.SetFields(both.Key, [][2]string{{ForField, "codex or claude"}}, claude)
	human, _ := s.Add(Backlog, board.New{Title: "Sign it"}, claude)
	s.SetFields(human.Key, [][2]string{{ForField, "you"}}, claude)
	codex := Actor{By: "codex", Session: "c1"}
	v, _ := s.View()
	for _, a := range []Actor{claude, codex} {
		if next := v.NextUp(a); next == nil || next.Key != both.Key {
			t.Fatalf("%s: next up %v, want the item meant for it", a.By, next)
		}
	}
	if next := v.NextUp(Actor{By: "you"}); next == nil || next.Key != human.Key {
		t.Fatalf("human: next up %v", next)
	}
	if _, err := s.ClaimItem(both.Key, "codex", "c1", false); err != nil {
		t.Fatal(err)
	}
	if s.ForSomeoneElse("claude, codex", Actor{By: "gemini", Session: "g"}) != true {
		t.Fatal("gemini may take an item for claude and codex")
	}
	it := v.Item(Backlog, v.Files[Backlog.Name].Find(both.Key))
	if got := FormatItem(it); !strings.Contains(got, "for codex or claude") {
		t.Fatalf("format: %s", got)
	}
	if ForText("you, codex") != "the human or codex" {
		t.Fatal(ForText("you, codex"))
	}
}

func TestClaimLastsWhileSessionRuns(t *testing.T) {
	s := repo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	a, _ := s.Add(Backlog, board.New{Title: "Alpha"}, claude)
	b, _ := s.Add(Backlog, board.New{Title: "Beta"}, claude)
	s.ClaimItem(a.Key, "claude", "live", false)
	s.ClaimItem(b.Key, "claude", "gone", false)
	old := time.Now().Add(-2 * ClaimTTL).Format(time.RFC3339)
	s.withClaims(func(m map[string]Claim) error {
		for k, c := range m {
			c.Seen = old
			m[k] = c
		}
		return nil
	})
	f := filepath.Join(home, ".claude", "sessions", fmt.Sprint(os.Getpid())+".json")
	os.MkdirAll(filepath.Dir(f), 0o755)
	j, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "sessionId": "live", "cwd": s.Root})
	os.WriteFile(f, j, 0o644)
	c := s.Claims()
	if _, ok := c[strings.ToUpper(a.Key)]; !ok {
		t.Fatal("claim of a running session expired")
	}
	if _, ok := c[strings.ToUpper(b.Key)]; ok {
		t.Fatal("claim of a finished session kept")
	}
}

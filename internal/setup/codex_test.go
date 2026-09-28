package setup

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/gitx"
)

func TestCodexSetupKeepsYourFiles(t *testing.T) {
	root := t.TempDir()
	agents := "# Agents\n\nUse tabs.\n"
	toml := "model = \"x\"\n\n[mcp_servers.other]\ncommand = \"other\"\n"
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(agents), 0o644)
	os.MkdirAll(filepath.Join(root, ".codex"), 0o755)
	os.WriteFile(filepath.Join(root, ".codex", "config.toml"), []byte(toml), 0o644)
	r := gitx.Repo{Root: root}
	for i := 0; i < 2; i++ {
		if err := Codex(io.Discard, r); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.HasPrefix(string(b), agents+"\n"+agentsStart) || strings.Count(string(b), agentsStart) != 1 {
		t.Fatalf("AGENTS.md:\n%s", b)
	}
	c, _ := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if !strings.HasPrefix(string(c), toml) || strings.Count(string(c), tomlHead) != 1 {
		t.Fatalf("config.toml:\n%s", c)
	}
	if set, missing := CodexStatus(filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".codex")); !set || len(missing) > 0 {
		t.Fatal(set, missing)
	}
	disconnectCodex(&Step{W: io.Discard}, filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".codex"), relTo(root))
	b, _ = os.ReadFile(filepath.Join(root, "AGENTS.md"))
	c, _ = os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if string(b) != agents || string(c) != toml {
		t.Fatalf("after disconnect:\n%s\n---\n%s", b, c)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatal("hooks.json left behind")
	}
}

package setup

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGlobal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0o755)

	os.WriteFile(filepath.Join(bin, "claude"), []byte(`#!/bin/sh
case "$2" in
add) echo '{"mcpServers":{"callboard":{"command":"callboard","args":["mcp"]}}}' > "$CLAUDE_CONFIG_DIR/.claude.json" ;;
remove) echo '{}' > "$CLAUDE_CONFIG_DIR/.claude.json" ;;
esac
`), 0o755)
	os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\n"), 0o755)
	if runtime.GOOS == "windows" {
		os.WriteFile(filepath.Join(bin, "claude.bat"), []byte("@echo off\r\n"+
			`if "%2"=="add" echo {"mcpServers":{"callboard":{"command":"callboard","args":["mcp"]}}}> "%CLAUDE_CONFIG_DIR%\.claude.json"`+"\r\n"+
			`if "%2"=="remove" echo {}> "%CLAUDE_CONFIG_DIR%\.claude.json"`+"\r\n"), 0o755)
		os.WriteFile(filepath.Join(bin, "codex.bat"), []byte("@echo off\r\n"), 0o755)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Join(home, ".local", "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.MkdirAll(filepath.Join(home, "codex"), 0o755)
	os.MkdirAll(filepath.Join(home, "claude"), 0o755)
	mine := "model = \"x\"\n"
	os.WriteFile(filepath.Join(home, "codex", "config.toml"), []byte(mine), 0o644)

	exe := filepath.Join(home, "callboard-build")
	os.WriteFile(exe, []byte("x"), 0o755)
	var first strings.Builder
	if err := Global(&first, exe); err != nil {
		t.Fatalf("%v\n%s", err, first.String())
	}
	if set, missing := GlobalStatus(); !set || len(missing) > 0 {
		t.Fatalf("after setup: %v %v", set, missing)
	}
	var out strings.Builder
	if err := Global(&out, exe); err != nil || strings.Contains(out.String(), "✓") {
		t.Fatalf("a second run should change nothing: %v\n%s", err, out.String())
	}
	b, _ := os.ReadFile(filepath.Join(home, ".config", "git", "attributes"))
	if !strings.Contains(string(b), "backlog.md merge=callboard") {
		t.Fatalf("attributes: %s", b)
	}
	if _, ours := ourSlash(); !ours {
		t.Fatal("no /callboard command")
	}
	if err := DisconnectGlobal(io.Discard); err != nil {
		t.Fatal(err)
	}
	if set, _ := GlobalStatus(); set {
		t.Fatal("still set after disconnect")
	}
	if there, _ := ourSlash(); there {
		t.Fatal("/callboard left behind")
	}
	if b, _ := os.ReadFile(filepath.Join(home, "codex", "config.toml")); string(b) != mine {
		t.Fatalf("codex config not restored: %q", b)
	}
}

func TestGlobalWithCodexOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\n"), 0o755)
	if runtime.GOOS == "windows" {
		os.WriteFile(filepath.Join(bin, "codex.bat"), []byte("@echo off\r\n"), 0o755)
	}
	git, _ := exec.LookPath("git")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+filepath.Join(home, ".local", "bin")+string(os.PathListSeparator)+filepath.Dir(git))
	if _, err := exec.LookPath("claude"); err == nil {
		t.Skip("claude is in git's folder")
	}
	os.MkdirAll(filepath.Join(home, "codex"), 0o755)

	exe := filepath.Join(home, "callboard-build")
	os.WriteFile(exe, []byte("x"), 0o755)
	var out strings.Builder
	if err := Global(&out, exe); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "✗") || strings.Contains(out.String(), "Claude") {
		t.Fatalf("setup without Claude Code should skip it quietly:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, "claude")); err == nil {
		t.Fatal("made a Claude Code folder without Claude Code")
	}
	if set, missing := GlobalStatus(); !set || len(missing) > 0 {
		t.Fatalf("status with Codex only: %v %v", set, missing)
	}
}

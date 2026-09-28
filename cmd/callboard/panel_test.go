package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/store"
)

func TestShellLine(t *testing.T) {
	dir := t.TempDir() + "/it's here"
	os.MkdirAll(dir, 0o755)
	line := shellLine(dir, []string{"sh", "-c", `pwd; echo "$0"`, "a b'c"})
	out, err := exec.Command("sh", "-c", line).Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(out)), "\n"); len(got) != 2 || !strings.HasSuffix(got[0], "/it's here") || got[1] != "a b'c" {
		t.Fatalf("got %q", got)
	}
}

func TestPanelPid(t *testing.T) {
	dir := t.TempDir()
	exec.Command("git", "-C", dir, "init", "-q").Run()
	st := store.Open(dir)
	if panelPid(st) != 0 {
		t.Fatal("a panel was open before any started")
	}
	done := markPanel(st)
	if panelPid(st) != os.Getpid() {
		t.Fatalf("panelPid = %d, want %d", panelPid(st), os.Getpid())
	}
	done()
	if panelPid(st) != 0 {
		t.Fatal("the panel was still open after it closed")
	}
	os.WriteFile(panelFile(st), []byte("999999999\n"), 0o644)
	if panelPid(st) != 0 {
		t.Fatal("a panel whose process is gone counted as open")
	}
}

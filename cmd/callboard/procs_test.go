package main

import (
	"os"
	"strings"
	"testing"
)

func TestParentOf(t *testing.T) {
	ppid, comm, ok := parentOf(os.Getpid())
	if !ok || ppid != os.Getppid() || !strings.Contains(comm, "callboard") && !strings.Contains(comm, ".test") {
		t.Fatalf("parentOf(self) = %d %q %v, want %d", ppid, comm, ok, os.Getppid())
	}
	if _, _, ok := parentOf(1 << 30); ok {
		t.Fatal("a process that doesn't exist was found")
	}
}

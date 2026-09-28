package board

import (
	"strings"
	"testing"
)

const sectioned = `# Backlog

## Now

- [ ] One <!-- id:B-aaa1 -->
  a note

## Next

- [ ] Two <!-- id:B-aaa2 -->

## Later

- [ ] Three <!-- id:B-aaa3 -->
`

func TestRenameSection(t *testing.T) {
	f := Parse(sectioned)
	old, err := f.RenameSection("now", "Doing")
	if err != nil || old != "Now" {
		t.Fatal(old, err)
	}
	if f.Find("B-aaa1").Section != "Doing" || !strings.Contains(f.String(), "\n## Doing\n") {
		t.Fatal(f.String())
	}
	if _, err := f.RenameSection("Doing", "next"); err == nil {
		t.Fatal("renamed onto an existing section")
	}
	if _, err := f.RenameSection("Nope", "X"); err == nil || !strings.Contains(err.Error(), "Doing, Next, Later") {
		t.Fatal(err)
	}
}

func TestMoveSection(t *testing.T) {
	f := Parse(sectioned)
	if err := f.MoveSection("Later", "Now", false); err != nil {
		t.Fatal(err)
	}
	want := "# Backlog\n\n## Later\n\n- [ ] Three <!-- id:B-aaa3 -->\n\n## Now\n\n- [ ] One <!-- id:B-aaa1 -->\n  a note\n\n## Next\n\n- [ ] Two <!-- id:B-aaa2 -->\n"
	if f.String() != want {
		t.Fatalf("got\n%s", f.String())
	}
	if err := f.MoveSection("Later", "Next", true); err != nil {
		t.Fatal(err)
	}
	if f.String() != sectioned {
		t.Fatalf("got\n%s", f.String())
	}
	if err := f.MoveSection("Backlog", "Now", true); err == nil {
		t.Fatal("moved a section into itself")
	}
}

func TestRemoveSection(t *testing.T) {
	f := Parse(sectioned)
	if _, err := f.RemoveSection("Next", ""); err == nil {
		t.Fatal("removed a section that holds items")
	}
	moved, err := f.RemoveSection("Next", "Later")
	if err != nil || len(moved) != 1 || moved[0].Section != "Later" {
		t.Fatal(moved, err)
	}
	want := "# Backlog\n\n## Now\n\n- [ ] One <!-- id:B-aaa1 -->\n  a note\n\n## Later\n\n- [ ] Three <!-- id:B-aaa3 -->\n- [ ] Two <!-- id:B-aaa2 -->\n"
	if f.String() != want {
		t.Fatalf("got\n%s", f.String())
	}
	f = Parse("# B\n\n## Now\n\n- [ ] One <!-- id:B-aaa1 -->\n\n## Empty\n\n## Last\n")
	if _, err := f.RemoveSection("Empty", ""); err != nil {
		t.Fatal(err)
	}
	if f.String() != "# B\n\n## Now\n\n- [ ] One <!-- id:B-aaa1 -->\n\n## Last\n" {
		t.Fatalf("got\n%q", f.String())
	}
	if _, err := f.RemoveSection("Last", ""); err != nil || f.String() != "# B\n\n## Now\n\n- [ ] One <!-- id:B-aaa1 -->\n" {
		t.Fatalf("%v %q", err, f.String())
	}
	f = Parse("# B\n\n## Now\n\nSome words.\n")
	if _, err := f.RemoveSection("Now", ""); err == nil {
		t.Fatal("removed a section with text")
	}
}

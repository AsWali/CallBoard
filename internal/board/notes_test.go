package board

import (
	"strings"
	"testing"
)

func TestSetNotes(t *testing.T) {
	f := Parse(sample)
	if _, err := f.SetNotes("B-k3f9", "", "Plan: one parser for all three", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SetNotes("B-k3f9", "", "Then the writer\n  - keep blank lines out", true, false); err != nil {
		t.Fatal(err)
	}
	want := "- [ ] Parse the three files <!-- id:B-k3f9 by:claude at:2026-09-27T09:32 -->\n  Plan: one parser for all three\n  Then the writer\n    - keep blank lines out\n- [x] Random keys"
	if !strings.Contains(f.String(), want) {
		t.Fatalf("add:\n%s", f.String())
	}
	tk, err := f.SetNotes("B-k3f9", "", "Only this now", false, false)
	if err != nil || len(tk.Body) != 1 || strings.TrimSpace(tk.Body[0]) != "Only this now" {
		t.Fatalf("replace: %v %q", err, tk.Body)
	}
	tk, _ = f.SetNotes("B-k3f9", "", "", false, false)
	if len(tk.Body) != 0 || !strings.Contains(f.String(), "-->\n- [x] Random keys") {
		t.Fatalf("clear:\n%s", f.String())
	}
	if _, err := f.SetNotes("B-k3f9", "", "  ", true, false); err == nil {
		t.Fatal("an empty note was added")
	}
	if _, err := f.SetNotes("B-k3f9@zz", "", "x", true, false); err == nil {
		t.Fatal("a stale version was accepted")
	}
}

func TestSetNotesKeepsOptions(t *testing.T) {
	f := Parse("- [ ] Which port? <!-- id:Q-7x1c -->\n  Old note\n  - 4700 (recommended)\n  - 8080\n")
	tk, err := f.SetNotes("Q-7x1c", "", "Bookmarks matter", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tk.Options()) != 2 || !tk.Options()[0].Recommended || strings.Contains(f.String(), "Old note") || !strings.Contains(f.String(), "  Bookmarks matter") {
		t.Fatalf("question:\n%s", f.String())
	}
	if _, err := f.SetNotes("Q-7x1c", "", "- a third option", true, true); err == nil {
		t.Fatal("a note that reads as an option was added to a question")
	}
}

func TestPlace(t *testing.T) {
	f := Parse("## Now\n- [ ] A <!-- id:B-aaaa -->\n  a note\n- [ ] B <!-- id:B-bbbb -->\n## Later\n- [ ] C <!-- id:B-cccc -->\n")
	if _, err := f.Place("B-aaaa", "", "B-bbbb", true); err != nil {
		t.Fatal(err)
	}
	if got := f.String(); got != "## Now\n- [ ] B <!-- id:B-bbbb -->\n- [ ] A <!-- id:B-aaaa -->\n  a note\n## Later\n- [ ] C <!-- id:B-cccc -->\n" {
		t.Fatalf("after:\n%s", got)
	}
	tk, err := f.Place("B-aaaa", "", "B-cccc", false)
	if err != nil || tk.Section != "Later" || tk.Body[0] != "  a note" {
		t.Fatalf("into Later: %v %+v\n%s", err, tk, f.String())
	}
	if !strings.Contains(f.String(), "## Later\n- [ ] A <!-- id:B-aaaa -->\n  a note\n- [ ] C") {
		t.Fatalf("before C:\n%s", f.String())
	}
	if _, err := f.Place("B-aaaa", "", "B-zzzz", false); err == nil {
		t.Fatal("placed next to an item that isn't there")
	}
}

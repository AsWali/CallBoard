package board

import (
	"strings"
	"testing"
)

const nested = `## Now

- [ ] Ship it <!-- id:B-par1 -->
  why we ship
  - [ ] Write docs <!-- id:B-kid1 -->
    a note on docs
  - [x] Tag release <!-- id:B-kid2 -->
  ` + "```" + `
  - [ ] not a subtask
  ` + "```" + `
- [ ] Next thing <!-- id:B-nxt1 -->
`

func TestParseSubtasks(t *testing.T) {
	f := Parse(nested)
	var got []string
	for _, x := range f.Tasks {
		got = append(got, x.Key+">"+x.Parent)
	}
	if strings.Join(got, " ") != "B-par1> B-kid1>B-par1 B-kid2>B-par1 B-nxt1>" {
		t.Fatal(got)
	}
	p := f.Find("B-par1")
	if p.End != 10 || len(p.Body) != 4 || p.Body[0] != "  why we ship" || strings.Join(p.Block(), "|") != p.Raw+"|  why we ship" {
		t.Fatalf("%d %q %q", p.End, p.Body, p.Block())
	}
	if k := f.Find("B-kid1"); len(k.Body) != 1 || k.Depth != 1 {
		t.Fatal(k.Body, k.Depth)
	}
	q := ParseAs(strings.ReplaceAll(nested, "B-", "Q-"), "Q-")
	if len(q.Tasks) != 2 {
		t.Fatalf("a question's checkbox options became items: %d", len(q.Tasks))
	}
}

func TestSubtaskWriters(t *testing.T) {
	f := Parse(nested)
	if _, err := f.SetNotes("B-par1", "", "new why", false, false); err != nil {
		t.Fatal(err)
	}
	if f.Find("B-kid1") == nil || f.Find("B-par1").Body[0] != "  new why" || strings.Contains(f.String(), "not a subtask") {
		t.Fatalf("notes clobbered the subtasks:\n%s", f.String())
	}
	f = Parse(nested)
	f.AppendBody("B-par1", "second why")
	if f.Lines[4] != "  second why" || f.Find("B-kid1").Parent != "B-par1" {
		t.Fatalf("note not before the subtasks:\n%s", f.String())
	}
	s, err := f.AddNew(New{Title: "Record a demo", Parent: "B-par1"})
	if err != nil || s.Parent != "B-par1" || s.Depth != 1 {
		t.Fatal(s, err)
	}
	f.Tasks = nil
	f.index()
	if f.Find("B-nxt1").Parent != "" {
		t.Fatal("the next item became a subtask")
	}
	if _, err := f.Move("B-kid1", "", "Later"); err != nil {
		t.Fatal(err)
	}
	if k := f.Find("B-kid1"); k.Parent != "" || k.Section != "Later" || !strings.Contains(f.String(), "## Later\n\n- [ ] Write docs") || !strings.Contains(f.String(), "\n  a note on docs") {
		t.Fatalf("moved subtask:\n%s", f.String())
	}
	if _, err := ParseAs("- [ ] Q <!-- id:Q-aaa1 -->\n", "Q-").AddNew(New{Title: "x", Parent: "Q-aaa1"}); err == nil {
		t.Fatal("a question got a subtask")
	}
}

func TestCutSubtaskPutsBack(t *testing.T) {
	f := Parse(nested)
	before := f.String()
	_, r, err := f.Cut("B-kid2", "")
	if err != nil || r.Parent != "B-par1" || r.After != "B-kid1" {
		t.Fatal(r, err)
	}
	if _, err := f.PutBack("B-kid2", r); err != nil || f.String() != before {
		t.Fatalf("%v\n%s", err, f.String())
	}
	_, r, _ = f.Cut("B-par1", "")
	if f.Find("B-kid1") != nil {
		t.Fatal("deleting a parent left its subtasks")
	}
	f.PutBack("B-par1", r)
	if f.String() != before {
		t.Fatalf("%s", f.String())
	}
}

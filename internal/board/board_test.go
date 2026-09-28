package board

import (
	"strings"
	"testing"
)

func init() { Now = func() string { return "2026-09-27T10:14" } }

const sample = `# Backlog

Some words that are not tasks.

## v1
- [ ] Parse the three files <!-- id:B-k3f9 by:claude at:2026-09-27T09:32 -->
- [x] Random keys <!-- id:B-p0q1 by:claude at:2026-09-27T08:44 done:2026-09-27T09:00 -->
- [ ] Typed by hand

` + "```" + `
- [ ] not a task, it's in a code block
` + "```" + `

## Later
- [ ] Field-level merge <!-- id:B-w81c by:you at:2026-09-27T05:14 prio:low -->
`

func TestParseRoundTrip(t *testing.T) {
	f := Parse(sample)
	if got := f.String(); got != sample {
		t.Fatalf("round trip changed the file:\n%s", got)
	}
	if len(f.Tasks) != 4 {
		t.Fatalf("want 4 tasks, got %d", len(f.Tasks))
	}
	k := f.Find("B-k3f9")
	if k.Title != "Parse the three files" || k.Section != "v1" || k.By != "claude" || k.Line != 6 || k.Done {
		t.Fatalf("bad task %+v", k)
	}
	if p := f.Find("b-P0Q1"); !p.Done || p.DoneAt != "2026-09-27T09:00" {
		t.Fatalf("bad done task %+v", p)
	}
	if w := f.Find("B-w81c"); w.Section != "Later" || w.Format() != w.Raw {
		t.Fatalf("unknown fields not kept: %q", w.Format())
	}
	if strings.Join(f.Sections, ",") != "Backlog,v1,Later" {
		t.Fatalf("sections %v", f.Sections)
	}
}

func TestNoTrailingNewlineKept(t *testing.T) {
	for _, s := range []string{"", "\n", "- [ ] a", "- [ ] a\n", "x\n\n"} {
		if got := Parse(s).String(); got != s {
			t.Errorf("%q came back as %q", s, got)
		}
	}
}

func TestAddToSection(t *testing.T) {
	f := Parse(sample)
	task, err := f.Add("Merge driver", "v1", "claude")
	if err != nil {
		t.Fatal(err)
	}
	if task.Section != "v1" || task.Line != 9 || !strings.HasPrefix(task.Key, "B-") || len(task.Key) != 6 {
		t.Fatalf("bad new task %+v", task)
	}
	want := "- [ ] Merge driver <!-- id:" + task.Key + " by:claude at:2026-09-27T10:14 -->"
	if f.Lines[8] != want {
		t.Fatalf("line is %q", f.Lines[8])
	}

	if h := f.Tasks[2]; h.Title != "Typed by hand" || h.Key == "" {
		t.Fatalf("hand-typed task not keyed: %+v", h)
	}

	before, after := strings.Split(sample, "\n"), f.Lines
	if len(after) != len(before) {
		t.Fatalf("line count %d vs %d", len(after), len(before))
	}
}

func TestAddDefaultSection(t *testing.T) {
	f := Parse(sample)
	if task, _ := f.Add("x", "", "you"); task.Section != "v1" {
		t.Fatalf("went to %q", task.Section)
	}
	f = Parse("# Backlog\n\n## Now\n")
	if task, _ := f.Add("x", "", "you"); task.Section != "Now" {
		t.Fatalf("went to %q", task.Section)
	}
	f = Parse("## Now\n\n## Later\n")
	task, _ := f.Add("first", "", "you")
	if task.Section != "Now" {
		t.Fatalf("went to %q", task.Section)
	}
	if got := f.String(); got != "## Now\n\n- [ ] first <!-- id:"+task.Key+" by:you at:2026-09-27T10:14 -->\n\n## Later\n" {
		t.Fatalf("got\n%s", got)
	}
}

func TestAddNewSectionAndEmptyFile(t *testing.T) {
	f := Parse("")
	a, _ := f.Add("one", "", "you")
	if f.String() != a.Raw+"\n" {
		t.Fatalf("empty file: %q", f.String())
	}
	b, _ := f.Add("two", "Later", "you")
	want := a.Raw + "\n\n## Later\n\n" + b.Raw + "\n"
	if f.String() != want {
		t.Fatalf("got %q want %q", f.String(), want)
	}
	if b.Section != "Later" {
		t.Fatalf("section %q", b.Section)
	}
}

func TestAddRejects(t *testing.T) {
	f := Parse("")
	if _, err := f.Add("  ", "", "you"); err == nil {
		t.Fatal("empty title accepted")
	}
	if _, err := f.Add("a --> b", "", "you"); err == nil {
		t.Fatal("--> accepted")
	}
}

func TestTick(t *testing.T) {
	f := Parse(sample)
	task, err := f.Tick("B-k3f9", true)
	if err != nil {
		t.Fatal(err)
	}
	if f.Lines[5] != "- [x] Parse the three files <!-- id:B-k3f9 by:claude at:2026-09-27T09:32 done:2026-09-27T10:14 -->" || !task.Done {
		t.Fatalf("line is %q", f.Lines[5])
	}
	f.Tick("B-k3f9", false)
	if f.Lines[5] != "- [ ] Parse the three files <!-- id:B-k3f9 by:claude at:2026-09-27T09:32 -->" {
		t.Fatalf("reopen: %q", f.Lines[5])
	}
	_, err = f.Tick("B-zzzz", true)
	if err == nil || !strings.Contains(err.Error(), "B-k3f9 Parse the three files") {
		t.Fatalf("unknown key error: %v", err)
	}
}

func TestKeysAreUnique(t *testing.T) {
	f := Parse("")
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		task, _ := f.Add("t", "", "you")
		if seen[task.Key] {
			t.Fatalf("duplicate key %s", task.Key)
		}
		seen[task.Key] = true
	}
}

func TestAddKeepsBlankBeforeNextHeading(t *testing.T) {
	f := Parse("## v1\n\n## Later\n")
	a, _ := f.Add("x", "v1", "you")
	if got := f.String(); got != "## v1\n\n"+a.Raw+"\n\n## Later\n" {
		t.Fatalf("got %q", got)
	}
	b, _ := f.Add("y", "v1", "you")
	if got := f.String(); got != "## v1\n\n"+a.Raw+"\n"+b.Raw+"\n\n## Later\n" {
		t.Fatalf("got %q", got)
	}
}

const questions = `## Open

- [ ] Which port for the page? <!-- id:Q-7x1c by:claude at:2026-09-27T09:00 needs:R-2m8a assumed:1 -->
  - From the worktree's path *(recommended)*: bookmarks keep working
  - Always 4700
- [ ] Rename on the page? <!-- id:Q-aaaa -->

Text after.
`

func TestBodyOptionsNeedsVersion(t *testing.T) {
	f := Parse(questions)
	if f.String() != questions {
		t.Fatal("round trip changed the file")
	}
	q := f.Find("Q-7x1c")
	if len(q.Body) != 2 || q.End != 5 || q.Assumed != 1 || strings.Join(q.Needs, ",") != "R-2m8a" {
		t.Fatalf("bad question %+v", q)
	}
	opts := q.Options()
	if len(opts) != 2 || !opts[0].Recommended || opts[0].Text != "From the worktree's path: bookmarks keep working" || opts[1].Recommended {
		t.Fatalf("options %+v", opts)
	}
	if len(q.Version) == 0 || q.Version == f.Find("Q-aaaa").Version {
		t.Fatalf("versions %q", q.Version)
	}
	if q.Format() != q.Raw {
		t.Fatalf("format %q", q.Format())
	}
}

func TestVersionCheck(t *testing.T) {
	f := Parse(questions)
	v := f.Find("Q-aaaa").Version
	if _, err := f.Rename("Q-aaaa@"+v, "", "Rename tasks on the page?"); err != nil {
		t.Fatal(err)
	}
	_, err := f.TickV("Q-aaaa", v, true)
	if _, ok := err.(ErrChanged); !ok {
		t.Fatalf("stale edit allowed: %v", err)
	}
	nv := f.Find("Q-aaaa").Version
	if _, err := f.TickV("Q-aaaa@"+nv, "", true); err != nil {
		t.Fatal(err)
	}
}

func TestAddNewWithBodyAndRemove(t *testing.T) {
	f := Parse(questions)
	f.Prefix = "Q-"
	n, err := f.AddNew(New{Title: "Keep a Decisions view?", Section: "Open", By: "claude", Body: []string{"- Yes (recommended)", "- No"}, Needs: []string{"B-k3f9"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.Key, "Q-") || len(n.Options()) != 2 || n.Line != 7 || n.End != 9 {
		t.Fatalf("new %+v\n%s", n, f.String())
	}
	if f.Lines[9] != "" || f.Lines[10] != "Text after." {
		t.Fatalf("text after moved:\n%s", f.String())
	}
	if _, err := f.Remove("Q-7x1c", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.String(), "Always 4700") || f.Find("Q-7x1c") != nil {
		t.Fatalf("not removed:\n%s", f.String())
	}
	a, _ := f.AppendBody(n.Key, "Answered: yes")
	if a.Body[len(a.Body)-1] != "  Answered: yes" {
		t.Fatalf("body %q", a.Body)
	}
}

func TestConflicts(t *testing.T) {
	f := Parse("## Now\n\n- [x] A <!-- id:B-aaaa -->\n<<<<<<< ours\n- [ ] E on main <!-- id:B-eeee -->\n=======\n- [ ] E on x <!-- id:B-eeee -->\n>>>>>>> theirs\n- [ ] D <!-- id:B-dddd -->\n")
	cs := f.Conflicts()
	if len(cs) != 1 || strings.Join(cs[0].Keys(), ",") != "B-eeee" || cs[0].Side(2)[0].Title != "E on x" {
		t.Fatalf("conflicts %+v", cs)
	}
	if o, d := f.Counts(); o != 2 || d != 1 {
		t.Fatalf("counts %d open %d done", o, d)
	}
	if _, err := f.Tick("B-eeee", true); err == nil {
		t.Fatal("edit inside a conflict allowed")
	}
	if _, err := f.Tick("B-dddd", true); err != nil {
		t.Fatalf("edit outside a conflict refused: %v", err)
	}
	kept, err := f.Resolve("B-eeee", 2)
	if err != nil || kept[0].Title != "E on x" || len(f.Conflicts()) != 0 || f.Find("B-eeee").Title != "E on x" || len(f.Tasks) != 3 {
		t.Fatalf("resolve: %v\n%s", err, f.String())
	}
}

func TestQuotedMeta(t *testing.T) {
	task := ParseLine(`- [x] Which port? <!-- id:Q-aaaa answer:"From the path, \"always\"" why:bookmarks prio:high -->`)
	if task.Meta("answer") != `From the path, "always"` || task.Meta("why") != "bookmarks" || task.Meta("prio") != "high" {
		t.Fatalf("meta %v", task.Fields())
	}
	if task.Format() != task.Raw {
		t.Fatalf("round trip %q", task.Format())
	}
	task.SetMeta("why", `keeps a \ and spaces`)
	if again := ParseLine(task.Format()); again.Meta("why") != `keeps a \ and spaces` {
		t.Fatalf("got %q from %q", again.Meta("why"), task.Format())
	}
}

func TestMoveAndSetFields(t *testing.T) {
	f := Parse("## Now\n\n- [ ] A <!-- id:B-aaaa -->\n  a note\n- [ ] B <!-- id:B-bbbb -->\n\n## Later\n\n- [ ] C <!-- id:B-cccc -->\n")
	m, err := f.Move("B-aaaa", "", "Later")
	if err != nil || m.Section != "Later" || len(m.Body) != 1 {
		t.Fatalf("move: %v %+v", err, m)
	}
	if got := f.String(); got != "## Now\n\n- [ ] B <!-- id:B-bbbb -->\n\n## Later\n\n- [ ] C <!-- id:B-cccc -->\n- [ ] A <!-- id:B-aaaa -->\n  a note\n" {
		t.Fatalf("after move:\n%s", got)
	}
	if _, err := f.Move("B-bbbb", "", "Doing"); err != nil || f.Find("B-bbbb").Section != "Doing" {
		t.Fatalf("move to a new section: %v\n%s", err, f.String())
	}
	x, err := f.SetFields("B-cccc", "", [][2]string{{"status", "in review"}, {"prio", "high"}})
	if err != nil || x.Meta("status") != "in review" || !strings.Contains(x.Raw, `status:"in review" prio:high`) {
		t.Fatalf("set: %v %q", err, x.Raw)
	}
	if _, err := f.SetFields("B-cccc", "", [][2]string{{"prio", ""}}); err != nil || f.Find("B-cccc").Meta("prio") != "" {
		t.Fatal("clearing a field")
	}
	for _, bad := range []string{"done", "Status", "a b", "answer"} {
		if _, err := f.SetFields("B-cccc", "", [][2]string{{bad, "x"}}); err == nil {
			t.Errorf("field %q accepted", bad)
		}
	}
}

func TestHandTypedOddities(t *testing.T) {
	cases := []struct{ in, key, title, out string }{
		{"- [ ] Item 1 <!-- id:B-one --> - [ ] Item 2 <!-- id:B-two -->", "B-two", "Item 1 <!-- id:B-one --> - [ ] Item 2", "- [ ] Item 1 <!-- id:B-one --> - [ ] Item 2 <!-- id:B-two -->"},
		{`- [ ] Item <!-- id:B-test f:"value-->" -->`, "B-test", "Item", `- [ ] Item <!-- id:B-test f:value-> -->`},
		{"- [ ] Item <!-- id:B-test f:v", "B-test", "Item", "- [ ] Item <!-- id:B-test f:v -->"},
		{"- [ ] Item <!-- a note --> and more", "", "Item <!-- a note --> and more", "- [ ] Item <!-- a note --> and more"},
	}
	for _, c := range cases {
		tk := ParseLine(c.in)
		if tk.Key != c.key || tk.Title != c.title {
			t.Errorf("%q: key %q title %q", c.in, tk.Key, tk.Title)
		}
		if got := tk.Format(); got != c.out {
			t.Errorf("%q: written back as %q, want %q", c.in, got, c.out)
		}
	}
	f := Parse("## Tasks\n- [ ] Item <!-- id:B-test at:2026-09-27T10:00\n  continued -->\n")
	if n := f.FillKeys(); n != 0 {
		t.Errorf("gave %d item(s) with an unclosed comment a second key:\n%s", n, f)
	}
	f = Parse("## Tasks\r\n- [ ] A <!-- id:B-aaaa -->\r\n- [ ] B <!-- id:B-bbbb -->\r\n")
	if _, err := f.Rename("B-aaaa", "", "Changed"); err != nil {
		t.Fatal(err)
	}
	if want := "## Tasks\r\n- [ ] Changed <!-- id:B-aaaa -->\r\n- [ ] B <!-- id:B-bbbb -->\r\n"; f.String() != want {
		t.Errorf("CRLF file written back as %q", f.String())
	}
}

func TestControlCharactersAreDropped(t *testing.T) {
	f := Parse("## Now\x1b[2J\n\n- [ ] Pay\x1b]0;owned\x07 the\rbill <!-- id:B-aaaa prio:\"hi\x1b[31mgh\" -->\n  a note\x1b[8m hidden\n")
	tk := f.Find("B-aaaa")
	if tk == nil || tk.Title != "Pay]0;owned thebill" || tk.Meta("prio") != "hi[31mgh" || tk.Body[0] != "  a note[8m hidden" || f.Sections[0] != "Now[2J" {
		t.Fatalf("%+v %q", tk, f.Sections)
	}
	if _, err := f.AddNew(New{Title: "New\x1b[1A one", Body: []string{"  body\x9b2J"}}); err != nil {
		t.Fatal(err)
	}
	if out := f.String(); strings.ContainsAny(out, "\x1b\r\x07\u009b") {
		t.Fatalf("control characters written back: %q", out)
	}
}

func TestLineEndingsFollowMostLines(t *testing.T) {
	mostlyLF := "## Now\n\n- [ ] One <!-- id:B-aaaa -->\r\n- [ ] Two <!-- id:B-bbbb -->\n- [ ] Three <!-- id:B-cccc -->\n"
	f := Parse(mostlyLF)
	f.Tick("B-bbbb", true)
	if out := f.String(); strings.Contains(out, "\r") || strings.Count(out, "\n") != 5 {
		t.Fatalf("mostly LF became %q", out)
	}
	mostlyCRLF := "## Now\r\n\r\n- [ ] One <!-- id:B-aaaa -->\n- [ ] Two <!-- id:B-bbbb -->\r\n"
	f = Parse(mostlyCRLF)
	f.Tick("B-bbbb", true)
	if out := f.String(); strings.Count(out, "\r\n") != 4 {
		t.Fatalf("mostly CRLF became %q", out)
	}
}

func TestNoteThatLooksLikeATask(t *testing.T) {
	f := Parse("## Now\n\n- [ ] Parent <!-- id:B-aaaa -->\n")
	f.AppendBody("B-aaaa", "- [ ] call the bank")
	f.SetNotes("B-aaaa", "", "- [x] already done\n* [ ] and this", true, false)
	f.AddNew(New{Title: "Other", Body: []string{"- [ ] step one"}})
	if len(f.Tasks) != 2 || f.Find("B-aaaa").kids != 0 {
		t.Fatalf("a note became a subtask:\n%s", f.String())
	}
	var got []string
	for _, b := range f.Find("B-aaaa").Body {
		got = append(got, UnescapeNote(strings.TrimSpace(b)))
	}
	if strings.Join(got, "|") != "- [ ] call the bank|- [x] already done|* [ ] and this" {
		t.Fatalf("notes read back as %q\n%s", got, f.String())
	}
}

func TestUnclosedFenceHidesNothing(t *testing.T) {
	f := Parse("# Backlog\n\n## Now\n\n- [ ] One <!-- id:B-aaaa -->\n\n```sh\nmake all\n\n## Later\n\n- [ ] Two <!-- id:B-bbbb -->\n")
	if len(f.Tasks) != 2 {
		t.Fatalf("an unclosed fence hid items: %d", len(f.Tasks))
	}
	n, err := f.AddNew(New{Title: "Three", Section: "Later"})
	if err != nil || f.Find(n.Key) == nil || len(f.Tasks) != 3 || f.Find(n.Key).Section != "Later" {
		t.Fatalf("new item hidden: %v\n%s", err, f.String())
	}
	g := Parse("## Now\n\n```\na\n```\n\n- [ ] One <!-- id:B-aaaa -->\n\n```\nb\n```\n")
	if len(g.Tasks) != 1 {
		t.Fatalf("closed fences broke: %d", len(g.Tasks))
	}
	h := Parse("## Now\n\n~~~\n- [ ] In a fence <!-- id:B-cccc -->\n~~~\n~~~\n- [ ] Two <!-- id:B-dddd -->\n")
	if h.Find("B-cccc") != nil || h.Find("B-dddd") == nil {
		t.Fatalf("fence pairing wrong: %v", h.Tasks)
	}
}

func TestOptionLineMarksRecommendedOnce(t *testing.T) {
	for _, c := range []struct {
		in   string
		rec  bool
		want string
	}{
		{"Apple only", false, "- Apple only"},
		{"Apple only", true, "- Apple only (recommended)"},
		{"Apple only (recommended)", true, "- Apple only (recommended)"},
		{"- Apple only (Recommended)", false, "- Apple only (recommended)"},
		{"  Apple plus Google  ", true, "- Apple plus Google (recommended)"},
	} {
		if got := OptionLine(c.in, c.rec); got != c.want {
			t.Errorf("OptionLine(%q, %v) = %q, want %q", c.in, c.rec, got, c.want)
		}
	}
}

package merge

import (
	"strings"
	"testing"
)

const base = `## v1
- [ ] Parse <!-- id:B-aaaa -->
- [ ] Keys <!-- id:B-bbbb -->
- [ ] Merge <!-- id:B-cccc -->

## Later
- [ ] Field merge <!-- id:B-dddd -->
`

func TestCleanMerge(t *testing.T) {
	ours := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [x] Keys <!-- id:B-bbbb done:x -->", 1)
	ours = strings.Replace(ours, "- [ ] Merge <!-- id:B-cccc -->\n", "- [ ] Merge <!-- id:B-cccc -->\n- [ ] Ours new <!-- id:B-o001 -->\n", 1)
	theirs := strings.Replace(base, "- [ ] Parse <!-- id:B-aaaa -->", "- [x] Parse <!-- id:B-aaaa done:y -->", 1)
	theirs = strings.Replace(theirs, "- [ ] Field merge <!-- id:B-dddd -->\n", "- [ ] Field merge <!-- id:B-dddd -->\n- [ ] Theirs new <!-- id:B-t001 -->\n", 1)
	theirs = strings.Replace(theirs, "- [ ] Merge <!-- id:B-cccc -->\n", "", 1)
	r := Merge(base, ours, theirs)
	want := `## v1
- [x] Parse <!-- id:B-aaaa done:y -->
- [x] Keys <!-- id:B-bbbb done:x -->
- [ ] Ours new <!-- id:B-o001 -->

## Later
- [ ] Field merge <!-- id:B-dddd -->
- [ ] Theirs new <!-- id:B-t001 -->
`
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestBothAddAtSamePlace(t *testing.T) {
	ours := base + "- [ ] A <!-- id:B-a001 -->\n"
	theirs := base + "- [ ] B <!-- id:B-b001 -->\n"
	r := Merge(base, ours, theirs)
	if r.Conflicts != 0 || !strings.HasSuffix(r.Text, "- [ ] A <!-- id:B-a001 -->\n- [ ] B <!-- id:B-b001 -->\n") {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestNewSectionFromTheirs(t *testing.T) {
	theirs := base + "\n## Bugs\n\n- [ ] Crash <!-- id:B-bug1 -->\n"
	ours := strings.Replace(base, "- [ ] Parse", "- [x] Parse", 1)
	r := Merge(base, ours, theirs)
	if r.Conflicts != 0 || !strings.HasSuffix(r.Text, "- [ ] Field merge <!-- id:B-dddd -->\n\n## Bugs\n\n- [ ] Crash <!-- id:B-bug1 -->\n") || !strings.Contains(r.Text, "- [x] Parse") {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestSameChangeBothSides(t *testing.T) {
	both := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [x] Keys <!-- id:B-bbbb done:z -->", 1)
	r := Merge(base, both, both)
	if r.Conflicts != 0 || r.Text != both {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestConflictOnOneTask(t *testing.T) {
	ours := strings.Replace(base, "Keys <!--", "Keys, random <!--", 1)
	ours = strings.Replace(ours, "- [ ] Parse", "- [x] Parse", 1)
	theirs := strings.Replace(base, "Keys <!--", "Keys, short <!--", 1)
	r := Merge(base, ours, theirs, "HEAD", "feat-b")
	want := `## v1
- [x] Parse <!-- id:B-aaaa -->
<<<<<<< HEAD
- [ ] Keys, random <!-- id:B-bbbb -->
=======
- [ ] Keys, short <!-- id:B-bbbb -->
>>>>>>> feat-b
- [ ] Merge <!-- id:B-cccc -->
`
	if r.Conflicts != 1 || !strings.HasPrefix(r.Text, want) {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestChangedVersusDeleted(t *testing.T) {
	ours := strings.Replace(base, "- [ ] Merge", "- [x] Merge", 1)
	theirs := strings.Replace(base, "- [ ] Merge <!-- id:B-cccc -->\n", "", 1)
	r := Merge(base, ours, theirs)
	if r.Conflicts != 1 || !strings.Contains(r.Text, "<<<<<<< ours\n- [x] Merge <!-- id:B-cccc -->\n=======\n>>>>>>> theirs\n") {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestMoveFromTheirsLands(t *testing.T) {
	theirs := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->\n", "", 1)
	theirs = strings.Replace(theirs, "- [ ] Field merge <!-- id:B-dddd -->\n", "- [ ] Field merge <!-- id:B-dddd -->\n- [ ] Keys <!-- id:B-bbbb -->\n", 1)
	ours := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [ ] Random keys <!-- id:B-bbbb -->\n- [ ] Ours new <!-- id:B-o001 -->", 1)
	r := Merge(base, ours, theirs)
	want := `## v1
- [ ] Parse <!-- id:B-aaaa -->
- [ ] Ours new <!-- id:B-o001 -->
- [ ] Merge <!-- id:B-cccc -->

## Later
- [ ] Field merge <!-- id:B-dddd -->
- [ ] Random keys <!-- id:B-bbbb -->
`
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}

	if r := Merge(base, theirs, base); r.Text != theirs {
		t.Fatalf("ours moved:\n%s", r.Text)
	}
}

func TestSameFieldTwoWaysConflicts(t *testing.T) {
	set := func(meta string) string {
		return strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [ ] Keys <!-- id:B-bbbb "+meta+" -->", 1)
	}

	if r := Merge(base, set("prio:low"), set("prio:high")); r.Conflicts != 1 || !strings.Contains(r.Text, "<<<<<<< ours\n- [ ] Keys <!-- id:B-bbbb prio:low -->") {
		t.Fatalf("prio two ways: conflicts=%d\n%s", r.Conflicts, r.Text)
	}

	if r := Merge(base, set("area:ui"), set("prio:high")); r.Conflicts != 0 || !strings.Contains(r.Text, "id:B-bbbb area:ui prio:high") {
		t.Fatalf("two fields: conflicts=%d\n%s", r.Conflicts, r.Text)
	}

	withPrio := set("prio:low")
	if r := Merge(withPrio, base, set("prio:high")); r.Conflicts != 1 {
		t.Fatalf("removed vs changed: conflicts=%d\n%s", r.Conflicts, r.Text)
	}

	q := "- [ ] Colour? <!-- id:Q-aaaa -->\n"
	ans := func(a string) string { return "- [x] Colour? <!-- id:Q-aaaa done:1 answer:" + a + " -->\n" }
	if r := Merge(q, ans("red"), ans("blue")); r.Conflicts != 1 {
		t.Fatalf("two answers: conflicts=%d\n%s", r.Conflicts, r.Text)
	}
	if r := Merge(q, ans("red"), ans("red")); r.Conflicts != 0 {
		t.Fatalf("same answer: conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestStatusGoesWithTheTick(t *testing.T) {
	b := "- [ ] A <!-- id:B-aaaa status:todo -->\n"
	claimed := "- [ ] A <!-- id:B-aaaa status:doing -->\n"
	ticked := "- [x] A <!-- id:B-aaaa done:1 status:done -->\n"
	for _, r := range []Result{Merge(b, claimed, ticked), Merge(b, ticked, claimed)} {
		if r.Conflicts != 0 || r.Text != ticked {
			t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
		}
	}
}

func TestLabelsNameTheSides(t *testing.T) {
	ours := strings.Replace(base, "- [ ] Keys", "- [ ] Keys here", 1)
	theirs := strings.Replace(base, "- [ ] Keys", "- [ ] Keys there", 1)
	r := Merge(base, ours, theirs, "main", "feat")
	if !strings.Contains(r.Text, "<<<<<<< main\n") || !strings.Contains(r.Text, ">>>>>>> feat\n") {
		t.Fatalf("labels:\n%s", r.Text)
	}
}

func TestTickAndRenameBothLand(t *testing.T) {
	ours := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [x] Keys <!-- id:B-bbbb done:1 -->", 1)
	theirs := strings.Replace(base, "- [ ] Keys <!-- id:B-bbbb -->", "- [ ] Random keys <!-- id:B-bbbb needs:Q-aaaa -->", 1)
	r := Merge(base, ours, theirs)
	if r.Conflicts != 0 || !strings.Contains(r.Text, "- [x] Random keys <!-- id:B-bbbb done:1 needs:Q-aaaa -->\n") {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

func TestBodiesMerge(t *testing.T) {
	b := "- [ ] Q? <!-- id:Q-aaaa -->\n  - One\n  - Two\n- [ ] Next <!-- id:B-cccc -->\n"
	ours := "- [ ] Q? <!-- id:Q-aaaa -->\n  - One\n  - Two\n  Answer to Q-x: yes\n- [ ] Next <!-- id:B-cccc -->\n"
	theirs := "- [ ] Q, reworded? <!-- id:Q-aaaa -->\n  - One\n  - Two\n  Note from theirs\n- [ ] Next <!-- id:B-cccc -->\n"
	r := Merge(b, ours, theirs)
	want := "- [ ] Q, reworded? <!-- id:Q-aaaa -->\n  - One\n  - Two\n  Answer to Q-x: yes\n  Note from theirs\n- [ ] Next <!-- id:B-cccc -->\n"
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}

	theirs = "- [ ] Next <!-- id:B-cccc -->\n"
	if r := Merge(b, b, theirs); r.Text != theirs || r.Conflicts != 0 {
		t.Fatalf("delete with body:\n%s", r.Text)
	}
}

func TestConflictVersionsKeepCleanChanges(t *testing.T) {
	b := "- [ ] Sign in <!-- id:B-aaaa -->\n"
	o := "- [ ] Sign in <!-- id:B-aaaa prio:low status:doing -->\n"
	th := "- [x] Sign in <!-- id:B-aaaa done:1 prio:high area:auth -->\n"
	r := Merge(b, o, th, "main", "feat")
	want := "<<<<<<< main\n- [x] Sign in <!-- id:B-aaaa done:1 prio:low status:doing area:auth -->\n=======\n- [x] Sign in <!-- id:B-aaaa done:1 prio:high status:doing area:auth -->\n>>>>>>> feat\n"
	if r.Conflicts != 1 || r.Text != want {
		t.Fatalf("got:\n%s\nwant:\n%s", r.Text, want)
	}

	r = Merge(b, "- [ ] Sign-in page <!-- id:B-aaaa -->\n", "- [x] Log in <!-- id:B-aaaa done:1 -->\n")
	if r.Conflicts != 1 || strings.Count(r.Text, "- [x]") != 2 {
		t.Fatalf("titles:\n%s", r.Text)
	}
}

func TestMergeSubtasks(t *testing.T) {
	base := "# B\n\n- [ ] Ship <!-- id:B-par1 -->\n  why\n  - [ ] Docs <!-- id:B-kid1 -->\n  - [ ] Tag <!-- id:B-kid2 -->\n"
	ours := strings.Replace(base, "- [ ] Docs", "- [x] Docs", 1)
	theirs := strings.Replace(base, "- [ ] Tag", "- [x] Tag", 1) + "  - [ ] Demo <!-- id:B-kid3 -->\n"
	r := MergeAs("B-", base, ours, theirs)
	want := "# B\n\n- [ ] Ship <!-- id:B-par1 -->\n  why\n  - [x] Docs <!-- id:B-kid1 -->\n  - [x] Tag <!-- id:B-kid2 -->\n  - [ ] Demo <!-- id:B-kid3 -->\n"
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("%d conflicts:\n%s", r.Conflicts, r.Text)
	}
	q := "- [ ] Which? <!-- id:Q-aaa1 -->\n  - [ ] One\n  - [ ] Two\n"
	if r := MergeAs("Q-", q, strings.Replace(q, "One", "Uno", 1), strings.Replace(q, "Two", "Dos", 1)); r.Conflicts == 0 && !strings.Contains(r.Text, "Uno") {
		t.Fatalf("question options merged as items:\n%s", r.Text)
	}
}

func TestMovesNextToAMovedItemBothLand(t *testing.T) {
	theirs := strings.Replace(base, "- [ ] Parse <!-- id:B-aaaa -->\n", "", 1)
	theirs += "- [ ] Parse <!-- id:B-aaaa -->\n- [ ] Theirs new <!-- id:B-t001 -->\n"
	ours := strings.Replace(base, "\n## Later\n- [ ] Field merge <!-- id:B-dddd -->\n", "- [ ] Field merge <!-- id:B-dddd -->\n\n## Later\n", 1)
	r := Merge(base, ours, theirs)
	want := `## v1
- [ ] Keys <!-- id:B-bbbb -->
- [ ] Merge <!-- id:B-cccc -->
- [ ] Field merge <!-- id:B-dddd -->

## Later
- [ ] Parse <!-- id:B-aaaa -->
- [ ] Theirs new <!-- id:B-t001 -->
`
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("conflicts=%d\n%s", r.Conflicts, r.Text)
	}
	r = Merge(base, theirs, ours)
	if r.Conflicts != 0 || r.Text != want {
		t.Fatalf("other way round, conflicts=%d\n%s", r.Conflicts, r.Text)
	}
}

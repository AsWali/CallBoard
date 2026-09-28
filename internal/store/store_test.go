package store

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func repo(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
	return Open(dir)
}

var claude = Actor{By: "claude", Session: "s1"}
var you = Actor{By: "you"}

func TestNeedsAnswerFlow(t *testing.T) {
	s := repo(t)
	q, err := s.Add(Questions, board.New{Title: "Which port?", Body: []string{"- From the path (recommended)", "- Always 4700"}}, claude)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := s.Add(Requests, board.New{Title: "Make a GitHub repo", Body: []string{"1. Go to github.com/new", "2. Name it callboard"}}, claude)
	task, err := s.Add(Backlog, board.New{Title: "Serve on the right port", Needs: []string{q.Key, strings.ToLower(r.Key)}}, claude)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Backlog, board.New{Title: "x", Needs: []string{"B-nope"}}, claude); err == nil {
		t.Fatal("needs on a missing key accepted")
	}

	l, _ := s.List("task", "ready", "")
	if len(l.Items) != 0 {
		t.Fatalf("blocked task listed as ready: %+v", l.Items)
	}
	l, _ = s.List("", "open", "")
	if len(l.Items) != 3 || l.Items[0].Kind != "task" || !l.Items[0].Blocked || len(l.Items[0].WaitingOn) != 2 {
		t.Fatalf("list %+v", l.Items)
	}
	if l.Items[2].Kind != "question" || len(l.Items[2].Options) != 2 {
		t.Fatalf("question item %+v", l.Items[2])
	}

	if _, err := s.Assume(q.Key, 1, claude); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(r.Key, true, you); err != nil {
		t.Fatal(err)
	}
	l, _ = s.List("task", "ready", "")
	if len(l.Items) != 1 {
		t.Fatal("assumed question and done request should unblock the task")
	}
	if _, err := s.Tick(q.Key, true, you); err == nil {
		t.Fatal("ticking a question should fail")
	}
	res, err := s.AnswerQuestion(q.Key, "", "bookmarks keep working", you)
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer != "From the path" || len(res.Unblocked) != 1 || res.Unblocked[0] != task.Key {
		t.Fatalf("answer %+v", res)
	}

	qs, _ := s.Load(Questions)
	aq := qs.Find(q.Key)
	if aq == nil || !aq.Done || aq.Meta("answer") != "From the path" || aq.Meta("why") != "bookmarks keep working" || aq.Meta("answered-by") != "you" || aq.Assumed != 0 {
		t.Fatalf("question after answer: %q", aq.Raw)
	}
	l, _ = s.List("task", "open", "")
	it := l.Items[0]
	if it.Blocked || len(it.Needs) != 2 || len(it.Answers) != 1 || it.Answers[0].Answer != "From the path" {
		t.Fatalf("task after answer %+v", it)
	}
	if out, _ := s.Prime(Actor{Session: ""}); !strings.Contains(out, q.Key+` answered: "From the path"`) {
		t.Fatalf("prime doesn't show the answer:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".callboard")); err == nil {
		t.Fatal("answering made a side file")
	}

	rq, err := s.Reopen(q.Key, you)
	if err != nil {
		t.Fatal(err)
	}
	if rq.Done || rq.Meta("answer") != "" || rq.Meta("was") != "From the path" || len(rq.Options()) != 2 {
		t.Fatalf("reopened %q", rq.Raw)
	}
	l, _ = s.List("task", "ready", "")
	if len(l.Items) != 0 {
		t.Fatal("task should wait again once the question is reopened")
	}
	if _, err := s.AnswerQuestion(q.Key, "Always 4700, after all", "", you); err != nil {
		t.Fatal(err)
	}
	qs, _ = s.Load(Questions)
	if aq := qs.Find(q.Key); aq.Meta("answer") != "Always 4700, after all" || aq.Meta("was") != "" {
		t.Fatalf("second answer %q", aq.Raw)
	}
}

func TestOverturnedAssumptionReopensWork(t *testing.T) {
	s := repo(t)
	q, _ := s.Add(Questions, board.New{Title: "Count the last line?", Body: []string{"- Yes", "- No (recommended)"}}, claude)
	x, _ := s.Add(Backlog, board.New{Title: "Line counting", Needs: []string{q.Key}}, claude)
	s.SetFields(x.Key, [][2]string{{"status", "todo"}}, claude)
	y, _ := s.Add(Backlog, board.New{Title: "Docs for it", Needs: []string{q.Key}}, claude)
	s.Assume(q.Key, 2, claude)
	s.Tick(x.Key, true, claude)
	s.Prime(Actor{Session: "s1"})
	res, err := s.AnswerQuestion(q.Key, "1", "people expect it", you)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Reopened) != 1 || res.Reopened[0] != x.Key || res.Assumed != "No" {
		t.Fatalf("answer %+v", res)
	}
	f, _ := s.Load(Backlog)
	if it := f.Find(x.Key); it.Done || it.Meta("status") != "todo" {
		t.Fatalf("built on the wrong answer, still %q", it.Raw)
	}
	if f.Find(y.Key).Done {
		t.Fatal("an open item was touched")
	}
	if n := FormatNews(s.News("s1")); !strings.Contains(n, `was answered "Yes", not the assumed "No". Redo it`) {
		t.Fatalf("news:\n%s", n)
	}

	q2, _ := s.Add(Questions, board.New{Title: "Tabs?", Body: []string{"- A", "- B"}}, claude)
	z, _ := s.Add(Backlog, board.New{Title: "Tabs work", Needs: []string{q2.Key}}, claude)
	s.Assume(q2.Key, 1, claude)
	s.Tick(z.Key, true, claude)
	if res, _ := s.AnswerQuestion(q2.Key, "1", "", you); len(res.Reopened) != 0 || res.Assumed != "" {
		t.Fatalf("confirmed assumption %+v", res)
	}
}

func TestVersionsAndRename(t *testing.T) {
	s := repo(t)
	x, _ := s.AddTask("Old title", "", "claude")
	if _, err := s.Rename(x.Key+"@"+x.Version, "New title", you); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(x.Key+"@"+x.Version, true, claude); err == nil || !strings.Contains(err.Error(), "changed since you read it") {
		t.Fatalf("stale tick: %v", err)
	}
}

func TestClaims(t *testing.T) {
	s := repo(t)
	x, _ := s.AddTask("Work", "", "claude")
	if _, err := s.ClaimItem(x.Key, "claude", "s1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimItem(x.Key, "codex", "s2", false); err == nil || !strings.Contains(err.Error(), "claimed by claude") {
		t.Fatalf("double claim: %v", err)
	}
	l, _ := s.List("task", "open", "")
	if l.Items[0].Claim == nil || l.Items[0].Claim.Session != "s1" {
		t.Fatalf("claim not listed: %+v", l.Items[0])
	}
	s.Tick(x.Key, true, claude)
	if len(s.Claims()) != 0 {
		t.Fatal("tick should drop the claim")
	}
}

func TestNews(t *testing.T) {
	s := repo(t)
	x, _ := s.AddTask("Work", "", "claude")
	s.Prime(Actor{Session: "s1"})
	if n := s.News("s1"); len(n) != 0 {
		t.Fatalf("news right after prime: %+v", n)
	}
	s.Rename(x.Key, "Work, renamed by you", you)
	s.AddTask("Agent's own task", "", "claude")
	n := s.News("s1")
	if len(n) != 1 || n[0].What != "renamed" {
		t.Fatalf("news %+v", n)
	}
	if !strings.Contains(FormatNews(n), `The human renamed `+x.Key+` to "Work, renamed by you" (was "Work")`) {
		t.Fatal(FormatNews(n))
	}
	if n := s.News("s1"); len(n) != 0 {
		t.Fatal("news told twice")
	}
}

func TestAnswerReachesEveryWorktree(t *testing.T) {
	s := repo(t)
	q, _ := s.Add(Questions, board.New{Title: "Ship it?", Body: []string{"- Yes", "- No"}}, claude)
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = s.Root
	cmd.Run()
	cmd = exec.Command("git", "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "q")
	cmd.Dir = s.Root
	cmd.Run()
	other := filepath.Join(t.TempDir(), "wt")
	cmd = exec.Command("git", "worktree", "add", "-q", other, "-b", "feat")
	cmd.Dir = s.Root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	o := Open(other)
	o.Prime(Actor{Session: "s9"})
	_, where, err := Everywhere(s, q.Key, func(st *Store) (Answered, error) { return st.AnswerQuestion(q.Key, "1", "", you) })
	if err != nil || len(where) != 2 {
		t.Fatalf("answered in %v: %v", where, err)
	}
	for _, st := range []*Store{s, o} {
		f, _ := st.Load(Questions)
		if aq := f.Find(q.Key); aq == nil || !aq.Done || aq.Meta("answer") != "Yes" {
			t.Fatalf("%s not answered", st.Root)
		}
	}
	if n := o.News("s9"); len(n) != 1 || n[0].Detail != "Yes" {
		t.Fatalf("other worktree's news %+v", n)
	}
}

func TestPrimeTellsResumedSessionWhatChanged(t *testing.T) {
	s := repo(t)
	agent := Actor{By: "claude", Session: "s9"}
	if _, err := s.Add(Backlog, board.New{Title: "first"}, agent); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Prime(Actor{Session: "s9"})
	if strings.Contains(out, "Callboard news") {
		t.Fatalf("new session got news:\n%s", out)
	}
	s.Add(Backlog, board.New{Title: "while you were away"}, Actor{By: "you"})
	out, _ = s.Prime(Actor{Session: "s9"})
	if !strings.Contains(out, "Callboard news") || !strings.Contains(out, "while you were away") {
		t.Fatalf("resumed session missed the news:\n%s", out)
	}
	if n := s.News("s9"); len(n) != 0 {
		t.Fatalf("news given twice: %v", n)
	}
}

func TestFieldsMoveAndViews(t *testing.T) {
	s := repo(t)
	x, _ := s.Add(Backlog, board.New{Title: "Build the board", Section: "Now"}, claude)
	s.Add(Backlog, board.New{Title: "Other", Section: "Now"}, claude)
	if _, err := s.SetFields(x.Key, [][2]string{{"status", "doing"}, {"area", "page ui"}}, claude); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(x.Key, "Later", you); err != nil {
		t.Fatal(err)
	}
	l, _ := s.List("task", "open", "")
	it := l.Items[1]
	if it.Section != "Later" || it.Fields["status"] != "doing" || it.Fields["area"] != "page ui" {
		t.Fatalf("item %+v", it)
	}
	list, layout, group, name := "backlog", "board", "status", "By status"
	order := []string{"todo", "doing", "done"}
	v, err := s.SaveView(ViewSpec{Name: &name, List: &list, Layout: &layout, Group: &group, Order: &order}, claude)
	if err != nil || v.Layout != "board" || v.Group != "status" || len(v.Order) != 3 || v.List != "backlog" {
		t.Fatalf("view %+v %v", v, err)
	}
	table := "table"
	sortBy := []string{"-prio", "title"}
	typo := []string{"-priority"}
	if _, err := s.SaveView(ViewSpec{Key: v.Key, Sort: &typo}, you); err == nil || !strings.Contains(err.Error(), "no item has") {
		t.Fatalf("a field no item has was accepted: %v", err)
	}
	if _, err := s.SetFields(it.Key, [][2]string{{"prio", "high"}}, you); err != nil {
		t.Fatal(err)
	}
	filter := []string{"status!=done", "area=page ui|api"}
	if v, err = s.SaveView(ViewSpec{Key: v.Key, Layout: &table, Sort: &sortBy, Filter: &filter}, you); err != nil || v.Layout != "table" || v.Group != "status" || len(v.Filter) != 2 {
		t.Fatalf("change view %+v %v", v, err)
	}
	if vs := s.SavedViews(); len(vs) != 1 || vs[0].Sort[0] != "-prio" {
		t.Fatalf("saved %+v", vs)
	}
	bad := "grid"
	if _, err := s.SaveView(ViewSpec{Key: v.Key, Layout: &bad}, you); err == nil {
		t.Fatal("bad layout accepted")
	}
	out, _ := s.Prime(Actor{Session: ""})
	if !strings.Contains(out, "Fields in use: area (page ui); prio (high); status (doing)") || !strings.Contains(out, "Views on the page: By status ("+v.Key+")") {
		t.Fatalf("prime:\n%s", out)
	}
	s.Prime(Actor{Session: "s7"})
	s.SetFields(x.Key, [][2]string{{"prio", "high"}}, you)
	s.SaveView(ViewSpec{Key: v.Key, Group: &group}, you)
	s.SaveView(ViewSpec{Key: v.Key, Layout: &layout}, you)
	news := FormatNews(s.News("s7"))
	if !strings.Contains(news, `set `+x.Key+` "Build the board": prio: high`) || strings.Count(news, "changed the view") != 1 {
		t.Fatalf("news:\n%s", news)
	}
	if err := s.DeleteView(v.Key, you); err != nil || len(s.SavedViews()) != 0 {
		t.Fatal("delete view", err)
	}
}

func TestStatusFollowsTickAndClaims(t *testing.T) {
	s := repo(t)
	x, _ := s.Add(Backlog, board.New{Title: "Fix it"}, claude)
	y, _ := s.Add(Backlog, board.New{Title: "Other"}, claude)
	w, _ := s.Add(Backlog, board.New{Title: "Waiting"}, claude)
	s.SetFields(w.Key, [][2]string{{"status", "to do"}}, claude)
	s.SetFields(x.Key, [][2]string{{"status", "to do"}}, claude)
	s.SetFields(y.Key, [][2]string{{"status", "In progress"}}, claude)
	status := func(key string) (string, bool) {
		f, _ := s.Load(Backlog)
		it := f.Find(key)
		return it.Meta("status"), it.Done
	}
	if _, err := s.ClaimItem(x.Key, "claude", "s1", false); err != nil {
		t.Fatal(err)
	}
	if st, _ := status(x.Key); st != "In progress" {
		t.Fatalf("claimed: status %q, want the list's own word for doing", st)
	}
	s.Release(x.Key, "claude", "s1", false)
	if st, _ := status(x.Key); st != "to do" {
		t.Fatalf("released: status %q", st)
	}
	s.ClaimItem(x.Key, "claude", "s1", false)
	s.Tick(x.Key, true, claude)
	if st, done := status(x.Key); st != "done" || !done {
		t.Fatalf("ticked: %q %v", st, done)
	}
	s.Tick(x.Key, false, you)
	if st, done := status(x.Key); st != "to do" || done {
		t.Fatalf("opened again: %q %v", st, done)
	}

	s.SetFields(y.Key, [][2]string{{"status", "shipped"}}, you)
	if _, done := status(y.Key); !done {
		t.Fatal("status shipped should tick it")
	}
	s.SetFields(y.Key, [][2]string{{"status", "In progress"}}, you)
	if _, done := status(y.Key); done {
		t.Fatal("status back to in progress should open it")
	}

	z, _ := s.Add(Requests, board.New{Title: "Make a key"}, claude)
	s.Tick(z.Key, true, you)
	f, _ := s.Load(Requests)
	if f.Find(z.Key).Meta("status") != "" {
		t.Fatal("status added to an item without one")
	}
}

func TestOwnChangesDontMakeVersionsStale(t *testing.T) {
	s := repo(t)
	x, _ := s.Add(Backlog, board.New{Title: "Fix it"}, claude)
	x, _ = s.SetFields(x.Key, [][2]string{{"status", "todo"}}, claude)
	read := x.Key + "@" + x.Version
	s.ClaimItem(x.Key, "claude", "s1", false)
	if _, err := s.SetFields(read, [][2]string{{"prio", "high"}}, claude); err != nil {
		t.Fatalf("own claim made the version stale: %v", err)
	}
	if _, err := s.Tick(read, true, claude); err != nil {
		t.Fatalf("own changes made the version stale: %v", err)
	}

	other := Actor{By: "claude", Session: "s2"}
	if _, err := s.Rename(read, "Renamed", other); err == nil {
		t.Fatal("another session's stale version was taken")
	}

	y, _ := s.Add(Backlog, board.New{Title: "Other"}, claude)
	r := y.Key + "@" + y.Version
	s.SetFields(y.Key, [][2]string{{"area", "a"}}, claude)
	s.Rename(y.Key, "Renamed by you", you)
	if _, err := s.Tick(r, true, claude); err == nil {
		t.Fatal("stale after the human's rename, but taken")
	}
}

func TestSuggestedViews(t *testing.T) {
	s := repo(t)
	name, layout, why, empty := "By priority", "table", "see the urgent work first", " "
	sortBy := []string{"-prio"}
	ship, _ := s.Add(Backlog, board.New{Title: "Ship"}, claude)
	if _, err := s.SetFields(ship.Key, [][2]string{{"prio", "high"}}, claude); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveView(ViewSpec{Name: &name, Suggest: &empty}, claude); err == nil {
		t.Fatal("a suggestion without why was saved")
	}
	v, err := s.SaveView(ViewSpec{Name: &name, Layout: &layout, Sort: &sortBy, Suggest: &why}, claude)
	if err != nil || !v.Suggested || v.Why != why {
		t.Fatalf("suggested: %+v %v", v, err)
	}
	p, _ := s.Prime(Actor{Session: "s1"})
	if !strings.Contains(p, "By priority ("+v.Key+"), suggested, not kept yet") {
		t.Fatalf("prime doesn't say it's suggested:\n%s", p)
	}
	if v, err = s.SaveView(ViewSpec{Key: v.Key, Keep: true}, you); err != nil || v.Suggested || v.Why != "" || v.Layout != "table" {
		t.Fatalf("kept: %+v %v", v, err)
	}
}

func TestCheckView(t *testing.T) {
	s := repo(t)
	for _, x := range []struct{ title, fields string }{{"A", "prio=high area=api"}, {"B", "prio=low area=page"}, {"C", "prio=urgent area=api"}} {
		it, _ := s.Add(Backlog, board.New{Title: x.title}, claude)
		var kv [][2]string
		for _, f := range strings.Fields(x.fields) {
			k, v, _ := strings.Cut(f, "=")
			kv = append(kv, [2]string{k, v})
		}
		if _, err := s.SetFields(it.Key, kv, claude); err != nil {
			t.Fatal(err)
		}
	}
	c := s.CheckView(BoardView{List: "backlog", Layout: "board", Group: "area", Sort: []string{"-priority"}, Filter: []string{"area=api|docs", "done=no"}})
	if !strings.Contains(c.Shows, "2 of 3 tasks") || !strings.Contains(c.Shows, "columns by area: api 2") {
		t.Errorf("shows: %s", c.Shows)
	}
	w := strings.Join(c.Warnings, "\n")
	if !strings.Contains(w, `"priority"`) || !strings.Contains(w, `did you mean "prio"`) || !strings.Contains(w, "area=docs") {
		t.Errorf("warnings:\n%s", w)
	}
	if c := s.CheckView(BoardView{List: "backlog", Layout: "list", Filter: []string{"done=done"}}); !strings.Contains(strings.Join(c.Warnings, " "), "shows nothing") {
		t.Errorf("an empty view isn't pointed out: %+v", c)
	}
	c = s.CheckView(BoardView{List: "backlog", Layout: "board", Group: "claimed", Order: []string{"claude", "unclaimed"}})
	if w := strings.Join(c.Warnings, " "); !strings.Contains(w, "claimed=unclaimed") || !strings.Contains(c.Shows, "nobody on it 3") {
		t.Errorf("order: %+v", c)
	}
}

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AsWali/CallBoard/internal/board"
)

func TestOwnLists(t *testing.T) {
	s := repo(t)
	gitIn(t, s.Root, "config", "merge.callboard.driver", "callboard merge %O %A %B %L %P")
	k, made, err := s.AddList("Ideas", claude)
	if err != nil || !made || k.File != "ideas.md" || k.Prefix != "I-" {
		t.Fatalf("add: %+v %v %v", k, made, err)
	}
	if _, made, _ := s.AddList("ideas", claude); made {
		t.Fatal("added twice")
	}
	if _, _, err := s.AddList("backlog", claude); err == nil {
		t.Fatal("took a built-in name")
	}
	if _, _, err := s.AddList("my ideas", claude); err == nil {
		t.Fatal("took a name with a space")
	}
	b, _, _ := s.AddList("inbox", claude)
	if b.Prefix != "N-" {
		t.Fatalf("inbox got %s, want N- (I- is taken)", b.Prefix)
	}
	attrs, _ := os.ReadFile(filepath.Join(s.Root, ".gitattributes"))
	if !strings.Contains(string(attrs), "ideas.md merge=callboard") || !strings.Contains(string(attrs), ".callboard/lists.md merge=union") {
		t.Fatalf(".gitattributes: %q", attrs)
	}

	named, err := s.KindNamed("ideas")
	if err != nil || named != k {
		t.Fatalf("named: %+v %v", named, err)
	}
	x, err := s.Add(k, board.New{Title: "Dark mode"}, claude)
	if err != nil || !strings.HasPrefix(x.Key, "I-") {
		t.Fatalf("add item: %+v %v", x, err)
	}
	sub, err := s.Add(k, board.New{Title: "Pick colours", Parent: x.Key}, claude)
	if err != nil || !strings.HasPrefix(sub.Key, "I-") {
		t.Fatalf("subtask: %+v %v", sub, err)
	}
	if _, err := s.ClaimItem(sub.Key, "claude", "s1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Tick(sub.Key, true, claude); err != nil {
		t.Fatal(err)
	}
	l, _ := s.List("ideas", "all", "")
	if len(l.Items) != 2 || l.Items[0].List != "ideas" || l.Items[0].Kind != "task" {
		t.Fatalf("list: %+v", l.Items)
	}
	if out := FormatList(l); !strings.Contains(out, "ideas.md") || !strings.Contains(out, "done by claude on ") {
		t.Fatalf("format:\n%s", out)
	}
	if kk, ok := s.ListFile(filepath.Join(s.Root, "ideas.md")); !ok || kk != k {
		t.Fatal("ideas.md isn't known as a list")
	}
	p, _ := s.Prime(Actor{Session: "s2"})
	if !strings.Contains(p, "ideas.md: 1 open, 1 done") || !strings.Contains(p, "Own lists") {
		t.Fatalf("prime:\n%s", p)
	}
	if _, err := s.KindNamed("nope"); err == nil || !strings.Contains(err.Error(), "ideas, inbox") {
		t.Fatalf("unknown kind: %v", err)
	}
}

func TestSubtaskReopensDoneParent(t *testing.T) {
	s := repo(t)
	p, _ := s.Add(Backlog, board.New{Title: "Export"}, claude)
	s.Tick(p.Key, true, claude)
	if _, err := s.Add(Backlog, board.New{Title: "Header row", Parent: p.Key}, claude); err != nil {
		t.Fatal(err)
	}
	v, _ := s.View()
	if it := v.Item(Backlog, v.Files[Backlog.Name].Find(p.Key)); it.Done || it.SubsOpen != 1 {
		t.Fatalf("parent after a new subtask: %+v", it)
	}
}

func TestListsSharingALetter(t *testing.T) {
	s := repo(t)
	os.MkdirAll(filepath.Join(s.Root, ".callboard"), 0o755)
	os.WriteFile(filepath.Join(s.Root, listsFile), []byte(listsHead+"- ideas.md · keys I-\n- inbox.md · keys I-\n"), 0o644)
	os.WriteFile(filepath.Join(s.Root, "ideas.md"), []byte("# Ideas\n\n- [ ] Dark mode <!-- id:I-aaaa by:you at:2026-09-01 -->\n"), 0o644)
	os.WriteFile(filepath.Join(s.Root, "inbox.md"), []byte("# Inbox\n\n- [ ] Reply to Sam <!-- id:I-aaab by:you at:2026-09-01 -->\n"), 0o644)
	if n := len(s.Custom()); n != 2 {
		t.Fatalf("custom lists: %d, want 2", n)
	}
	if k, _ := s.KindOf("I-aaab"); k.Name != "inbox" {
		t.Fatalf("I-aaab is in %s, want inbox", k.Name)
	}
	if k, _ := s.KindOf("I-aaaa"); k.Name != "ideas" {
		t.Fatalf("I-aaaa is in %s, want ideas", k.Name)
	}
	if _, err := s.Tick("I-aaab", true, claude); err != nil {
		t.Fatal(err)
	}
	inbox, _ := s.KindNamed("inbox")
	f, _ := s.Load(inbox)
	if !f.Find("I-aaab").Done {
		t.Fatal("inbox item not ticked")
	}
	if !f.Taken("I-aaaa") || f.Taken("I-zzzz") {
		t.Fatal("new keys in inbox must skip the keys ideas uses")
	}
	if got := s.Sharing(inbox); len(got) != 1 || got[0] != "ideas" {
		t.Fatalf("sharing: %v", got)
	}
}

func TestRenameAndRemoveLists(t *testing.T) {
	s := repo(t)
	gitIn(t, s.Root, "config", "merge.callboard.driver", "callboard merge %O %A %B %L %P")
	k, _, _ := s.AddList("ideas", claude)
	it, _ := s.Add(k, board.New{Title: "Dark mode"}, claude)
	gitIn(t, s.Root, "add", "-A")
	gitIn(t, s.Root, "commit", "-qm", "lists")
	if _, err := s.SaveView(ViewSpec{Name: ptr("Ideas board"), List: ptr("ideas"), Layout: ptr("board")}, claude); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameList("backlog", "todo", claude); err == nil {
		t.Fatal("renamed the backlog")
	}
	nk, err := s.RenameList("ideas", "someday", claude)
	if err != nil || nk.File != "someday.md" || nk.Prefix != "I-" {
		t.Fatalf("rename: %+v %v", nk, err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "ideas.md")); !os.IsNotExist(err) {
		t.Fatal("ideas.md still there")
	}
	b, _ := os.ReadFile(filepath.Join(s.Root, "someday.md"))
	if !strings.HasPrefix(string(b), "# Someday\n") || !strings.Contains(string(b), it.Key) {
		t.Fatalf("someday.md: %q", b)
	}
	if k, _ := s.KindOf(it.Key); k.Name != "someday" {
		t.Fatalf("%s is in %s", it.Key, k.Name)
	}
	attrs, _ := os.ReadFile(filepath.Join(s.Root, ".gitattributes"))
	if !strings.Contains(string(attrs), "someday.md merge=callboard") || strings.Contains(string(attrs), "ideas.md") {
		t.Fatalf(".gitattributes: %q", attrs)
	}
	if vs := s.SavedViews(); len(vs) != 1 || vs[0].List != "someday" {
		t.Fatalf("views: %+v", vs)
	}
	if _, _, err := s.RemoveList("someday", false, claude); err == nil || !strings.Contains(err.Error(), "1 open") {
		t.Fatalf("removed a list with open items: %v", err)
	}
	gone, views, err := s.RemoveList("someday", true, claude)
	if err != nil || gone.File != "someday.md" || len(views) != 1 {
		t.Fatalf("remove: %v %v", views, err)
	}
	if len(s.Custom()) != 0 || len(s.SavedViews()) != 0 {
		t.Fatal("list or view left behind")
	}
	if _, err := os.Stat(filepath.Join(s.Root, "someday.md")); !os.IsNotExist(err) {
		t.Fatal("someday.md still there")
	}
}

func ptr(s string) *string { return &s }

func TestRenameListIsQuiet(t *testing.T) {
	s := repo(t)
	k, _, _ := s.AddList("ideas", claude)
	s.Add(k, board.New{Title: "Dark mode"}, claude)
	s.Reconcile()
	if _, err := s.RenameList("ideas", "someday", claude); err != nil {
		t.Fatal(err)
	}
	if evs := s.Reconcile(); len(evs) != 0 {
		t.Fatalf("a rename looked like hand edits: %+v", evs)
	}
	news := FormatNews(s.Events(0)[:1])
	if !strings.Contains(news, "renamed the list ideas.md to someday.md") {
		t.Fatalf("news: %s", news)
	}
}

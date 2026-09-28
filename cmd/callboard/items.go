package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/store"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func list(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	kind := fs.String("kind", "all", "task, request, question or all")
	all := fs.Bool("all", false, "open and done items")
	done := fs.Bool("done", false, "only done items")
	ready := fs.Bool("ready", false, "only open items that aren't waiting on anything")
	section := fs.String("section", "", "only this heading")
	find := fs.String("find", "", "words that must all be in the title, notes or fields")
	from := fs.Int("from", 0, "start at this item, counting from 0")
	limit := fs.Int("limit", -1, "how many items to show; 0 shows them all (default: all, or 50 when an agent asks)")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	status := "open"
	switch {
	case *find != "" && !*done && !*ready:
		status = "all"
	case *all:
		status = "all"
	case *done:
		status = "done"
	case *ready:
		status = "ready"
	}
	l, err := store.Open(wd()).List(*kind, status, *section)
	if err != nil {
		return err
	}
	if *limit < 0 {
		*limit = 0
		if store.InAgent() {
			*limit = store.PageSize
		}
	}
	l = store.Open(wd()).Search(l, *find).Page(*from, *limit)
	if *asJSON {
		return printJSON(l)
	}
	fmt.Print(store.FormatList(l))
	return nil
}

func add(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	kind := fs.String("kind", "task", "task, request or question")
	section := fs.String("section", "", "heading to add it under")
	needs := fs.String("needs", "", "keys it waits on, comma-separated")
	under := fs.String("under", "", "add it as a subtask of this item")
	rec := fs.Int("recommended", 0, "which option you recommend (1-based)")
	var body, opts multi
	fs.Var(&body, "body", "a line under it (repeat for more)")
	fs.Var(&opts, "option", "an option for a question (repeat for more)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 {
		return errors.New(`add needs a title: callboard add "Parse the three files"`)
	}
	k, err := store.Open(wd()).KindNamed(*kind)
	if err != nil {
		return err
	}
	if len(opts) > 0 && k != store.Questions {
		return errors.New("--option is for questions: add --kind question")
	}
	var lines []string
	for i, o := range opts {
		lines = append(lines, board.OptionLine(o, i+1 == *rec))
	}
	lines = append(lines, body...)
	var nd []string
	for _, n := range strings.Split(*needs, ",") {
		if n = strings.TrimSpace(n); n != "" {
			nd = append(nd, n)
		}
	}
	t, err := store.Open(wd()).Add(k, board.New{Title: strings.Join(pos, " "), Section: *section, Body: lines, Needs: nd, Parent: *under}, actor())
	if err != nil {
		return err
	}
	where := ""
	switch {
	case t.Parent != "":
		where = " under " + t.Parent
	case t.Section != "":
		where = " under ## " + t.Section
	}
	fmt.Printf("%s@%s  %s%s (%s)\n", t.Key, t.Version, t.Title, where, k.File)
	return nil
}

func one(name string, args []string, fs *flag.FlagSet, n int, usage string) ([]string, error) {
	pos, err := parse(fs, args)
	if err != nil {
		return nil, err
	}
	if len(pos) < n {
		return nil, fmt.Errorf("%s: %s", name, usage)
	}
	return pos, nil
}

func tick(args []string) error {
	fs := flag.NewFlagSet("tick", flag.ContinueOnError)
	undo := fs.Bool("undo", false, "open it again")
	pos, err := one("tick", args, fs, 1, "callboard tick B-k3f9")
	if err != nil {
		return err
	}
	t, err := store.Open(wd()).Tick(pos[0], !*undo, actor())
	if err != nil {
		return err
	}
	state := "done"
	if !t.Done {
		state = "open"
	}
	fmt.Printf("%s@%s  %s: %s\n", t.Key, t.Version, t.Title, state)
	return nil
}

func rename(args []string) error {
	pos, err := one("rename", args, flag.NewFlagSet("rename", flag.ContinueOnError), 2, `callboard rename B-k3f9 "new title"`)
	if err != nil {
		return err
	}
	t, err := store.Open(wd()).Rename(pos[0], strings.Join(pos[1:], " "), actor())
	if err != nil {
		return err
	}
	fmt.Printf("%s@%s  %s\n", t.Key, t.Version, t.Title)
	return nil
}

func note(args []string) error {
	fs := flag.NewFlagSet("note", flag.ContinueOnError)
	replace := fs.Bool("replace", false, "replace all of its notes")
	clear := fs.Bool("clear", false, "remove its notes")
	n := 2
	if slices.Contains(args, "--clear") {
		n = 1
	}
	pos, err := one("note", args, fs, n, `callboard note B-k3f9 "text"   (--replace swaps all its notes for it, --clear removes them)`)
	if err != nil {
		return err
	}
	text := strings.Join(pos[1:], " ")
	if *clear {
		text = ""
	}
	t, err := store.Open(wd()).SetNotes(pos[0], text, !*replace && !*clear, actor())
	if err != nil {
		return err
	}
	fmt.Printf("%s@%s  %s\n", t.Key, t.Version, t.Title)
	for _, b := range t.Body {
		fmt.Println(b)
	}
	return nil
}

func resolve(args []string) error {
	pos, err := one("resolve", args, flag.NewFlagSet("resolve", flag.ContinueOnError), 2, "callboard resolve B-k3f9 1   (1 keeps the first version, 2 the second)")
	if err != nil {
		return err
	}
	side, err := strconv.Atoi(pos[1])
	if err != nil {
		return fmt.Errorf("keep 1 or 2, not %q", pos[1])
	}
	st := store.Open(wd())
	kept, err := st.Resolve(pos[0], side, actor())
	if err != nil {
		return err
	}
	key := strings.ToUpper(strings.SplitN(pos[0], "@", 2)[0])
	found := false
	for _, t := range kept {
		fmt.Printf("kept %s@%s  %s\n", t.Key, t.Version, t.Title)
		found = found || strings.EqualFold(t.Key, key)
	}
	if !found {
		fmt.Printf("removed %s: version %d is the one without it\n", key, side)
	}
	if v, err := st.View(); err == nil && len(v.Merges()) == 0 {
		fmt.Println("No unfinished merges left in the lists: git add them, then commit (or git rebase --continue) to finish.")
	}
	return nil
}

func needs(args []string) error {
	fs := flag.NewFlagSet("needs", flag.ContinueOnError)
	none := fs.Bool("none", false, "wait on nothing")
	pos, err := one("needs", args, fs, 1, "callboard needs B-k3f9 Q-7x1c R-2m8a")
	if err != nil {
		return err
	}
	var nd []string
	for _, p := range pos[1:] {
		for _, n := range strings.Split(p, ",") {
			if n = strings.TrimSpace(n); n != "" {
				nd = append(nd, n)
			}
		}
	}
	if len(nd) == 0 && !*none {
		return errors.New("say what it waits on (callboard needs B-k3f9 Q-7x1c), or --none")
	}
	t, err := store.Open(wd()).SetNeeds(pos[0], nd, actor())
	if err != nil {
		return err
	}
	what := strings.Join(t.Needs, ", ")
	if what == "" {
		what = "nothing"
	}
	fmt.Printf("%s@%s  %s: waits on %s\n", t.Key, t.Version, t.Title, what)
	return nil
}

func claim(args []string) error {
	fs := flag.NewFlagSet("claim", flag.ContinueOnError)
	force := fs.Bool("force", false, "take it over from another claim, or take an item that is for someone else")
	pos, err := one("claim", args, fs, 1, "callboard claim B-k3f9")
	if err != nil {
		return err
	}
	a := actor()
	if a.Session == "" {
		a.Session = "cli-" + a.By
	}
	c, err := store.Open(wd()).ClaimItem(strings.SplitN(pos[0], "@", 2)[0], a.By, a.Session, *force)
	if err != nil {
		return err
	}
	fmt.Printf("%s claimed by %s (lasts while that session runs, or %s after it goes quiet; tick drops it)\n", c.Key, c.By, store.ClaimTTL)
	return nil
}

func release(args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	force := fs.Bool("force", false, "drop anyone's claim")
	pos, err := one("release", args, fs, 1, "callboard release B-k3f9")
	if err != nil {
		return err
	}
	a := actor()
	if a.Session == "" {
		a.Session = "cli-" + a.By
	}
	if err := store.Open(wd()).Release(strings.SplitN(pos[0], "@", 2)[0], a.By, a.Session, *force); err != nil {
		return err
	}
	fmt.Printf("%s released\n", pos[0])
	return nil
}

func assume(args []string) error {
	pos, err := one("assume", args, flag.NewFlagSet("assume", flag.ContinueOnError), 2, "callboard assume Q-7x1c 1")
	if err != nil {
		return err
	}
	n, err := strconv.Atoi(pos[1])
	if err != nil {
		return fmt.Errorf("the option is a number: callboard assume %s 1", pos[0])
	}
	t, err := store.Open(wd()).Assume(pos[0], n, actor())
	if err != nil {
		return err
	}
	fmt.Printf("%s@%s  assumed option %d: %s\n", t.Key, t.Version, n, t.Options()[n-1].Text)
	return nil
}

func answer(args []string) error {
	fs := flag.NewFlagSet("answer", flag.ContinueOnError)
	why := fs.String("why", "", "the reason")
	pos, err := one("answer", args, fs, 1, `callboard answer Q-7x1c 1 --why "reason"`)
	if err != nil {
		return err
	}
	s := store.Open(wd())
	a := store.Actor{By: "you", Session: os.Getenv("CALLBOARD_SESSION")}
	res, where, err := store.Everywhere(s, pos[0], func(w *store.Store) (store.Answered, error) {
		return w.AnswerQuestion(pos[0], strings.Join(pos[1:], " "), *why, a)
	})
	if err != nil {
		return err
	}
	fmt.Printf("%s@%s answered: %s\n", res.Key, res.Version, res.Answer)
	if len(res.Unblocked) > 0 {
		fmt.Printf("  no longer waiting on it: %s\n", strings.Join(res.Unblocked, ", "))
	}
	if len(where) > 1 {
		fmt.Printf("  answered in %d worktrees: %s\n", len(where), strings.Join(where, ", "))
	}
	return nil
}

func reopen(args []string) error {
	pos, err := one("reopen", args, flag.NewFlagSet("reopen", flag.ContinueOnError), 1, "callboard reopen Q-7x1c")
	if err != nil {
		return err
	}
	s := store.Open(wd())
	t, _, err := store.Everywhere(s, pos[0], func(w *store.Store) (*board.Task, error) { return w.Reopen(pos[0], actor()) })
	if err != nil {
		return err
	}
	fmt.Printf("%s@%s  %s: open again (it was answered %q)\n", t.Key, t.Version, t.Title, t.Meta("was"))
	return nil
}

func news() error {
	s := store.Open(wd())
	session := os.Getenv("CALLBOARD_SESSION")
	if session == "" {
		session = "cli-" + store.By()
	}
	text := store.FormatNews(s.News(session))
	if text == "" {
		text = "No news.\n"
	}
	fmt.Print(text)
	return nil
}

func keys() error {
	s := store.Open(wd())
	n := 0
	for _, k := range s.Lists() {
		got, err := s.Adopt(k)
		if err != nil {
			return err
		}
		n += got
	}
	fmt.Printf("✓ gave %d item(s) a key\n", n)
	return nil
}

func deleteItem(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	pos, err := one("delete", args, fs, 1, "callboard delete B-k3f9")
	if err != nil {
		return err
	}
	st := store.Open(wd())
	for _, key := range pos {
		t, freed, err := st.Delete(key, actor())
		if err != nil {
			return err
		}
		msg := fmt.Sprintf("deleted %s %s; callboard restore %s brings it back", t.Key, t.Title, t.Key)
		if len(freed) > 0 {
			verb := " no longer waits on it"
			if len(freed) > 1 {
				verb = " no longer wait on it"
			}
			msg += "; " + strings.Join(freed, ", ") + verb
		}
		fmt.Println(msg)
	}
	return nil
}

func restore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	pos, err := one("restore", args, fs, 1, "callboard restore B-k3f9")
	if err != nil {
		return err
	}
	st := store.Open(wd())
	for _, key := range pos {
		t, err := st.Restore(key, actor())
		if err != nil {
			return err
		}
		fmt.Println("brought back " + st.Line(t))
	}
	return nil
}

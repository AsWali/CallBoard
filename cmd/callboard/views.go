package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/store"
)

func setFields(args []string) error {
	pos, err := one("set", args, flag.NewFlagSet("set", flag.ContinueOnError), 2, `callboard set B-k3f9 status=doing prio=high area="page ui"   (field= removes one)`)
	if err != nil {
		return err
	}
	var fields [][2]string
	for _, kv := range pos[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("set: %q isn't field=value", kv)
		}
		fields = append(fields, [2]string{strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`)})
	}
	t, err := store.Open(wd()).SetFields(pos[0], fields, actor())
	if err != nil {
		return err
	}
	var shown []string
	for _, f := range t.Fields() {
		shown = append(shown, f[0]+"="+f[1])
	}
	fmt.Printf("%s@%s  %s  %s\n", t.Key, t.Version, t.Title, strings.Join(shown, " "))
	return nil
}

func move(args []string) error {
	fs := flag.NewFlagSet("move", flag.ContinueOnError)
	before := fs.String("before", "", "put it just before this item")
	after := fs.String("after", "", "put it just after this item")
	pos, err := one("move", args, fs, 1, `callboard move B-k3f9 "Later" | --before B-x2p1 | --after B-x2p1`)
	if err != nil {
		return err
	}
	st, a := store.Open(wd()), actor()
	var t *board.Task
	switch {
	case *before != "" && *after != "":
		return fmt.Errorf("move: give --before or --after, not both")
	case *before != "" || *after != "":
		if len(pos) > 1 {
			return fmt.Errorf("move: give a heading or --before/--after, not both")
		}
		target := *before + *after
		t, err = st.Place(pos[0], target, *after != "", a)
	case len(pos) > 1:
		t, err = st.Move(pos[0], strings.Join(pos[1:], " "), a)
	default:
		return fmt.Errorf(`move: callboard move B-k3f9 "Later" | --before B-x2p1 | --after B-x2p1`)
	}
	if err != nil {
		return err
	}
	fmt.Println(st.Line(t))
	return nil
}

func views(args []string) error {
	fs := flag.NewFlagSet("views", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parse(fs, args); err != nil {
		return err
	}
	vs := store.Open(wd()).SavedViews()
	if *asJSON {
		if vs == nil {
			vs = []store.BoardView{}
		}
		return printJSON(vs)
	}
	if len(vs) == 0 {
		fmt.Println(`No saved views. Make one: callboard view --name "By status" --layout board --group status`)
	}
	for _, v := range vs {
		fmt.Println(v.Describe())
	}
	return nil
}

func view(args []string) error {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	var spec store.ViewSpec
	str := func(name, usage string, dst **string) {
		fs.Func(name, usage, func(v string) error { *dst = &v; return nil })
	}
	list := func(name, usage string, dst **[]string) {
		fs.Func(name, usage, func(v string) error {
			var l []string
			for _, x := range strings.Split(v, ",") {
				if x = strings.TrimSpace(x); x != "" {
					l = append(l, x)
				}
			}
			*dst = &l
			return nil
		})
	}
	str("name", "what the view is called", &spec.Name)
	str("list", "backlog, requests or questions", &spec.List)
	str("layout", "list, board or table", &spec.Layout)
	str("group", "the field to group by (a board's columns)", &spec.Group)
	list("order", "the groups in order: todo,doing,done", &spec.Order)
	list("sort", "fields to sort by: -prio,title", &spec.Sort)
	list("show", "fields shown on each item: status,prio", &spec.Show)
	var filters multi
	fs.Var(&filters, "filter", "field=value, field!=value or field=a|b (repeat; --filter= clears)")
	del := fs.Bool("delete", false, "remove the view")
	str("suggest", "propose it to the human instead of adding it: why it helps", &spec.Suggest)
	fs.BoolVar(&spec.Keep, "keep", false, "keep a suggested view")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "filter" {
			var l []string
			for _, x := range filters {
				if x != "" {
					l = append(l, x)
				}
			}
			spec.Filter = &l
		}
	})
	s := store.Open(wd())
	if len(pos) > 0 {
		spec.Key = pos[0]
	}
	if *del {
		if spec.Key == "" {
			return fmt.Errorf("view --delete V-KEY")
		}
		if err := s.DeleteView(spec.Key, actor()); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", spec.Key)
		return nil
	}
	if spec.Key == "" && spec.Name == nil {
		return fmt.Errorf(`a new view needs a name: callboard view --name "By status" --layout board --group status`)
	}
	v, err := s.SaveView(spec, actor())
	if err != nil {
		return err
	}
	fmt.Println(s.Checked(v))
	return nil
}

func section(args []string) error {
	fs := flag.NewFlagSet("section", flag.ContinueOnError)
	var c store.SectionChange
	fs.StringVar(&c.Kind, "kind", "task", "which list: task, request or question")
	fs.StringVar(&c.To, "to", "", "rename it")
	fs.StringVar(&c.Before, "before", "", "move it just before this section")
	fs.StringVar(&c.After, "after", "", "move it just after this section")
	fs.BoolVar(&c.Remove, "remove", false, "remove the heading")
	fs.StringVar(&c.Into, "into", "", "with --remove, the section its items go to")
	usage := `callboard section "Later" --to "Someday" | --before "Now" | --after "Next" | --remove [--into "Next"]`
	pos, err := one("section", args, fs, 1, usage)
	if err != nil {
		return err
	}
	c.Name = strings.Join(pos, " ")
	msg, err := store.Open(wd()).ChangeSection(c, actor())
	if err != nil {
		return fmt.Errorf("section: %w", err)
	}
	fmt.Println(msg)
	return nil
}

func lists(args []string) error {
	st := store.Open(wd())
	if len(args) > 0 && args[0] == "add" {
		if len(args) != 2 {
			return errors.New(`lists add needs one name: callboard lists add ideas`)
		}
		k, made, err := st.AddList(args[1], actor())
		if err != nil {
			return fmt.Errorf("lists: %w", err)
		}
		if !made {
			fmt.Printf("✓ %s is already a list (keys %s)\n", k.File, k.Prefix)
			return nil
		}
		fmt.Printf("✓ added %s, a task list with keys %s; commit it with .callboard/lists.md and .gitattributes\n  add to it: callboard add \"…\" --kind %s\n", k.File, k.Prefix, k.Name)
		return nil
	}
	if len(args) > 0 && args[0] == "rename" {
		if len(args) != 3 {
			return errors.New(`lists rename needs the old and new name: callboard lists rename ideas someday`)
		}
		k, err := st.RenameList(args[1], args[2], actor())
		if err != nil {
			return fmt.Errorf("lists: %w", err)
		}
		fmt.Printf("✓ renamed it to %s; its keys still start %s. Commit it with .callboard/lists.md and .gitattributes\n", k.File, k.Prefix)
		return nil
	}
	if len(args) > 0 && (args[0] == "remove" || args[0] == "rm") {
		force := len(args) == 3 && (args[2] == "--force" || args[2] == "-f")
		if len(args) != 2 && !force {
			return errors.New(`lists remove needs one name: callboard lists remove ideas [--force]`)
		}
		k, views, err := st.RemoveList(args[1], force, actor())
		if err != nil {
			return fmt.Errorf("lists: %w", err)
		}
		fmt.Printf("✓ removed %s", k.File)
		if len(views) > 0 {
			fmt.Printf(" and its views (%s)", strings.Join(views, ", "))
		}
		fmt.Printf("; once committed, git log -- %s still has it\n", k.File)
		return nil
	}
	if len(args) > 0 {
		return errors.New("callboard lists [add NAME | rename NAME NEW | remove NAME [--force]]")
	}
	for _, k := range st.Lists() {
		what := k.Noun + "s"
		if k.Own() {
			what = "your own list of tasks"
			if o := st.Sharing(k); len(o) > 0 {
				what += "; shares its key letter with " + strings.Join(o, ", ")
			}
		}
		fmt.Printf("%-16s keys %s  %s\n", k.File, k.Prefix, what)
	}
	return nil
}

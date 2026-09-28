package store

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

var Views = Kind{"views", ".callboard/views.md", "V-", "view"}

const viewsHead = "# Views\n"

type BoardView struct {
	Key     string   `json:"key"`
	Version string   `json:"version"`
	Name    string   `json:"name"`
	List    string   `json:"list"`
	Layout  string   `json:"layout"`
	Group   string   `json:"group,omitempty"`
	Order   []string `json:"order,omitempty"`
	Sort    []string `json:"sort,omitempty"`
	Filter  []string `json:"filter,omitempty"`
	Show    []string `json:"show,omitempty"`
	By      string   `json:"by,omitempty"`

	Suggested bool   `json:"suggested,omitempty"`
	Why       string `json:"why,omitempty"`
}

type ViewSpec struct {
	Key                       string
	Name                      *string
	List, Layout, Group       *string
	Order, Sort, Filter, Show *[]string

	Suggest *string
	Keep    bool
}

var BuiltinFields = []string{"section", "done", "by", "at", "doneat", "kind", "blocked", "claimed", "key", "needs"}

var Layouts = []string{"list", "board", "table"}

func viewOf(t *board.Task) BoardView {
	split := func(s string) []string {
		var out []string
		for _, x := range strings.Split(s, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	v := BoardView{Key: t.Key, Version: t.Version, Name: t.Title, List: t.Meta("list"), Layout: t.Meta("layout"), Group: t.Meta("group"),
		Order: split(t.Meta("order")), Sort: split(t.Meta("sort")), Filter: split(t.Meta("filter")), Show: split(t.Meta("show")), By: t.By,
		Suggested: t.Meta("suggested") != "", Why: t.Meta("why")}
	if v.List == "" {
		v.List = Backlog.Name
	}
	if !slices.Contains(Layouts, v.Layout) {
		v.Layout = "list"
	}
	return v
}

func (s *Store) SavedViews() []BoardView {
	f, err := s.Load(Views)
	if err != nil {
		return nil
	}
	var out []BoardView
	for _, t := range f.Tasks {
		if t.Key != "" {
			out = append(out, viewOf(t))
		}
	}
	return out
}

func checkViewField(name string) error {
	name = strings.TrimPrefix(name, "-")
	if slices.Contains(BuiltinFields, name) || name == "title" {
		return nil
	}
	if err := board.CheckField(name); err != nil && !strings.Contains(err.Error(), "set by Callboard") {
		return err
	}
	return nil
}

func (s *Store) checkViewFields(in ViewSpec) error {
	var names []string
	if in.Group != nil {
		names = append(names, *in.Group)
	}
	for _, l := range []*[]string{in.Sort, in.Show} {
		if l != nil {
			names = append(names, *l...)
		}
	}
	if in.Filter != nil {
		for _, fl := range *in.Filter {
			if f, _, _, err := ParseFilter(fl); err == nil {
				names = append(names, f)
			}
		}
	}
	files, err := s.LoadAll()
	if err != nil {
		return err
	}
	inUse := map[string]bool{"status": true, "title": true}

	for _, v := range s.SavedViews() {
		if in.Key != "" && strings.EqualFold(v.Key, in.Key) {
			for _, f := range append(append(append([]string{v.Group}, v.Sort...), v.Show...), v.Filter...) {
				f, _, _ = strings.Cut(strings.TrimPrefix(f, "-"), "=")
				inUse[strings.TrimSuffix(f, "!")] = true
			}
		}
	}
	for _, k := range s.Lists() {
		for f := range FieldsInUse(files[k.Name]) {
			inUse[f] = true
		}
	}
	for _, n := range names {
		n = strings.TrimPrefix(strings.TrimSpace(n), "-")
		if n == "" || inUse[n] || slices.Contains(BuiltinFields, n) {
			continue
		}
		var have []string
		for f := range inUse {
			if f != "status" && f != "title" {
				have = append(have, f)
			}
		}
		sort.Strings(have)
		msg := fmt.Sprintf("no item has a %q field", n)
		if m := closest(n, have); m != "" {
			msg += fmt.Sprintf("; did you mean %q?", m)
		}
		if len(have) > 0 {
			msg += "; fields in use: " + strings.Join(have, ", ")
		}
		return fmt.Errorf("%s; built in: status, section, done, blocked, claimed, by, at, doneat, needs. To use a new field, set it on the items first (the edit tool, or callboard set), then make the view", msg)
	}
	return nil
}

func (s *Store) SaveView(in ViewSpec, a Actor) (BoardView, error) {
	var fields [][2]string
	set := func(k, v string) { fields = append(fields, [2]string{k, v}) }
	if err := s.checkViewFields(in); err != nil {
		return BoardView{}, err
	}
	if in.List != nil {
		k, err := s.KindNamed(*in.List)
		if err != nil {
			return BoardView{}, err
		}
		set("list", k.Name)
	}
	if in.Layout != nil {
		if !slices.Contains(Layouts, *in.Layout) {
			return BoardView{}, fmt.Errorf("layout is list, board or table, not %q", *in.Layout)
		}
		set("layout", *in.Layout)
	}
	if in.Group != nil {
		if *in.Group != "" {
			if err := checkViewField(*in.Group); err != nil {
				return BoardView{}, err
			}
		}
		set("group", *in.Group)
	}
	for _, x := range []struct {
		name string
		v    *[]string
		keys bool
	}{{"order", in.Order, false}, {"sort", in.Sort, true}, {"filter", in.Filter, false}, {"show", in.Show, true}} {
		if x.v == nil {
			continue
		}
		var vals []string
		for _, v := range *x.v {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if strings.Contains(v, ",") {
				return BoardView{}, fmt.Errorf("%s values can't contain commas: %q", x.name, v)
			}
			if x.keys {
				if err := checkViewField(v); err != nil {
					return BoardView{}, err
				}
			}
			if x.name == "filter" {
				if _, _, _, err := ParseFilter(v); err != nil {
					return BoardView{}, err
				}
			}
			vals = append(vals, v)
		}
		set(x.name, strings.Join(vals, ","))
	}
	if in.Suggest != nil {
		if strings.TrimSpace(*in.Suggest) == "" {
			return BoardView{}, fmt.Errorf("say why the view helps, e.g. --suggest \"see what's urgent first\"")
		}
		set("suggested", "yes")
		set("why", strings.TrimSpace(*in.Suggest))
	}
	if in.Keep {
		set("suggested", "")
		set("why", "")
	}
	var t *board.Task
	detail := ""
	_, err := s.UpdateBy(Views, a, func(f *board.File) (err error) {
		if in.Key == "" {
			detail = "added"
			if in.Suggest != nil {
				detail = "suggested"
			}
			name := ""
			if in.Name != nil {
				name = *in.Name
			}
			if len(f.Lines) == 0 {
				nf := board.ParseAs(viewsHead, Views.Prefix)
				*f = *nf
			}
			if t, err = f.AddNew(board.New{Title: name, By: a.By}); err != nil {
				return err
			}
			in.Key = t.Key
			if in.Layout == nil {
				fields = append(fields, [2]string{"layout", "list"})
			}
		} else if in.Name != nil {
			if t, err = f.Rename(in.Key, "", *in.Name); err != nil {
				return err
			}
		}
		for i := range fields {
			v, err := board.CleanValue(fields[i][1])
			if err != nil {
				return err
			}
			fields[i][1] = v
		}
		t, err = f.Update(in.Key, "", func(t *board.Task) error {
			for _, kv := range fields {
				t.SetMeta(kv[0], kv[1])
			}
			return nil
		})
		return err
	})
	if err != nil {
		return BoardView{}, err
	}
	s.Log(Event{By: a.By, What: "view", Key: t.Key, Title: t.Title, Detail: detail, Session: a.Session})
	return viewOf(t), nil
}

func (s *Store) DeleteView(key string, a Actor) error {
	var t *board.Task
	_, err := s.UpdateBy(Views, a, func(f *board.File) (err error) {
		t, err = f.Remove(key, "")
		return err
	})
	if err == nil {
		s.Log(Event{By: a.By, What: "view", Key: t.Key, Title: t.Title, Detail: "removed", Session: a.Session})
	}
	return err
}

func ParseFilter(f string) (field string, not bool, values []string, err error) {
	op := "="
	if strings.Contains(f, "!=") {
		op, not = "!=", true
	}
	k, v, ok := strings.Cut(f, op)
	k = strings.TrimSpace(k)
	if !ok || k == "" {
		return "", false, nil, fmt.Errorf("a filter is field=value, field!=value or field=a|b, not %q", f)
	}
	if err := checkViewField(k); err != nil {
		return "", false, nil, err
	}
	for _, x := range strings.Split(v, "|") {
		values = append(values, strings.TrimSpace(x))
	}
	return k, not, values, nil
}

func (s *Store) SetFields(key string, fields [][2]string, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("which fields? e.g. status=doing prio=high")
	}
	var t *board.Task
	ticked := 0
	_, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		if t, err = f.SetFields(key, version, fields); err != nil || k == Questions {
			return err
		}
		for _, kv := range fields {
			if kv[0] != "status" {
				continue
			}
			if st := stage(kv[1]); st == "done" && !t.Done {
				t, err = f.TickBy(t.Key, "", true, a.By)
				ticked = 1
			} else if st != "" && st != "done" && t.Done {
				t, err = f.TickV(t.Key, "", false)
				ticked = -1
			}
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if ticked == 1 {
		s.Log(Event{By: a.By, What: "ticked", Key: t.Key, Title: t.Title, Session: a.Session})
		s.Release(t.Key, a.By, a.Session, true)
	} else if ticked == -1 {
		s.Log(Event{By: a.By, What: "reopened", Key: t.Key, Title: t.Title, Session: a.Session})
	}
	var d []string
	for _, kv := range fields {
		if kv[1] == "" {
			d = append(d, kv[0]+" cleared")
		} else {
			d = append(d, kv[0]+": "+kv[1])
		}
	}
	s.Log(Event{By: a.By, What: "set", Key: t.Key, Title: t.Title, Detail: strings.Join(d, ", "), Session: a.Session})
	return t, nil
}

func (s *Store) Move(key, section string, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	var t *board.Task
	var from string
	_, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		if o := f.Find(key); o != nil {
			from = o.Section
		}
		t, err = f.Move(key, version, section)
		return err
	})
	if err == nil && !strings.EqualFold(from, t.Section) {
		s.Log(Event{By: a.By, What: "moved", Key: t.Key, Title: t.Title, Detail: t.Section, Session: a.Session})
	}
	return t, err
}

func (s *Store) Place(key, target string, after bool, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	if tk, err := s.kindFor(target); err != nil || tk != k {
		return nil, fmt.Errorf("%s and %s aren't in the same list", key, target)
	}
	var t *board.Task
	var from string
	_, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		if o := f.Find(key); o != nil {
			from = o.Section
		}
		t, err = f.Place(key, version, target, after)
		return err
	})
	if err == nil && !strings.EqualFold(from, t.Section) {
		s.Log(Event{By: a.By, What: "moved", Key: t.Key, Title: t.Title, Detail: t.Section, Session: a.Session})
	}
	return t, err
}

func FieldList(m map[string]string) [][2]string {
	var out [][2]string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out = append(out, [2]string{k, m[k]})
	}
	return out
}

func FieldsInUse(f *board.File) map[string][]string {
	skip := []string{"answer", "why", "answered-by", "was", "reopens"}
	seen := map[string]map[string]bool{}
	for _, t := range f.Tasks {
		for _, kv := range t.Fields() {
			if slices.Contains(skip, kv[0]) || kv[1] == "" {
				continue
			}
			if seen[kv[0]] == nil {
				seen[kv[0]] = map[string]bool{}
			}
			seen[kv[0]][kv[1]] = true
		}
	}
	out := map[string][]string{}
	for k, vs := range seen {
		for v := range vs {
			out[k] = append(out[k], v)
		}
		sort.Strings(out[k])
	}
	return out
}

func FormatFields(v *View) string {
	var parts []string
	for _, k := range v.s.Lists() {
		in := FieldsInUse(v.Files[k.Name])
		names := make([]string, 0, len(in))
		for n := range in {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			vals := in[n]
			if len(vals) > 6 {
				vals = append(vals[:6], "…")
			}
			parts = append(parts, fmt.Sprintf("%s (%s)", n, strings.Join(vals, ", ")))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Fields in use: " + strings.Join(dedupe(parts), "; ") + ". Reuse these words when you set fields.\n"
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

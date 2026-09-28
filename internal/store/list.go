package store

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
)

type Item struct {
	Key       string            `json:"key"`
	Kind      string            `json:"kind"`
	List      string            `json:"list"`
	Title     string            `json:"title"`
	Done      bool              `json:"done"`
	Section   string            `json:"section,omitempty"`
	By        string            `json:"by,omitempty"`
	At        string            `json:"at,omitempty"`
	DoneAt    string            `json:"doneAt,omitempty"`
	DoneBy    string            `json:"doneBy,omitempty"`
	Version   string            `json:"version"`
	Needs     []string          `json:"needs,omitempty"`
	WaitingOn []string          `json:"waitingOn,omitempty"`
	Blocked   bool              `json:"blocked,omitempty"`
	Body      []string          `json:"body,omitempty"`
	Options   []board.Option    `json:"options,omitempty"`
	Assumed   string            `json:"assumed,omitempty"`
	Claim     *Claim            `json:"claim,omitempty"`
	Answer    string            `json:"answer,omitempty"`
	Why       string            `json:"why,omitempty"`
	Was       string            `json:"was,omitempty"`
	Answers   []NeedAnswer      `json:"answers,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	Parent    string            `json:"parent,omitempty"`
	Depth     int               `json:"depth,omitempty"`
	Subtasks  []string          `json:"subtasks,omitempty"`
	SubsOpen  int               `json:"subsOpen,omitempty"`
}

type NeedAnswer struct {
	Key      string `json:"key"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type View struct {
	Files  map[string]*board.File
	Claims map[string]Claim
	s      *Store
}

func (s *Store) View() (*View, error) {
	files, err := s.LoadAll()
	if err != nil {
		return nil, err
	}
	return &View{Files: files, Claims: s.Claims(), s: s}, nil
}

func (v *View) open(key string) bool {
	k, ok := v.s.KindOf(key)
	if !ok {
		return false
	}
	t := v.Files[k.Name].Find(key)
	if t == nil || t.Done {
		return false
	}
	return !(k == Questions && t.Assumed > 0)
}

func (v *View) find(key string) *board.Task {
	k, ok := v.s.KindOf(key)
	if !ok {
		return nil
	}
	return v.Files[k.Name].Find(key)
}

func (v *View) Item(k Kind, t *board.Task) Item {
	it := Item{Key: t.Key, Kind: k.Noun, List: k.Name, Title: t.Title, Done: t.Done, Section: t.Section, By: t.By, At: t.At, DoneAt: t.DoneAt, DoneBy: t.DoneBy, Version: t.Version, Needs: t.Needs}
	for _, n := range t.Needs {
		if v.open(n) {
			it.WaitingOn = append(it.WaitingOn, n)
		}
	}
	it.Parent, it.Depth = t.Parent, t.Depth
	if t.Key != "" {
		for _, x := range v.Files[k.Name].Tasks {
			if strings.EqualFold(x.Parent, t.Key) {
				it.Subtasks = append(it.Subtasks, x.Key)
				if !x.Done {
					it.SubsOpen++
				}
			}
		}
	}
	it.Blocked = (len(it.WaitingOn) > 0 || it.SubsOpen > 0) && !t.Done
	if k == Questions {
		it.Options = t.Options()
		if t.Assumed > 0 && t.Assumed <= len(it.Options) {
			it.Assumed = it.Options[t.Assumed-1].Text
		}
	}
	for _, b := range t.Body {
		it.Body = append(it.Body, board.UnescapeNote(b))
	}
	for _, f := range t.Fields() {
		switch f[0] {
		case "answer":
			it.Answer = f[1]
		case "why":
			it.Why = f[1]
		case "was":
			it.Was = f[1]
		default:
			if it.Fields == nil {
				it.Fields = map[string]string{}
			}
			it.Fields[f[0]] = f[1]
		}
	}
	for _, n := range t.Needs {
		if q := v.find(n); q != nil && q.Done && q.Meta("answer") != "" {
			it.Answers = append(it.Answers, NeedAnswer{Key: q.Key, Question: q.Title, Answer: q.Meta("answer")})
		}
	}
	if c, ok := v.Claims[strings.ToUpper(t.Key)]; ok {
		it.Claim = &c
	}
	return it
}

type List struct {
	Repo    string  `json:"repo"`
	Branch  string  `json:"branch,omitempty"`
	File    string  `json:"file"`
	Open    int     `json:"open"`
	Done    int     `json:"done"`
	Items   []Item  `json:"items"`
	Merges  []Merge `json:"merges,omitempty"`
	Deleted []Gone  `json:"deleted,omitempty"`
	From    int     `json:"from,omitempty"`
	Total   int     `json:"total,omitempty"`
}

const PageSize = 50

func (l List) Page(from, limit int) List {
	items, n := len(l.Items), len(l.Items)+len(l.Deleted)
	from = min(max(from, 0), n)
	end := n
	if limit > 0 {
		end = min(from+limit, n)
	}
	if from == 0 && end == n {
		return l
	}
	l.Items = l.Items[min(from, items):min(end, items)]
	l.Deleted = l.Deleted[max(from-items, 0):max(end-items, 0)]
	l.From, l.Total = from, n
	return l
}

func (s *Store) List(kind, status, section string) (List, error) {
	var kinds []Kind
	if kind == "" || kind == "all" {
		kinds = s.Lists()
	} else {
		k, err := s.KindNamed(kind)
		if err != nil {
			return List{}, err
		}
		kinds = []Kind{k}
	}
	switch status {
	case "", "open", "ready", "done", "all":
	default:
		return List{}, fmt.Errorf("status is open, ready, done or all, not %q", status)
	}
	v, err := s.View()
	if err != nil {
		return List{}, err
	}
	wd, _ := os.Getwd()
	out := List{Repo: s.Name, Branch: s.Branch(), File: s.Rel(wd), Items: []Item{}}
	for _, k := range kinds {
		f := v.Files[k.Name]
		o, d := f.Counts()
		out.Open, out.Done = out.Open+o, out.Done+d
		merges := MergesIn(k, f)
		for _, t := range f.Tasks {
			if section != "" && !strings.EqualFold(section, t.Section) || inMerge(merges, t.Key) {
				continue
			}
			it := v.Item(k, t)
			switch {
			case (status == "" || status == "open") && t.Done,
				status == "ready" && (t.Done || it.Blocked),
				status == "done" && !t.Done:
				continue
			}
			out.Items = append(out.Items, it)
		}
		out.Merges = append(out.Merges, merges...)
	}
	if status == "done" || status == "all" {
		s.fillDoneBy(out.Items)
	}
	return out, nil
}

const Rule = `Keep your work in Callboard, not in your head or in files you edit by hand:
- reading: answer any question about what's in the lists from Callboard, never by reading or grepping backlog.md, requests.md or questions.md: the list tool (find searches; key gives one item's notes, what it waits on and frees, and its history), or callboard list --find WORDS and callboard show B-k3f9.
- tasks: add what you find; a deleted item comes back where it was with edit's restore (callboard restore B-k3f9), and list's find shows deleted ones too; claim one before you start it, tick it when it's done. Split a big one into subtasks (add's parent, or callboard add "…" --under B-k3f9); it can be ticked once they are.
- notes: a plan or finding about an item goes under it (edit's add_note, or callboard note B-k3f9 "text"), not in a new item.
- requests: what only the human can do (accounts, keys, money, deploys), with exact steps.
- questions: the human's decisions, with options and yours marked "(recommended)". If you can't wait, assume one and go on; the human confirms or overturns it.
- needs: what a task waits on (a request, a question or another task).
- fields: facts on an item (prio, area, due), set with edit or callboard set KEY prio=high; reuse the words in use. Callboard keeps status, needs, claims and done itself; don't make fields for them.
- for: a field saying who an item is for: you (meaning the human), claude, codex or a session's name, or several (claude or codex). Leave items for others alone; claim refuses them unless the human asked you.
- lists: when the human asks for a list of its own (bugs, ideas), make it (the lists tool, or callboard lists add bugs) and add to it with kind bugs, not under a heading in the backlog.
- views: when the human asks to see the lists a certain way (a board, most urgent first, by area, what's blocked), make that view (the view tool, or callboard view) and say what it shows. A view that's your own idea, suggest instead.
Tools: list, add, edit, tick, claim, assume, answer, news, resolve, section, lists, view; or the callboard command. When you change an item, pass the version you read (B-k3f9@7c); if someone else changed it since, you get the new one.`

func (s *Store) Prime(a Actor) (string, error) {
	session := a.Session
	v, err := s.View()
	if err != nil {
		return "", err
	}

	news := s.News(session)
	var b strings.Builder
	head := []string{"Callboard", s.Name}
	if br := s.Branch(); br != "" {
		head = append(head, br)
	}
	b.WriteString(strings.Join(head, " · ") + "\n")
	merges := v.Merges()
	if due := v.dueSoon(time.Now()); len(due) > 0 {
		if len(due) > 8 {
			due = append(due[:8], fmt.Sprintf("and %d more", len(due)-8))
		}
		b.WriteString("Due soon or overdue: " + strings.Join(due, "; ") + "\n")
	}
	for _, k := range s.Lists() {
		f := v.Files[k.Name]
		open, done := f.Counts()
		if open+done == 0 {
			if k == Backlog {
				b.WriteString("No tasks yet.\n")
			}
			continue
		}
		fmt.Fprintf(&b, "%s: %d open, %d done\n", k.File, open, done)
		shown, sec := 0, ""
		for _, t := range f.Tasks {
			if t.Done || InMerge(merges, t.Key) {
				continue
			}
			if shown == 25 {
				fmt.Fprintf(&b, "  … and %d more (list tool, or callboard list)\n", open-shown)
				break
			}
			if t.Section != sec && t.Section != "" {
				fmt.Fprintf(&b, "  ## %s\n", t.Section)
			}
			sec = t.Section
			b.WriteString("  " + strings.Repeat("  ", t.Depth) + FormatItem(v.Item(k, t)) + "\n")
			shown++
		}
	}
	if next := v.NextUp(a); next != nil {
		whose := ""
		if Assigned(*next) {
			whose = " It's meant for you."
		}
		fmt.Fprintf(&b, "Next up (ready, nobody on it, not meant for someone else, most important first): %s@%s %s.%s Claim it before you start: callboard claim %s, or the claim tool.\n", next.Key, next.Version, next.Title, whose, next.Key)
	}
	if own := s.Custom(); len(own) > 0 {
		var names []string
		for _, k := range own {
			names = append(names, k.File+" (keys "+k.Prefix+")")
		}
		b.WriteString("Own lists, task lists like the backlog: " + strings.Join(names, ", ") + ". Pass the name as kind to list and add (callboard add \"…\" --kind " + own[0].Name + "); don't read the files either.\n")
	}
	b.WriteString(FormatFields(v))
	if vs := s.SavedViews(); len(vs) > 0 {
		var names []string
		for _, x := range vs {
			n := x.Name + " (" + x.Key + ")"
			if x.Suggested {
				n += ", suggested, not kept yet"
			}
			names = append(names, n)
		}
		b.WriteString("Views on the page: " + strings.Join(names, ", ") + ".\n")
	}
	b.WriteString(FormatNews(news))
	b.WriteString(FormatMerges(v.Merges()))
	b.WriteString(Rule + "\n")
	return b.String(), nil
}

func strconvQuote(s string) string { return `"` + s + `"` }

func (v *View) dueSoon(now time.Time) []string {
	type due struct {
		at   time.Time
		line string
	}
	var ds []due
	for _, k := range v.s.Lists() {
		for _, t := range v.Files[k.Name].Tasks {
			if t.Done || t.Key == "" {
				continue
			}
			it := v.Item(k, t)
			if w := DueWords(it, now); w != "" {
				_, at, _ := DueOf(it)
				ds = append(ds, due{at, it.Key + " " + it.Title + " (" + w + ")"})
			}
		}
	}
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].at.Before(ds[j].at) })
	var out []string
	for _, d := range ds {
		out = append(out, d.line)
	}
	return out
}

package show

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AsWali/CallBoard/internal/store"
)

type Opts struct {
	Color bool
	Width int
	Now   time.Time
	Wrap  bool
	Keys  map[string]string
}

type drawer struct {
	Opts
	st    *store.Store
	items []store.Item
	byKey map[string]store.Item
	views []store.BoardView
	down  map[string][]string
	names map[string]string
	b     strings.Builder
}

func (d *drawer) who(c *store.Claim) string {
	if d.names == nil {
		d.names = map[string]string{}
		for _, a := range d.st.Agents(0) {
			if a.Name != "" {
				d.names[a.Session] = a.Name
			}
		}
	}
	if n := d.names[c.Session]; n != "" {
		return n
	}
	return c.By
}

type query struct {
	kinds   []string
	status  string
	layout  string
	view    *store.BoardView
	views   bool
	key     string
	filters []string
	sorts   []string
	group   string
	order   []string
	show    []string
	words   []string
	help    bool
	page    string
}

func Render(st *store.Store, args []string, o Opts) string {
	out := render(st, args, o)
	if !st.Off() {
		return out
	}
	d := &drawer{Opts: o}
	return strings.TrimRight(out, "\n") + "\n\n" + d.c(faint, "Callboard is switched off in this repo, so the lists can be read but not changed. Turn it back on with: callboard on") + "\n"
}

func render(st *store.Store, args []string, o Opts) string {
	if o.Width <= 0 {
		o.Width = 100
	}
	d := &drawer{Opts: o, st: st, byKey: map[string]store.Item{}, views: st.SavedViews()}
	v, err := st.View()
	if err != nil {
		return err.Error()
	}
	merges := v.Merges()
	for _, k := range st.Lists() {
		for _, t := range v.Files[k.Name].Tasks {
			if store.InMerge(merges, t.Key) {
				continue
			}
			it := v.Item(k, t)
			d.items = append(d.items, it)
			if it.Key != "" {
				d.byKey[strings.ToUpper(it.Key)] = it
			}
		}
	}
	q := d.parse(args)
	switch {
	case q.help:
		d.help()
	case q.key != "":
		d.detail(q.key)
	case q.views:
		d.viewList()
	case q.page == "you":
		d.waitingOnYou()
	case q.page == "now":
		d.rightNow()
	case q.page == "quiet":
		d.goneQuiet()
	case q.page == "recent":
		d.recentlyDone(7)
	case len(args) == 0:
		d.overview()
	case q.layout == "graph":
		d.graph(q)
	default:
		d.listing(q)
	}
	return d.finish()
}

func Split(s string) []string {
	var out []string
	var cur strings.Builder
	quote, in := rune(0), false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote, in = r, true
		case quote == 0 && (r == ' ' || r == '\t' || r == '\n'):
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

var keyRe = regexp.MustCompile(`(?i)^[A-Z]-[a-z0-9]{2,}(@[a-z0-9]+)?$`)

var layoutWords = map[string]string{
	"list": "list", "board": "board", "table": "table",
	"graph": "graph", "map": "graph", "deps": "graph", "dependencies": "graph", "tree": "graph",
}

func (d *drawer) parse(args []string) query {
	var q query
	if len(args) > 0 {
		if v := d.findView(strings.Join(args, " ")); v != nil {
			q.view = v
			return q
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		w := strings.ToLower(a)

		if name, ok := cutAny(w, "view:", "layout:", "v:"); ok || w == "view" {
			if !ok {
				if i+1 >= len(args) {
					q.views = true
					continue
				}
				i++
				name = strings.ToLower(args[i])
			}
			if l, ok := layoutWords[name]; ok {
				q.layout = l
				continue
			}
			rest := strings.TrimPrefix(strings.Join(args[i:], " "), a[:len(a)-len(name)])
			if !ok {
				rest = strings.Join(args[i:], " ")
			}
			found := false
			for j := len(args); j > i; j-- {
				n := strings.Join(args[i:j], " ")
				if ok {
					n = strings.TrimSpace(n[len(a)-len(name):])
				}
				if v := d.findView(n); v != nil {
					q.view, i, found = v, j-1, true
					break
				}
			}
			if !found {
				q.words = append(q.words, "view:"+rest)
				i = len(args)
			}
			continue
		}
		if f, ok := cutAny(w, "sort:"); ok {
			q.sorts = append(q.sorts, f)
			continue
		}
		if f, ok := cutAny(w, "group:", "by:"); ok {
			q.group = f
			continue
		}
		if f, ok := cutAny(w, "show:"); ok {
			q.show = append(q.show, strings.Split(f, ",")...)
			continue
		}
		if strings.Contains(a, "=") {
			q.filters = append(q.filters, a)
			continue
		}
		if _, known := d.st.KindOf(a); known && keyRe.MatchString(a) {
			q.key = a
			continue
		}
		if l, ok := layoutWords[w]; ok {
			q.layout = l
			continue
		}
		switch w {
		case "help", "?", "-h", "--help":
			q.help = true
		case "you", "foryou", "mine", "waiting-on-you":
			q.page = "you"
		case "now", "live":
			q.page = "now"
		case "quiet", "stale", "idle":
			q.page = "quiet"
		case "recent", "history":
			q.page = "recent"
		case "views":
			q.views = true
		case "tasks", "task", "backlog":
			q.kinds = append(q.kinds, "backlog")
		case "todo", "todos":
			q.kinds = append(q.kinds, "backlog")
			if q.status == "" {
				q.status = "open"
			}
		case "requests", "request":
			q.kinds = append(q.kinds, "requests")
		case "questions", "question":
			q.kinds = append(q.kinds, "questions")
		case "open":
			q.status = "open"
		case "ready", "next":
			q.status = "ready"
		case "waiting", "blocked":
			q.status = "waiting"
		case "claimed", "doing", "working", "active":
			q.status = "claimed"
		case "done", "answered", "finished":
			q.status = "done"
		case "all", "everything":
			q.status = "all"
		default:
			if k, err := d.st.KindNamed(w); err == nil && k.Own() {
				q.kinds = append(q.kinds, k.Name)
			} else {
				q.words = append(q.words, w)
			}
		}
	}
	return q
}

func cutAny(s string, prefixes ...string) (string, bool) {
	for _, p := range prefixes {
		if rest, ok := strings.CutPrefix(s, p); ok {
			return rest, true
		}
	}
	return "", false
}

func (d *drawer) findView(name string) *store.BoardView {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for i, v := range d.views {
		if strings.EqualFold(v.Name, name) || strings.EqualFold(v.Key, name) {
			return &d.views[i]
		}
	}
	return nil
}

const (
	bold    = "1"
	faint   = "2"
	green   = "32"
	yellow  = "33"
	blue    = "34"
	magenta = "35"
	cyan    = "36"
	orange  = "38;5;208"
	red     = "31"
)

func (d *drawer) c(code, s string) string {
	if !d.Color || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (d *drawer) line(parts ...string) {
	s := strings.Join(parts, "")
	if d.Wrap && d.Width > 0 && vlen(s) > d.Width {
		s = strings.Join(fold(s, d.Width), "\n")
	}
	d.b.WriteString(s + "\n")
}

func fold(s string, w int) []string {
	rest := strings.TrimLeft(s, " ")
	lead := s[:len(s)-len(rest)]
	pad := lead + "  "
	var out []string
	cur, col, start, color := lead, len(lead), true, ""
	for _, word := range strings.Split(rest, " ") {
		n := vlen(word)
		if !start && col+1+n > w {
			out = append(out, cur)
			cur, col, start = pad+color, len(pad), true
		}
		if !start {
			cur, col = cur+" ", col+1
		}
		cur, col, start = cur+word, col+n, false
		for _, c := range ansiRe.FindAllString(word, -1) {
			if color = c; c == "\x1b[0m" {
				color = ""
			}
		}
	}
	return append(out, cur)
}

func (d *drawer) finish() string {
	out := strings.TrimRight(d.b.String(), "\n")
	if !d.Color {
		return out + "\n"
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = "\x1b[0m" + l
	}
	return strings.Join(lines, "\n") + "\n"
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func vlen(s string) int { return utf8.RuneCountInString(ansiRe.ReplaceAllString(s, "")) }

func Width(s string) int { return vlen(s) }

func trunc(s string, n int) string {
	if n < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

func pad(s string, n int) string {
	if k := vlen(s); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}

func wrap(s string, w int) []string {
	if w < 8 {
		w = 8
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(s) {
		for utf8.RuneCountInString(word) > w {
			r := []rune(word)
			if cur != "" {
				out, cur = append(out, cur), ""
			}
			out, word = append(out, string(r[:w])), string(r[w:])
		}
		switch {
		case cur == "":
			cur = word
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(word) <= w:
			cur += " " + word
		default:
			out, cur = append(out, cur), word
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func kindColor(kind string) string {
	switch kind {
	case "request":
		return yellow
	case "question":
		return magenta
	}
	return cyan
}

var plural = map[string]string{"task": "Tasks", "request": "Requests", "question": "Questions"}

func listTitle(it store.Item) string {
	if it.List == "" || it.List == store.Backlog.Name || it.Kind != "task" {
		return plural[it.Kind]
	}
	return strings.ToUpper(it.List[:1]) + strings.ReplaceAll(it.List[1:], "-", " ")
}

func (d *drawer) glyph(it store.Item) string {
	switch {
	case it.Done:
		return d.c(green, "✓")
	case it.Claim != nil:
		return d.c(blue, "◐")
	case it.Blocked:
		return d.c(orange, "⊘")
	}
	return "○"
}

func (d *drawer) key(it store.Item) string {
	k := it.Key
	if k == "" {
		k = "(new)"
	}
	return d.c(kindColor(it.Kind), pad(k, 6))
}

func (d *drawer) legend() string {
	return d.c(faint, "○ ready  ") + d.c(blue, "◐") + d.c(faint, " being worked on  ") + d.c(orange, "⊘") + d.c(faint, " waiting  ") +
		d.c(green, "✓") + d.c(faint, " done   ") + d.c(cyan, "tasks") + " " + d.c(yellow, "requests") + " " + d.c(magenta, "questions")
}

func fieldText(it store.Item) string {
	var names []string
	for n, v := range it.Fields {
		if v != "" && n != "answered-by" && n != "reopens" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var parts []string
	for _, n := range names {
		parts = append(parts, n+":"+it.Fields[n])
	}
	return strings.Join(parts, " ")
}

func (d *drawer) tail(it store.Item, waits bool) string {
	var parts []string
	if due := store.DueWords(it, time.Now()); due != "" {
		parts = append(parts, due)
	}
	if f := fieldText(it); f != "" {
		parts = append(parts, f)
	}
	switch {
	case it.Done && it.Answer != "":
		parts = append(parts, "→ "+it.Answer)
	case it.Done && it.DoneAt != "":
		parts = append(parts, "done "+prefix(it.DoneAt, 10))
	case it.Claim != nil:
		parts = append(parts, d.who(it.Claim)+" is on it")
	}
	if n := len(it.Subtasks); n > 0 && !it.Done {
		parts = append(parts, fmt.Sprintf("%d of %d subtasks done", n-it.SubsOpen, n))
	}
	if len(it.WaitingOn) > 0 && !it.Done && waits {
		parts = append(parts, "waits on "+strings.Join(it.WaitingOn, ", "))
	}
	if it.Assumed != "" && !it.Done {
		parts = append(parts, "assumed: "+it.Assumed)
	}
	return strings.Join(parts, " · ")
}

func prefix(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (d *drawer) row(indent string, it store.Item, extra string) {
	d.treeRow(indent, indent, it, extra, true)
}

func (d *drawer) treeRow(indent, cont string, it store.Item, extra string, waits bool) {
	head := indent + d.glyph(it) + " " + d.key(it) + " "
	t := d.tail(it, waits)
	if extra != "" {
		t = strings.TrimPrefix(t+" · "+extra, " · ")
	}
	title := it.Title
	if it.Done {
		title = d.c(faint, title)
	}
	room := d.Width - vlen(head)
	if t == "" && (!d.Wrap || utf8.RuneCountInString(it.Title) <= room) {
		d.line(head, d.fit(title, it.Done, room))
		return
	}
	if utf8.RuneCountInString(it.Title)+2+utf8.RuneCountInString(t) <= room {
		d.line(head, title, "  ", d.tailc(it, t))
		return
	}
	pad := cont + strings.Repeat(" ", max(vlen(head)-vlen(cont), 0))
	if !d.Wrap {
		d.line(head, d.fit(title, it.Done, room))
		d.line(pad, d.tailc(it, trunc(t, room)))
		return
	}
	lines := wrap(it.Title, room)
	for i, l := range lines {
		if it.Done {
			l = d.c(faint, l)
		}
		lead := pad
		if i == 0 {
			lead = head
		}
		if i == len(lines)-1 && t != "" && utf8.RuneCountInString(lines[i])+2+utf8.RuneCountInString(t) <= room {
			d.line(lead, l, "  ", d.tailc(it, t))
			return
		}
		d.line(lead, l)
	}
	for _, l := range wrap(t, room) {
		d.line(pad, d.tailc(it, l))
	}
}

func (d *drawer) tailc(it store.Item, t string) string {
	due := store.DueWords(it, time.Now())
	if due == "" || !strings.HasPrefix(t, due) {
		return d.c(faint, t)
	}
	col := orange
	if strings.HasPrefix(due, "overdue") {
		col = red
	}
	return d.c(col, due) + d.c(faint, t[len(due):])
}

func (d *drawer) clip(s string, n int) string {
	if d.Wrap {
		return s
	}
	return trunc(s, n)
}

func (d *drawer) long(lead, s string) {
	room := d.Width - len(lead)
	if !d.Wrap {
		d.line(lead, d.c(faint, trunc(s, room)))
		return
	}
	for _, l := range wrap(s, room) {
		d.line(lead, d.c(faint, l))
	}
}

func (d *drawer) fit(title string, done bool, room int) string {
	plain := ansiRe.ReplaceAllString(title, "")
	if utf8.RuneCountInString(plain) <= room {
		return title
	}
	if done {
		return d.c(faint, trunc(plain, room))
	}
	return trunc(plain, room)
}

func (q query) status0() string {
	if q.status != "" {
		return q.status
	}
	if q.view != nil {
		return "all"
	}
	return "open"
}

func statusOK(it store.Item, status string) bool {
	switch status {
	case "open":
		return !it.Done
	case "ready":
		return !it.Done && !it.Blocked
	case "waiting":
		return !it.Done && it.Blocked
	case "claimed":
		return !it.Done && it.Claim != nil
	case "done":
		return it.Done
	}
	return true
}

func (d *drawer) matches(it store.Item, q query) bool {
	if len(q.kinds) > 0 && !slices.Contains(q.kinds, it.List) {
		return false
	}
	if !statusOK(it, q.status0()) {
		return false
	}
	for _, fl := range q.filters {
		f, not, vals, err := store.ParseFilter(fl)
		if err != nil {
			continue
		}
		f = strings.ToLower(f)
		var have []string
		if f == "needs" {
			have = it.WaitingOn
		} else {
			have = []string{store.NormVal(f, store.ItemVal(it, f))}
		}
		hit := slices.ContainsFunc(vals, func(x string) bool {
			return slices.ContainsFunc(have, func(h string) bool { return strings.EqualFold(store.NormVal(f, x), h) })
		})
		if hit == not {
			return false
		}
	}
	if len(q.words) > 0 {
		text := strings.ToLower(it.Key + " " + it.Title + " " + strings.Join(it.Body, " ") + " " + fieldText(it))
		for _, w := range q.words {
			if !strings.Contains(text, w) {
				return false
			}
		}
	}
	return true
}

func cmpVals(a, b string) int { return store.CompareValues(a, b) }

func sortItems(its []store.Item, keys []string) {
	if len(keys) == 0 {
		return
	}
	sort.SliceStable(its, func(i, j int) bool {
		for _, k := range keys {
			desc := strings.HasPrefix(k, "-")
			f := strings.TrimPrefix(k, "-")
			a, b := store.NormVal(f, store.ItemVal(its[i], f)), store.NormVal(f, store.ItemVal(its[j], f))
			if a == b {
				continue
			}
			if a == "" || b == "" {
				return b == ""
			}
			c := cmpVals(a, b)
			if c == 0 {
				continue
			}
			return c < 0 != desc
		}
		return false
	})
}

type group struct {
	name  string
	items []store.Item
}

func groupBy(its []store.Item, f string, order []string) []group {
	idx := map[string]int{}
	var gs []group
	add := func(val string) int {
		k := strings.ToLower(val)
		if i, ok := idx[k]; ok {
			return i
		}
		idx[k] = len(gs)
		gs = append(gs, group{name: val})
		return len(gs) - 1
	}
	for _, it := range its {
		i := add(store.NormVal(f, store.ItemVal(it, f)))
		gs[i].items = append(gs[i].items, it)
	}
	pos := func(g group) int {
		for i, o := range order {
			if strings.EqualFold(o, g.name) {
				return i
			}
		}
		return len(order)
	}
	sort.SliceStable(gs, func(i, j int) bool {
		a, b := gs[i], gs[j]
		if pa, pb := pos(a), pos(b); pa != pb {
			return pa < pb
		}
		if (a.name == "") != (b.name == "") {
			return b.name == ""
		}
		if pos(a) < len(order) {
			return false
		}
		ra, x := store.PriorityWord(a.name)
		rb, y := store.PriorityWord(b.name)
		if x && y {
			return ra > rb
		}
		return cmpVals(a.name, b.name) < 0
	})
	return gs
}

var groupWords = map[string]map[string]string{
	"done":    {"open": "Open", "done": "Done"},
	"blocked": {"ready": "Ready", "waiting": "Waiting on something"},
}

func groupName(f, v string) string {
	if w, ok := groupWords[f][v]; ok {
		return w
	}
	if v == "" {
		switch f {
		case "claimed":
			return "Nobody on it"
		case "section":
			return "No heading"
		}
		return "No " + f
	}
	if f == "kind" {
		return plural[v]
	}
	return v
}

func defaultOrder(f string) []string {
	switch f {
	case "done":
		return []string{"open", "done"}
	case "blocked":
		return []string{"ready", "waiting"}
	case "kind":
		return []string{"task", "request", "question"}
	}
	return nil
}

func (d *drawer) listing(q query) {
	if q.view != nil {
		v := q.view
		if k, err := d.st.KindNamed(v.List); err == nil {
			q.kinds = []string{k.Name}
		}
		if q.layout == "" {
			q.layout = v.Layout
		}
		if q.group == "" {
			q.group, q.order = v.Group, v.Order
		}
		if len(q.sorts) == 0 {
			q.sorts = v.Sort
		}
		q.filters = append(append([]string{}, v.Filter...), q.filters...)
		if len(q.show) == 0 {
			q.show = v.Show
		}
		d.line(d.c(bold, v.Name), d.c(faint, " · "+v.Layout+" of "+v.List+" · "+v.Key))
	}
	var its []store.Item
	for _, it := range d.items {
		if d.matches(it, q) {
			its = append(its, it)
		}
	}
	if len(its) == 0 {
		d.line("Nothing here matches. ", d.c(faint, "Try "+d.see("all")+", or "+d.see("help")+"."))
		return
	}
	done := q.status0() == "done"
	if len(q.sorts) == 0 && done {
		q.sorts = []string{"-doneat"}
	}
	sortItems(its, q.sorts)
	if q.layout == "" {
		q.layout = "list"
	}
	if q.group == "" && done && q.layout == "list" {
		q.group = "doneat"
	}
	if q.group == "" && q.layout == "board" {
		if len(kindsIn(its)) > 1 {
			q.group = "kind"
		} else {
			q.group = "section"
		}
	}
	var gs []group
	if q.group != "" {
		order := append(append([]string{}, q.order...), defaultOrder(q.group)...)
		if q.group == "section" {
			order = append(order, sectionsIn(its)...)
		}
		gs = groupBy(its, q.group, order)
		if q.group == "doneat" {
			sort.SliceStable(gs, func(i, j int) bool {
				return gs[i].name > gs[j].name && gs[j].name != "" || gs[j].name == "" && gs[i].name != ""
			})
		}
	}
	switch q.layout {
	case "board":
		d.board(q.group, gs)
	case "table":
		d.table(q, its, gs)
	default:
		if gs != nil {
			for _, g := range gs {
				name := groupName(q.group, g.name)
				if q.group == "doneat" && g.name == "" {
					name = "Done, no date"
				}
				d.line(d.c(bold, name), d.c(faint, fmt.Sprintf(" · %d", len(g.items))))
				for _, it := range g.items {
					d.row("  ", it, "")
				}
				d.line()
			}
		} else {
			d.byFile(its)
		}
	}
	d.line(d.legend())
}

func kindsIn(its []store.Item) []string {
	var ks []string
	for _, it := range its {
		if !slices.Contains(ks, it.Kind) {
			ks = append(ks, it.Kind)
		}
	}
	return ks
}

func sectionsIn(its []store.Item) []string {
	var ss []string
	for _, it := range its {
		if !slices.Contains(ss, it.Section) {
			ss = append(ss, it.Section)
		}
	}
	return ss
}

func (d *drawer) byFile(its []store.Item) {
	kind, sec := "", "\x00"
	shown := map[string]bool{}
	for _, it := range its {
		if it.List != kind {
			if kind != "" {
				d.line()
			}
			open, done := 0, 0
			for _, x := range its {
				if x.List == it.List {
					if x.Done {
						done++
					} else {
						open++
					}
				}
			}
			counts := fmt.Sprintf("%d open", open)
			if done > 0 {
				counts += fmt.Sprintf(", %d done", done)
			}
			d.line(d.c(bold+";"+kindColor(it.Kind), listTitle(it)), d.c(faint, " · "+it.List+".md · "+counts))
			kind, sec = it.List, "\x00"
		}
		if it.Section != sec && it.Section != "" {
			d.line("  ", d.c(bold, it.Section))
		}
		sec = it.Section
		shown[strings.ToUpper(it.Key)] = true
		ind := "    "
		if it.Parent != "" && shown[strings.ToUpper(it.Parent)] {
			ind += strings.Repeat("  ", it.Depth)
		}
		d.row(ind, it, "")
	}
	d.line()
}

func (d *drawer) board(f string, gs []group) {
	if len(gs) == 0 {
		return
	}
	const gap = 3
	n := len(gs)
	cw := (d.Width - gap*(n-1)) / n
	if cw < 24 {
		for _, g := range gs {
			d.line(d.c(bold, groupName(f, g.name)), d.c(faint, fmt.Sprintf(" · %d", len(g.items))))
			for _, it := range g.items {
				d.row("  ", it, "")
			}
			d.line()
		}
		return
	}
	if cw > 44 {
		cw = 44
	}
	cols := make([][]string, n)
	for i, g := range gs {
		col := []string{d.c(bold, trunc(groupName(f, g.name), cw-4)) + d.c(faint, fmt.Sprintf(" %d", len(g.items))), d.c(faint, strings.Repeat("─", cw))}
		for _, it := range g.items {
			head := d.glyph(it) + " " + d.c(kindColor(it.Kind), it.Key)
			if ft := fieldText(it); ft != "" {
				head += " " + d.c(faint, trunc(ft, cw-9))
			}
			col = append(col, head)
			lines := wrap(it.Title, cw-2)
			if len(lines) > 3 {
				lines = append(lines[:2], trunc(strings.Join(lines[2:], " "), cw-2))
			}
			for _, l := range lines {
				if it.Done {
					l = d.c(faint, l)
				}
				col = append(col, "  "+l)
			}
			if len(it.WaitingOn) > 0 && !it.Done {
				col = append(col, "  "+d.c(orange, trunc("waits on "+strings.Join(it.WaitingOn, ", "), cw-2)))
			}
			col = append(col, "")
		}
		cols[i] = col
	}
	rows := 0
	for _, c := range cols {
		rows = max(rows, len(c))
	}
	for r := 0; r < rows; r++ {
		var parts []string
		for i, c := range cols {
			cell := ""
			if r < len(c) {
				cell = c[r]
			}
			if i < n-1 {
				cell = pad(cell, cw) + strings.Repeat(" ", gap)
			}
			parts = append(parts, cell)
		}
		d.line(strings.TrimRight(strings.Join(parts, ""), " "))
	}
}

func (d *drawer) table(q query, its []store.Item, gs []group) {
	fields := q.show
	if len(fields) == 0 {
		seen := map[string]bool{}
		for _, it := range its {
			for f, v := range it.Fields {
				if v != "" && f != "answered-by" && f != "reopens" && !seen[f] {
					seen[f] = true
					fields = append(fields, f)
				}
			}
		}
		sort.Strings(fields)
		if len(fields) > 4 {
			fields = fields[:4]
		}
	}
	waits := slices.ContainsFunc(its, func(it store.Item) bool { return it.Blocked })
	width := func(f string) int {
		w := utf8.RuneCountInString(f)
		for _, it := range its {
			w = max(w, utf8.RuneCountInString(store.ItemVal(it, f)))
		}
		return min(w, 18)
	}
	ws := make([]int, len(fields))
	used := 2 + 6 + 2
	for i, f := range fields {
		ws[i] = width(f)
		used += ws[i] + 2
	}
	ww := 0
	if waits {
		ww = 16
		used += ww + 2
	}
	tw := max(d.Width-used, 20)
	head := "  " + pad("", 6) + "  " + pad("Title", tw)
	for i, f := range fields {
		head += "  " + pad(trunc(f, ws[i]), ws[i])
	}
	if waits {
		head += "  Waits on"
	}
	d.line(d.c(faint, strings.TrimRight(head, " ")))
	d.line(d.c(faint, strings.Repeat("─", min(d.Width, vlen(head)))))
	emit := func(it store.Item) {
		title := trunc(it.Title, tw)
		if it.Done {
			title = d.c(faint, title)
		}
		l := d.glyph(it) + " " + d.key(it) + "  " + pad(title, tw)
		for i, f := range fields {
			l += "  " + pad(trunc(store.ItemVal(it, f), ws[i]), ws[i])
		}
		if waits && len(it.WaitingOn) > 0 && !it.Done {
			l += "  " + d.c(orange, trunc(strings.Join(it.WaitingOn, ", "), ww))
		}
		d.line(strings.TrimRight(l, " "))
	}
	if gs == nil {
		for _, it := range its {
			emit(it)
		}
		return
	}
	for _, g := range gs {
		d.line(d.c(bold, groupName(q.group, g.name)), d.c(faint, fmt.Sprintf(" · %d", len(g.items))))
		for _, it := range g.items {
			emit(it)
		}
	}
}

func (d *drawer) graph(q query) {
	status := q.status
	if status != "all" {
		status = "open"
	}
	in := map[string]store.Item{}
	var order []string
	for _, it := range d.items {
		if it.Key != "" && statusOK(it, status) {
			k := strings.ToUpper(it.Key)
			in[k] = it
			order = append(order, k)
		}
	}
	needs := map[string][]string{}
	waiters := map[string][]string{}
	for _, k := range order {
		for _, n := range in[k].Needs {
			n = strings.ToUpper(n)
			if _, ok := in[n]; ok && n != k {
				needs[k] = append(needs[k], n)
				waiters[n] = append(waiters[n], k)
			}
		}
	}
	reach := func(k string) int {
		seen := map[string]bool{}
		var walk func(string)
		walk = func(x string) {
			for _, w := range waiters[x] {
				if !seen[w] {
					seen[w] = true
					walk(w)
				}
			}
		}
		walk(k)
		return len(seen)
	}
	var roots []string
	linked := 0
	for _, k := range order {
		if len(needs[k]) > 0 || len(waiters[k]) > 0 {
			linked++
			if len(needs[k]) == 0 {
				roots = append(roots, k)
			}
		}
	}
	sort.SliceStable(roots, func(i, j int) bool { return reach(roots[i]) > reach(roots[j]) })

	reached := map[string]bool{}
	var mark func(string)
	mark = func(k string) {
		if reached[k] {
			return
		}
		reached[k] = true
		for _, w := range waiters[k] {
			mark(w)
		}
	}
	for _, r := range roots {
		mark(r)
	}
	for _, k := range order {
		if (len(needs[k]) > 0 || len(waiters[k]) > 0) && !reached[k] {
			roots = append(roots, k)
			mark(k)
		}
	}

	narrowed := len(q.kinds) > 0 || len(q.filters) > 0 || len(q.words) > 0
	if narrowed {
		q2 := q
		q2.status = "all"
		var keep []string
		for _, r := range roots {
			tree := map[string]bool{}
			var walk func(string)
			walk = func(x string) {
				if tree[x] {
					return
				}
				tree[x] = true
				for _, w := range waiters[x] {
					walk(w)
				}
			}
			walk(r)
			for x := range tree {
				if d.matches(in[x], q2) {
					keep = append(keep, r)
					break
				}
			}
		}
		roots = keep
	}
	what := "open items"
	if status == "all" {
		what = "items"
	}
	d.line(d.c(bold, "Dependencies"), d.c(faint, fmt.Sprintf(" · %d %s linked, %d with no links (not shown)", linked, what, len(order)-linked)))
	d.line(d.c(faint, "Each item unblocks the ones under it; start at the top."))
	d.line()
	if len(roots) == 0 {
		d.line("Nothing waits on anything yet. ", d.c(faint, "Agents set this with needs, e.g. callboard needs B-k3f9 R-2bq1."))
		return
	}
	printed := map[string]bool{}
	var walk func(k, from, lead, branch string)
	walk = func(k, from, lead, branch string) {
		it := in[k]
		var extra []string
		var others []string
		for _, n := range needs[k] {
			if n != from {
				others = append(others, in[n].Key)
			}
		}
		if from == "" {
			if r := reach(k); r > 0 {
				extra = append(extra, fmt.Sprintf("unblocks %d", r))
			}
		} else if len(others) > 0 {
			extra = append(extra, "also waits on "+strings.Join(others, ", "))
		}
		if printed[k] && len(waiters[k]) > 0 {
			extra = append(extra, "↑ more above")
		}
		next := lead
		switch branch {
		case "├─ ":
			next += "│  "
		case "└─ ":
			next += "   "
		}
		cont := next + "   "
		if !printed[k] && len(waiters[k]) > 0 {
			cont = next + "│  "
		}
		d.treeRow(lead+branch, cont, it, strings.Join(extra, " · "), false)
		if printed[k] {
			return
		}
		printed[k] = true
		ws := waiters[k]
		for i, w := range ws {
			b := "├─ "
			if i == len(ws)-1 {
				b = "└─ "
			}
			walk(w, k, next, b)
		}
	}
	for _, r := range roots {
		walk(r, "", "", "")
		d.line()
	}
	d.line(d.legend())
}

func (d *drawer) detail(key string) {
	key = strings.ToUpper(strings.SplitN(key, "@", 2)[0])
	if strings.HasPrefix(key, "V-") {
		if v := d.findView(key); v != nil {
			d.listing(query{view: v})
			return
		}
	}
	it, ok := d.byKey[key]
	if !ok {
		for _, g := range d.st.Gone() {
			if strings.EqualFold(g.Key, key) {
				day, _, _ := strings.Cut(g.At, "T")
				d.line(g.Key, " ", g.Title, " was deleted by ", g.By, " on ", day, ". ", d.c(faint, "callboard restore "+g.Key+" brings it back where it was."))
				return
			}
		}
		d.line("No item ", key, " here. ", d.c(faint, d.see("all")+" lists every item."))
		return
	}
	where := it.Kind + " · " + it.List + ".md"
	if it.Section != "" {
		where += " › " + it.Section
	}
	d.line(d.glyph(it), " ", d.c(bold+";"+kindColor(it.Kind), it.Key), d.c(faint, "@"+it.Version+"  "+where))
	for _, l := range wrap(it.Title, d.Width) {
		d.line(d.c(bold, l))
	}
	var state []string
	switch {
	case it.Done && it.Kind == "question":
		state = append(state, "Answered")
	case it.Done:
		state = append(state, "Done")
	case it.Claim != nil:
		state = append(state, d.who(it.Claim)+" is working on it, on "+orDetached(it.Claim.Branch))
	case it.Blocked && len(it.WaitingOn) == 0:
		state = append(state, "Waiting on its subtasks")
	case it.Blocked:
		state = append(state, "Waiting")
	case it.Kind == "question":
		state = append(state, "Waiting for your answer")
	case it.Kind == "request":
		state = append(state, "Waiting for you")
	default:
		state = append(state, "Ready")
	}
	if it.DoneAt != "" {
		state = append(state, "on "+prefix(it.DoneAt, 10))
	}
	added := "added"
	if it.By != "" {
		added += " by " + it.By
	}
	if it.At != "" {
		added += " on " + prefix(it.At, 10)
	}
	d.line(strings.Join(state, " "), d.c(faint, " · "+added))
	if f := fieldText(it); f != "" {
		d.line(d.c(faint, "Fields: "), f)
	}
	if x, ok := d.byKey[strings.ToUpper(it.Parent)]; ok && it.Parent != "" {
		d.line()
		d.line(d.c(bold, "Subtask of"))
		d.row("  ", x, "")
	}
	if len(it.Subtasks) > 0 {
		d.line()
		d.line(d.c(bold, "Subtasks"), d.c(faint, fmt.Sprintf(" · %d of %d done", len(it.Subtasks)-it.SubsOpen, len(it.Subtasks))))
		for _, n := range it.Subtasks {
			if x, ok := d.byKey[strings.ToUpper(n)]; ok {
				d.row("  ", x, "")
			}
		}
	}
	if len(it.Needs) > 0 {
		d.line()
		d.line(d.c(bold, "Waits on"))
		for _, n := range it.Needs {
			if x, ok := d.byKey[strings.ToUpper(n)]; ok {
				d.row("  ", x, "")
			} else {
				d.line("  ", d.c(faint, n+" (gone)"))
			}
		}
	}
	var waiters []store.Item
	for _, x := range d.items {
		if slices.ContainsFunc(x.Needs, func(n string) bool { return strings.EqualFold(n, it.Key) }) {
			waiters = append(waiters, x)
		}
	}
	if len(waiters) > 0 {
		d.line()
		d.line(d.c(bold, "Unblocks"))
		for _, x := range waiters {
			d.row("  ", x, "")
		}
	}
	if len(it.Options) > 0 {
		d.line()
		d.line(d.c(bold, "Options"))
		for i, o := range it.Options {
			l := fmt.Sprintf("  %d) %s", i+1, o.Text)
			var notes []string
			if o.Recommended {
				notes = append(notes, "recommended")
			}
			if it.Assumed == o.Text {
				notes = append(notes, "assumed by an agent")
			}
			if len(notes) > 0 {
				l += d.c(faint, " ("+strings.Join(notes, ", ")+")")
			}
			d.line(l)
		}
	}
	if it.Answer != "" {
		d.line()
		a := "Answer: " + it.Answer
		if it.Why != "" {
			a += d.c(faint, " (why: "+it.Why+")")
		}
		d.line(d.c(green, "→ "), a)
	}
	var notes []string
	for _, l := range it.Body {
		if s := strings.TrimSpace(l); s != "" && !(len(it.Options) > 0 && strings.HasPrefix(s, "- ")) {
			notes = append(notes, l)
		}
	}
	if len(notes) > 0 {
		d.line()
		d.line(d.c(bold, "Notes"))
		for _, l := range notes {
			ind := len(l) - len(strings.TrimLeft(l, " "))
			lead := "  " + strings.Repeat(" ", max(ind-2, 0))
			for _, w := range wrap(strings.TrimSpace(l), d.Width-len(lead)) {
				d.line(lead, w)
			}
		}
	}
	if evs := d.st.ItemEvents(it.Key, 8); len(evs) > 0 {
		d.line()
		d.line(d.c(bold, "History"))
		for _, e := range evs {
			d.line("  ", d.c(faint, strings.Replace(prefix(e.At, 16), "T", " ", 1)), "  ", d.clip(e.By+" "+happened(e), min(d.Width-20, 100)))
		}
	}
}

func orDetached(b string) string {
	if b == "" {
		return "a detached worktree"
	}
	return b
}

func happened(e store.Event) string {
	switch e.What {
	case "added":
		return "added it"
	case "ticked":
		return "ticked it"
	case "reopened":
		return "opened it again"
	case "renamed":
		return "renamed it (was \"" + e.Detail + "\")"
	case "needs":
		if e.Detail == "" {
			return "said it waits on nothing"
		}
		return "set it to wait on " + e.Detail
	case "claimed":
		return "started on it"
	case "released":
		return "let it go"
	case "assumed":
		return "assumed an answer: " + e.Detail
	case "answered":
		return "answered: " + e.Detail
	case "set":
		return "set " + e.Detail
	case "noted":
		return "added a note: " + e.Detail
	case "notes":
		if e.Detail == "" {
			return "removed the notes"
		}
		return "changed the notes: " + e.Detail
	case "moved":
		return "moved it to " + e.Detail
	case "removed":
		return "deleted it"
	case "restored":
		return "brought it back"
	}
	return e.What + " " + e.Detail
}

func (d *drawer) overview() {
	head := d.st.Name
	if b := d.st.Branch(); b != "" {
		head += " · " + b
	}
	d.line(d.c(bold, "Callboard"), d.c(faint, " · "+head))
	type counts struct{ open, ready, waiting, claimed, done int }
	cs := map[string]*counts{"task": {}, "request": {}, "question": {}}
	for _, it := range d.items {
		c := cs[it.Kind]
		switch {
		case it.Done:
			c.done++
		default:
			c.open++
			if it.Claim != nil {
				c.claimed++
			}
			if it.Blocked {
				c.waiting++
			} else {
				c.ready++
			}
		}
	}
	t := cs["task"]
	sum := fmt.Sprintf("%d tasks open: %d ready, %d waiting", t.open, t.ready, t.waiting)
	if t.claimed > 0 {
		sum += fmt.Sprintf(", %d being worked on", t.claimed)
	}
	sum += fmt.Sprintf(" · %d requests · %d questions · %d done", cs["request"].open, cs["question"].open, t.done+cs["request"].done+cs["question"].done)
	d.line(sum)
	d.line()

	down := d.downstream()
	section := func(title, hint, more string, its []store.Item, limit int, extra func(store.Item) string) {
		if len(its) == 0 {
			return
		}
		if hint != "" {
			hint = "  " + hint
		}
		d.line(d.c(bold, title), d.c(faint, hint))
		for i, it := range its {
			if i == limit {
				d.line(d.c(faint, fmt.Sprintf("  and %d more%s", len(its)-limit, more)))
				break
			}
			d.row("  ", it, extra(it))
		}
		d.line()
	}
	unblocks := func(it store.Item) string {
		if n := len(down[strings.ToUpper(it.Key)]); n > 0 {
			return fmt.Sprintf("unblocks %d", n)
		}
		return ""
	}
	mine := d.forYou()
	names, groups := d.claimGroups()
	next, _ := d.readyTasks()
	none := func(store.Item) string { return "" }
	section("Waiting on you", "requests and questions, the ones that free most first", ": "+d.see("you"), mine, 5, unblocks)
	if len(names) > 0 {
		d.line(d.c(bold, "Right now"))
		for _, n := range names {
			d.line("  ", d.c(blue, n))
			for _, it := range groups[n] {
				d.row("    ", it, "")
			}
		}
		d.line()
	}
	section("Next up", "ready, nobody on it", ": "+d.see("now"), next, 3, unblocks)
	section("Answered for you, needs your OK", "", ": "+d.see("now"), d.assumedAnswers(), 3, none)
	linked := 0
	for _, it := range d.items {
		if !it.Done && (len(it.WaitingOn) > 0 || len(down[strings.ToUpper(it.Key)]) > 0) {
			linked++
		}
	}
	if linked > 0 {
		d.line(d.c(bold, "Map"), d.c(faint, fmt.Sprintf("  %d open items wait on each other: %s", linked, d.see("graph"))))
		d.line()
	}
	var quiet []store.Item
	for _, s := range d.staleItems() {
		if d.now().Sub(s.at) >= quietAfter {
			quiet = append(quiet, s.it)
		}
	}
	section("Gone quiet", "untouched for a week or more", ": "+d.see("quiet"), quiet, 3, none)
	if days, by, _ := d.doneDays(); len(days) > 0 {
		var per []string
		for _, day := range days[:min(3, len(days))] {
			per = append(per, fmt.Sprintf("%s %d", d.dayName(day), len(by[day])))
		}
		d.line(d.c(bold, "Recently done"), d.c(faint, "  "+strings.Join(per, " · ")))
		for _, it := range by[days[0]][:min(3, len(by[days[0]]))] {
			d.row("  ", it, "")
		}
		d.line()
	}
	if d.Keys == nil {
		d.line(d.c(faint, "More: /callboard you · now · graph · quiet · recent · todo · done · board · B-k3f9 · help"))
	}
}

func (d *drawer) see(words string) string {
	switch k, ok := d.Keys[words]; {
	case ok:
		return "press " + k
	case d.Keys != nil:
		return "callboard show " + words
	}
	return "/callboard " + words
}

func (d *drawer) viewList() {
	if len(d.views) == 0 {
		d.line("No saved views yet. ", d.c(faint, "Ask an agent for one (\"a board by prio\"), or: callboard view --name NAME --layout board --group prio"))
		return
	}
	d.line(d.c(bold, "Saved views"))
	for _, v := range d.views {
		desc := v.Layout + " of " + v.List
		if v.Group != "" {
			desc += ", by " + v.Group
		}
		if len(v.Filter) > 0 {
			desc += ", only " + strings.Join(v.Filter, " and ")
		}
		if v.Suggested {
			desc += ", suggested"
		}
		d.line("  ", pad(d.c(bold, v.Name), 24), " ", d.c(faint, desc+" · "+v.Key))
	}
	d.line(d.c(faint, "Show one: "+d.see("view:NAME")))
}

func (d *drawer) help() {
	rows := [][2]string{
		{"/callboard", "the overview: a little of each view below"},
		{"you", "waiting on you: your requests and questions, the ones that free most first"},
		{"now", "right now: who is on what, what's ready, answers to confirm"},
		{"graph", "the map: what waits on what, across all three lists"},
		{"quiet", "gone quiet: open items nobody touched in a week or more"},
		{"recent", "recently done: what was ticked, day by day"},
		{"todo · tasks · requests · questions", "one list (open items; add done or all)"},
		{"ready · waiting · claimed · done · all", "by state, across the lists"},
		{"board · table · list", "the layout; board groups by heading, or by: a field"},
		{"B-k3f9", "one item: notes, options, what it waits on and unblocks, history"},
		{"views · view:NAME", "the saved views, or one of them"},
		{"panel · panel off", "keep the lists open beside the agent, drawn again as they change"},
		{"prio=high · by!=codex · needs=R-2bq1", "only items with that field (built in: section, by, at, doneat, claimed, kind, blocked)"},
		{"by:prio · sort:-prio · show:prio,due", "group, sort (- is high first) and the fields a table shows"},
		{"any other word", "items whose title or notes have it"},
	}
	d.line(d.c(bold, "/callboard"), d.c(faint, " shows the lists here, without asking the model. Words combine:"))
	d.line()
	for _, r := range rows {
		d.line("  ", pad(d.c(cyan, r[0]), 40), " ", r[1])
	}
	d.line()
	d.line(d.c(faint, "Examples: /callboard todo prio=high · /callboard requests done · /callboard board by:prio · /callboard graph firefox"))
}

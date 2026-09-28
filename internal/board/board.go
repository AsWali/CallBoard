package board

import (
	"crypto/rand"
	"fmt"
	"hash/fnv"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Task struct {
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Done    bool     `json:"done"`
	Section string   `json:"section"`
	By      string   `json:"by,omitempty"`
	At      string   `json:"at,omitempty"`
	DoneAt  string   `json:"doneAt,omitempty"`
	DoneBy  string   `json:"doneBy,omitempty"`
	Needs   []string `json:"needs,omitempty"`
	Assumed int      `json:"assumed,omitempty"`
	Body    []string `json:"body,omitempty"`
	Version string   `json:"version"`
	Line    int      `json:"line"`
	End     int      `json:"end"`
	Raw     string   `json:"raw"`

	Parent string `json:"parent,omitempty"`
	Depth  int    `json:"depth,omitempty"`

	indent string
	extra  [][2]string
	parent *Task
	kids   int
	head   int
	bodyAt []int
}

type File struct {
	Lines    []string
	Tasks    []*Task
	Sections []string
	Prefix   string

	Also    map[string][]string
	Taken   func(key string) bool
	levels  []int
	newline bool
	crlf    bool
}

var (
	taskRe    = regexp.MustCompile(`^(\s*)[-*+] \[( |x|X)\] (.*)$`)
	headingRe = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*\s*$`)
	fenceRe   = regexp.MustCompile("^\\s*(```|~~~)")
	optionRe  = regexp.MustCompile(`^\s+(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?(.+)$`)
	recRe     = regexp.MustCompile(`(?i)\s*[*_]*\(recommended\)[*_]*`)
	keyRe     = regexp.MustCompile(`^[A-Za-z]-[0-9a-z]{3,8}$`)
	boxRe     = regexp.MustCompile(`^(\s*[-*+] )\[( |x|X)\] `)
	escBoxRe  = regexp.MustCompile(`^(\s*[-*+] )\\\[( |x|X)\] `)
)

func Parse(content string) *File { return ParseAs(content, "B-") }

func ParseAs(content, prefix string) *File {
	f := &File{newline: true, Prefix: prefix}
	if content != "" {
		f.crlf = 2*strings.Count(content, "\r\n") > strings.Count(content, "\n")
		content = StripControl(strings.ReplaceAll(content, "\r\n", "\n"))
		f.newline = strings.HasSuffix(content, "\n")
		f.Lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	f.index()
	return f
}

func HasKeys(text string) bool {
	for _, t := range ParseAs(text, "").Tasks {
		if t.Key != "" {
			return true
		}
	}
	return false
}

func Foreign(text string) bool {
	if HasKeys(text) {
		return false
	}
	for _, l := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			return true
		}
	}
	return false
}

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }

func (f *File) Subtasks() bool { return f.Prefix != "Q-" && f.Prefix != "V-" }

func (f *File) index() {
	plain := map[int]bool{}
	for {
		open := f.indexWith(plain)
		if open < 0 || plain[open] {
			return
		}
		plain[open] = true
	}
}

func (f *File) indexWith(plain map[int]bool) int {
	f.Tasks, f.Sections, f.levels = nil, nil, nil
	section, fence, inner, openAt := "", false, false, -1
	isFence := func(i int, l string) bool { return !plain[i] && fenceRe.MatchString(l) }
	var stack []*Task
	for i, l := range f.Lines {
		if len(stack) == 0 && isFence(i, l) {
			if fence = !fence; fence {
				openAt = i
			}
			continue
		}
		if fence {
			continue
		}
		if len(stack) > 0 && strings.TrimSpace(l) != "" && indentOf(l) > len(stack[0].indent) {
			for !inner && len(stack) > 1 && indentOf(l) <= len(stack[len(stack)-1].indent) {
				stack = stack[:len(stack)-1]
			}
			top := stack[len(stack)-1]
			for _, x := range stack {
				x.End = i + 1
			}
			if t := ParseLine(l); t != nil && !inner && f.Subtasks() {
				t.Section, t.Line, t.End, t.Parent, t.Depth = section, i+1, i+1, top.Key, len(stack)
				t.parent = top
				top.kids++
				f.Tasks = append(f.Tasks, t)
				stack = append(stack, t)
				continue
			}
			if fenceRe.MatchString(l) {
				inner = !inner
			}
			if i == top.Line+top.head && top.kids == 0 {
				top.head++
			}
			top.Body = append(top.Body, l)
			top.bodyAt = append(top.bodyAt, i)
			continue
		}
		stack, inner = nil, false
		if isFence(i, l) {
			if fence = !fence; fence {
				openAt = i
			}
			continue
		}
		if m := headingRe.FindStringSubmatch(l); m != nil {
			section = m[1]
			f.Sections = append(f.Sections, section)
			f.levels = append(f.levels, len(l)-len(strings.TrimLeft(l, "#")))
			continue
		}
		if t := ParseLine(l); t != nil {
			t.Section, t.Line, t.End = section, i+1, i+1
			f.Tasks = append(f.Tasks, t)
			stack = []*Task{t}
		}
	}
	for _, t := range f.Tasks {
		t.Version = t.version()
	}
	if fence {
		return openAt
	}
	return -1
}

func unclosedFence(lines []string, from int) int {
	open := -1
	for i := from; i < len(lines); i++ {
		if fenceRe.MatchString(lines[i]) {
			if open < 0 {
				open = i
			} else {
				open = -1
			}
		}
	}
	return open
}

func ParseLine(l string) *Task {
	m := taskRe.FindStringSubmatch(l)
	if m == nil {
		return nil
	}
	t := &Task{indent: m[1], Done: m[2] != " ", Raw: l}
	rest := m[3]
	if title, meta, ok := cutMeta(rest); ok {
		rest = title
		for _, field := range splitMeta(meta) {
			k, v, ok := strings.Cut(field, ":")
			v = unquote(v)
			if !ok {
				t.extra = append(t.extra, [2]string{field, ""})
				continue
			}
			switch k {
			case "id":
				t.Key = v
			case "by":
				t.By = v
			case "at":
				t.At = v
			case "done":
				t.DoneAt = v
			case "done-by":
				t.DoneBy = v
			case "needs":
				for _, n := range strings.Split(v, ",") {
					if n = strings.TrimSpace(n); n != "" {
						t.Needs = append(t.Needs, n)
					}
				}
			case "assumed":
				t.Assumed, _ = strconv.Atoi(v)
			default:
				t.extra = append(t.extra, [2]string{k, v})
			}
		}
	}
	t.Title = strings.TrimSpace(rest)
	return t
}

func cutMeta(rest string) (title, meta string, ok bool) {
	i := strings.LastIndex(rest, "<!--")
	if i < 0 {
		return rest, "", false
	}
	meta = strings.TrimSpace(rest[i+4:])
	switch {
	case strings.HasSuffix(meta, "-->"):
		meta = strings.TrimSpace(strings.TrimSuffix(meta, "-->"))
	case strings.Contains(meta, "-->"):
		return rest, "", false
	}
	return strings.TrimRight(rest[:i], " \t"), meta, true
}

func (t *Task) Format() string {
	box := " "
	if t.Done {
		box = "x"
	}
	var meta []string
	add := func(k, v string) {
		if v != "" {
			meta = append(meta, k+":"+v)
		}
	}
	add("id", t.Key)
	add("by", quote(safe(t.By)))
	add("at", t.At)
	add("done", t.DoneAt)
	add("done-by", quote(safe(t.DoneBy)))
	add("needs", strings.Join(t.Needs, ","))
	if t.Assumed > 0 {
		add("assumed", strconv.Itoa(t.Assumed))
	}
	for _, e := range t.extra {
		if e[1] == "" {
			meta = append(meta, e[0])
		} else {
			meta = append(meta, e[0]+":"+quote(safe(e[1])))
		}
	}
	line := t.indent + "- [" + box + "] " + t.Title
	if len(meta) > 0 {
		line += " <!-- " + strings.Join(meta, " ") + " -->"
	}
	return line
}

func splitMeta(s string) []string {
	var out []string
	var b strings.Builder
	inQ, esc := false, false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case esc:
			b.WriteRune(r)
			esc = false
		case inQ && r == '\\':
			b.WriteRune(r)
			esc = true
		case r == '"':
			b.WriteRune(r)
			inQ = !inQ
		case !inQ && (r == ' ' || r == '\t'):
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func Quote(v string) string { return quote(v) }

func quote(v string) string {
	if !strings.ContainsAny(v, " \t\"\\") {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

func safe(v string) string {
	return strings.NewReplacer("<!--", "<!-", "-->", "->").Replace(v)
}

func unquote(v string) string {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return v
	}
	return strings.NewReplacer(`\\`, `\`, `\"`, `"`).Replace(v[1 : len(v)-1])
}

func CleanValue(v string) (string, error) {
	v = strings.Join(strings.Fields(StripControl(v)), " ")
	if strings.Contains(v, "-->") || strings.Contains(v, "<!--") {
		return "", fmt.Errorf("a value can't contain <!-- or -->")
	}
	return v, nil
}

func (t *Task) Fields() [][2]string { return slices.Clone(t.extra) }

func (t *Task) Meta(k string) string {
	for _, e := range t.extra {
		if e[0] == k {
			return e[1]
		}
	}
	return ""
}

func (t *Task) SetMeta(k, v string) {
	for i, e := range t.extra {
		if e[0] == k {
			if v == "" {
				t.extra = slices.Delete(t.extra, i, i+1)
			} else {
				t.extra[i][1] = v
			}
			return
		}
	}
	if v != "" {
		t.extra = append(t.extra, [2]string{k, v})
	}
}

func (t *Task) version() string {
	h := fnv.New32a()
	h.Write([]byte(t.Raw))
	for _, b := range t.Body {
		h.Write([]byte("\n" + b))
	}
	return strconv.FormatUint(uint64(h.Sum32()%1296), 36)
}

type Option struct {
	Text        string `json:"text"`
	Recommended bool   `json:"recommended,omitempty"`
}

func OptionLine(text string, recommended bool) string {
	text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "- "))
	if recRe.MatchString(text) {
		recommended = true
		text = strings.TrimSpace(recRe.ReplaceAllString(text, ""))
	}
	if recommended {
		text += " (recommended)"
	}
	return "- " + text
}

func (t *Task) Options() []Option {
	var out []Option
	for _, b := range t.Body {
		if m := optionRe.FindStringSubmatch(b); m != nil && t.isOption(b) {
			text := m[1]
			rec := recRe.MatchString(text)
			text = strings.TrimSpace(recRe.ReplaceAllString(text, ""))
			out = append(out, Option{Text: text, Recommended: rec})
		}
	}
	return out
}

func (f *File) String() string {
	nl := "\n"
	if f.crlf {
		nl = "\r\n"
	}
	s := StripControl(strings.Join(f.Lines, "\n"))
	if nl != "\n" {
		s = strings.ReplaceAll(s, "\n", nl)
	}
	if f.newline && len(f.Lines) > 0 {
		s += nl
	}
	return s
}

func (f *File) Find(key string) *Task {
	key = strings.TrimSpace(key)
	if k, _, ok := strings.Cut(key, "@"); ok {
		key = k
	}
	for _, t := range f.Tasks {
		if strings.EqualFold(t.Key, key) {
			return t
		}
	}
	return nil
}

func IsKey(s string) bool { return keyRe.MatchString(s) }

var Now = func() string { return time.Now().Format("2006-01-02T15:04") }

const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func (f *File) NewKey() string {
	used := map[string]bool{}
	for _, t := range f.Tasks {
		used[strings.ToLower(t.Key)] = true
	}
	prefix := f.Prefix
	if prefix == "" {
		prefix = "B-"
	}
	for n, tries := 4, 0; ; tries++ {
		if tries == 20 {
			n = 5
		}
		b := make([]byte, n)
		for i := range b {
			x, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			b[i] = alphabet[x.Int64()]
		}
		k := prefix + string(b)
		if !used[strings.ToLower(k)] && (f.Taken == nil || !f.Taken(k)) {
			return k
		}
	}
}

func (f *File) FillKeys() int {
	n := 0
	for _, t := range f.Tasks {
		if t.Key == "" {
			t.Key = f.NewKey()
			if t.At == "" {
				t.At = Now()
			}
			f.put(t)
			n++
		}
	}
	if n > 0 {
		f.index()
	}
	return n
}

func (f *File) put(t *Task) {
	f.Lines[t.Line-1] = t.Format()
	t.Raw = f.Lines[t.Line-1]
	t.Version = t.version()
}

type New struct {
	Title   string
	Section string
	By      string
	Body    []string
	Needs   []string
	Parent  string
}

func (f *File) Add(title, section, by string) (*Task, error) {
	return f.AddNew(New{Title: title, Section: section, By: by})
}

func (f *File) AddNew(n New) (*Task, error) {
	title, err := cleanTitle(n.Title)
	if err != nil {
		return nil, err
	}
	f.FillKeys()
	var parent *Task
	if n.Parent != "" {
		if !f.Subtasks() {
			return nil, fmt.Errorf("questions can't have subtasks")
		}
		if parent, err = f.Get(n.Parent, ""); err != nil {
			return nil, err
		}
	}
	t := &Task{Key: f.NewKey(), Title: title, By: n.By, At: Now(), Needs: n.Needs}
	block := []string{t.Format()}
	for _, b := range n.Body {
		for _, l := range strings.Split(b, "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if indentOf(l) == 0 {
				l = "  " + l
			}
			block = append(block, f.noteLine(strings.TrimRight(l, " \t")))
		}
	}
	if parent != nil {
		f.underParent(parent, block)
	} else {
		f.place(block, n.Section)
	}
	f.FillKeys()
	return f.Find(t.Key), nil
}

func (f *File) underParent(p *Task, block []string) {
	ind := p.indent + "  "
	for i, l := range block {
		block[i] = ind + l
	}
	f.insert(p.End, block...)
	f.newline = true
	f.index()
}

func (f *File) place(block []string, section string) {
	name := strings.TrimSpace(strings.TrimLeft(section, "#"))
	if name == "" {
		name = f.defaultSection()
	}
	head := f.heading(name)
	switch {
	case head >= 0:

		at := head + 1
		for _, x := range f.Tasks {
			if x.Line-1 > head && strings.EqualFold(x.Section, name) && x.Line-1 < f.nextHeading(head) {
				at = max(at, x.End)
			}
		}
		add := block
		if at == head+1 && at < len(f.Lines) && strings.TrimSpace(f.Lines[at]) == "" {
			at++
		} else if at == head+1 {
			add = append([]string{""}, block...)
		}
		if at < len(f.Lines) && headingRe.MatchString(f.Lines[at]) {
			add = append(slices.Clone(add), "")
		}
		f.insert(at, add...)
	case name != "" && section != "":

		var add []string
		if len(f.Lines) > 0 && strings.TrimSpace(f.Lines[len(f.Lines)-1]) != "" {
			add = append(add, "")
		}
		add = append(add, "## "+name, "")
		f.insert(len(f.Lines), append(add, block...)...)
	default:

		at := len(f.Lines)
		if len(f.Tasks) > 0 {
			at = 0
			for _, x := range f.Tasks {
				at = max(at, x.End)
			}
		}
		f.insert(at, block...)
	}
	f.newline = true
	f.index()
}

func (f *File) Move(key, version, section string) (*Task, error) {
	t, err := f.Get(key, version)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(strings.TrimLeft(section, "#"))
	if name == "" {
		return nil, fmt.Errorf("move %s to which section?", t.Key)
	}
	if strings.EqualFold(name, t.Section) {
		return t, nil
	}
	if _, err := cleanTitle(name); err != nil {
		return nil, err
	}
	f.FillKeys()
	t = f.Find(t.Key)
	block := slices.Clone(f.Lines[t.Line-1 : t.End])
	for i, l := range block {
		block[i] = strings.TrimPrefix(l, t.indent)
	}
	f.Lines = slices.Delete(f.Lines, t.Line-1, t.End)
	f.index()
	f.place(block, name)
	return f.Find(t.Key), nil
}

func (f *File) Place(key, version, target string, after bool) (*Task, error) {
	t, err := f.Get(key, version)
	if err != nil {
		return nil, err
	}
	f.FillKeys()
	t = f.Find(t.Key)
	to := f.Find(strings.SplitN(target, "@", 2)[0])
	if to == nil {
		return nil, f.unknown(target)
	}
	if to.Key == t.Key {
		return t, nil
	}
	block := slices.Clone(f.Lines[t.Line-1 : t.End])
	for i, l := range block {
		block[i] = to.indent + strings.TrimPrefix(l, t.indent)
	}
	f.Lines = slices.Delete(f.Lines, t.Line-1, t.End)
	f.index()
	to = f.Find(to.Key)
	at := to.Line - 1
	if after {
		at = to.End
	}
	f.Lines = slices.Insert(f.Lines, at, block...)
	f.index()
	return f.Find(t.Key), nil
}

var Reserved = []string{"id", "by", "at", "done", "done-by", "needs", "assumed", "answer", "why", "answered-by", "was", "section", "title", "key"}

var fieldRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func CheckField(name string) error {
	if !fieldRe.MatchString(name) {
		return fmt.Errorf("a field name is lowercase letters, digits, - and _, starting with a letter (like status or due-date), not %q", name)
	}
	if slices.Contains(Reserved, name) {
		return fmt.Errorf("%s is set by Callboard itself; use tick, rename, needs, answer or move for it", name)
	}
	return nil
}

func (f *File) SetFields(key, version string, fields [][2]string) (*Task, error) {
	for i, kv := range fields {
		if err := CheckField(kv[0]); err != nil {
			return nil, err
		}
		v, err := CleanValue(kv[1])
		if err != nil {
			return nil, err
		}
		fields[i][1] = v
	}
	return f.change(key, version, func(t *Task) error {
		for _, kv := range fields {
			t.SetMeta(kv[0], kv[1])
		}
		return nil
	})
}

func isControl(r rune) bool {
	return r != '\n' && r != '\t' && (r < 0x20 || r >= 0x7f && r < 0xa0)
}

func StripControl(s string) string {
	if strings.IndexFunc(s, isControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}

func cleanTitle(title string) (string, error) {
	title = strings.Join(strings.Fields(StripControl(title)), " ")
	if title == "" {
		return "", fmt.Errorf("an item needs a title")
	}
	if strings.Contains(title, "-->") || strings.Contains(title, "<!--") {
		return "", fmt.Errorf("a title can't contain <!-- or -->")
	}
	return title, nil
}

func (f *File) heading(name string) int {
	fence, open := false, unclosedFence(f.Lines, 0)
	for i, l := range f.Lines {
		if i != open && fenceRe.MatchString(l) {
			fence = !fence
		}
		if m := headingRe.FindStringSubmatch(l); !fence && m != nil && strings.EqualFold(m[1], name) {
			return i
		}
	}
	return -1
}

func (f *File) nextHeading(head int) int {
	fence, open := false, unclosedFence(f.Lines, head+1)
	for i := head + 1; i < len(f.Lines); i++ {
		if i != open && fenceRe.MatchString(f.Lines[i]) {
			fence = !fence
		}
		if !fence && headingRe.MatchString(f.Lines[i]) {
			return i
		}
	}
	return len(f.Lines)
}

func (f *File) defaultSection() string {
	for _, t := range f.Tasks {
		if !t.Done {
			return t.Section
		}
	}
	if len(f.Tasks) > 0 {
		return f.Tasks[0].Section
	}
	for i, n := range f.Sections {
		if f.levels[i] > 1 {
			return n
		}
	}
	if len(f.Sections) > 0 {
		return f.Sections[0]
	}
	return ""
}

func (f *File) insert(at int, lines ...string) {
	f.Lines = append(f.Lines[:at], append(append([]string{}, lines...), f.Lines[at:]...)...)
}

type ErrChanged struct{ Task *Task }

func (e ErrChanged) Error() string {
	return fmt.Sprintf("%s changed since you read it; it's now %s@%s: %s. Read it again and redo your change if it still applies", e.Task.Key, e.Task.Key, e.Task.Version, e.Task.Title)
}

func (f *File) Get(key, version string) (*Task, error) {
	if k, v, ok := strings.Cut(strings.TrimSpace(key), "@"); ok {
		key, version = k, v
	}
	t := f.Find(key)
	if t == nil {
		return nil, f.unknown(key)
	}
	if _, ok := f.conflictOn(t.Key); ok {
		return nil, ErrConflict{t.Key}
	}
	if version != "" && !strings.EqualFold(version, t.Version) && !slices.ContainsFunc(f.Also[strings.ToUpper(t.Key)], func(v string) bool { return strings.EqualFold(v, version) }) {
		return nil, ErrChanged{t}
	}
	return t, nil
}

func (f *File) change(key, version string, fn func(t *Task) error) (*Task, error) {
	if _, err := f.Get(key, version); err != nil {
		return nil, err
	}
	f.FillKeys()
	t := f.Find(strings.SplitN(key, "@", 2)[0])
	if err := fn(t); err != nil {
		return nil, err
	}
	f.put(t)
	return t, nil
}

func (f *File) Tick(key string, done bool) (*Task, error) {
	return f.TickV(key, "", done)
}

func (f *File) TickV(key, version string, done bool) (*Task, error) {
	return f.TickBy(key, version, done, "")
}

func (f *File) TickBy(key, version string, done bool, by string) (*Task, error) {
	return f.change(key, version, func(t *Task) error {
		if t.Done != done {
			t.Done, t.DoneAt, t.DoneBy = done, "", ""
			if done {
				t.DoneAt, t.DoneBy = Now(), by
			}
		}
		return nil
	})
}

func (f *File) Rename(key, version, title string) (*Task, error) {
	title, err := cleanTitle(title)
	if err != nil {
		return nil, err
	}
	return f.change(key, version, func(t *Task) error { t.Title = title; return nil })
}

func (f *File) SetNeeds(key, version string, needs []string) (*Task, error) {
	return f.change(key, version, func(t *Task) error { t.Needs = needs; return nil })
}

func (f *File) Update(key, version string, fn func(t *Task) error) (*Task, error) {
	return f.change(key, version, fn)
}

func (f *File) AppendBody(key string, lines ...string) (*Task, error) {
	t, err := f.change(key, "", func(*Task) error { return nil })
	if err != nil {
		return nil, err
	}
	var add []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			add = append(add, f.noteLine(t.indent+"  "+strings.TrimSpace(l)))
		}
	}
	f.insert(t.Line+t.head, add...)
	f.index()
	return f.Find(t.Key), nil
}

func (f *File) noteLine(l string) string {
	if !f.Subtasks() {
		return l
	}
	return boxRe.ReplaceAllString(l, `$1\[$2] `)
}

func UnescapeNote(l string) string { return escBoxRe.ReplaceAllString(l, `$1[$2] `) }

func (t *Task) isOption(l string) bool {
	return optionRe.MatchString(l) && indentOf(l) <= len(t.indent)+4
}

func (f *File) SetNotes(key, version, notes string, add, keepOptions bool) (*Task, error) {
	t, err := f.change(key, version, func(*Task) error { return nil })
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(notes, "\n") {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		l = f.noteLine(t.indent + "  " + l)
		if keepOptions && t.isOption(l) {
			return nil, fmt.Errorf("a note on a question can't start with \"- \" or \"1.\": it would read as an option")
		}
		lines = append(lines, l)
	}
	if add && len(lines) == 0 {
		return nil, fmt.Errorf("the note is empty")
	}
	var body []string
	switch {
	case add:
		body = append(slices.Clone(t.Body), lines...)
	case keepOptions:
		for _, b := range t.Body {
			if t.isOption(b) {
				body = append(body, b)
			}
		}
		body = append(body, lines...)
	default:
		body = lines
	}
	for n := len(t.bodyAt) - 1; n >= 0; n-- {
		f.Lines = slices.Delete(f.Lines, t.bodyAt[n], t.bodyAt[n]+1)
	}
	f.insert(t.Line, body...)
	f.index()
	return f.Find(t.Key), nil
}

func (f *File) Remove(key, version string) (*Task, error) {
	t, err := f.Get(key, version)
	if err != nil {
		return nil, err
	}
	f.Lines = slices.Delete(f.Lines, t.Line-1, t.End)
	f.index()
	return t, nil
}

func (f *File) unknown(key string) error {
	var near []string
	for _, t := range f.Tasks {
		if !t.Done && t.Key != "" {
			near = append(near, t.Key+" "+t.Title)
		}
	}
	if len(near) > 5 {
		near = near[:5]
	}
	msg := fmt.Sprintf("no item %s", key)
	if len(near) > 0 {
		msg += "; open ones: " + strings.Join(near, "; ")
	}
	return fmt.Errorf("%s", msg)
}

func (f *File) Counts() (open, done int) {
	cs := f.Conflicts()
	for _, t := range f.Tasks {
		if inTheirs(cs, t.Line-1) {
			continue
		}
		if t.Done {
			done++
		} else {
			open++
		}
	}
	return
}

type Heading struct {
	Name  string `json:"name"`
	Level int    `json:"level"`
}

func (f *File) Headings() []Heading {
	out := make([]Heading, len(f.Sections))
	for i, n := range f.Sections {
		out[i] = Heading{n, f.levels[i]}
	}
	return out
}

func (f *File) span(name string) (int, int, error) {
	head := f.heading(strings.TrimSpace(strings.TrimLeft(name, "#")))
	if head < 0 {
		return 0, 0, f.noSection(name)
	}
	level := len(f.Lines[head]) - len(strings.TrimLeft(f.Lines[head], "#"))
	end := head
	for {
		end = f.nextHeading(end)
		if end >= len(f.Lines) || len(f.Lines[end])-len(strings.TrimLeft(f.Lines[end], "#")) <= level {
			return head, end, nil
		}
	}
}

func (f *File) noSection(name string) error {
	msg := fmt.Sprintf("no section %q", strings.TrimSpace(strings.TrimLeft(name, "#")))
	if len(f.Sections) > 0 {
		msg += "; the sections are " + strings.Join(f.Sections, ", ")
	}
	return fmt.Errorf("%s", msg)
}

func (f *File) RenameSection(name, to string) (string, error) {
	head, _, err := f.span(name)
	if err != nil {
		return "", err
	}
	to = strings.TrimSpace(strings.TrimLeft(to, "#"))
	if to == "" {
		return "", fmt.Errorf("rename the section to what?")
	}
	if _, err := cleanTitle(to); err != nil {
		return "", err
	}
	old := headingRe.FindStringSubmatch(f.Lines[head])[1]
	if other := f.heading(to); other >= 0 && other != head {
		return "", fmt.Errorf("there's already a section %q", to)
	}
	hashes := f.Lines[head][:len(f.Lines[head])-len(strings.TrimLeft(f.Lines[head], "#"))]
	f.Lines[head] = hashes + " " + to
	f.index()
	return old, nil
}

func (f *File) MoveSection(name, target string, after bool) error {
	head, end, err := f.span(name)
	if err != nil {
		return err
	}
	th, _, err := f.span(target)
	if err != nil {
		return err
	}
	if th >= head && th < end {
		return fmt.Errorf("%s is inside %s", strings.TrimSpace(target), strings.TrimSpace(name))
	}
	block := slices.Clone(f.Lines[head:end])
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1]
	}
	f.Lines = slices.Delete(f.Lines, head, end)
	for len(f.Lines) > 0 && strings.TrimSpace(f.Lines[len(f.Lines)-1]) == "" {
		f.Lines = f.Lines[:len(f.Lines)-1]
	}
	f.index()
	at, tend, _ := f.span(target)
	if after {
		at = tend
		for at > 0 && strings.TrimSpace(f.Lines[at-1]) == "" {
			at--
		}
	}
	add := block
	if at > 0 && strings.TrimSpace(f.Lines[at-1]) != "" {
		add = append([]string{""}, add...)
	}
	if at < len(f.Lines) {
		add = append(slices.Clone(add), "")
	}
	f.insert(at, add...)
	f.newline = true
	f.index()
	return nil
}

func (f *File) RemoveSection(name, into string) ([]*Task, error) {
	f.FillKeys()
	head, end, err := f.span(name)
	if err != nil {
		return nil, err
	}
	var moved []*Task
	for _, t := range f.Tasks {
		if t.Line-1 > head && t.Line-1 < end {
			moved = append(moved, t)
		}
	}
	for i := head + 1; i < end; i++ {
		l := f.Lines[i]
		if strings.TrimSpace(l) == "" || slices.ContainsFunc(moved, func(t *Task) bool { return i >= t.Line-1 && i < t.End }) {
			continue
		}
		if headingRe.MatchString(l) {
			return nil, fmt.Errorf("%s has sections inside it; remove or move those first", strings.TrimSpace(name))
		}
		return nil, fmt.Errorf("%s has text besides its items; edit the file to remove it", strings.TrimSpace(name))
	}
	if len(moved) > 0 && strings.TrimSpace(into) == "" {
		return nil, fmt.Errorf("%s still holds %d items; give into, the section they go to", strings.TrimSpace(name), len(moved))
	}
	if len(moved) > 0 && f.heading(strings.TrimSpace(strings.TrimLeft(into, "#"))) == head {
		return nil, fmt.Errorf("move the items to another section, not %s", strings.TrimSpace(into))
	}
	var block []string
	for _, t := range f.Tasks {
		if t.Line-1 > head && t.Line-1 < end && t.parent == nil {
			block = append(block, f.Lines[t.Line-1:t.End]...)
		}
	}
	for end > head && end < len(f.Lines) && strings.TrimSpace(f.Lines[end]) == "" {
		end++
	}
	for head > 0 && strings.TrimSpace(f.Lines[head-1]) == "" && end >= len(f.Lines) {
		head--
	}
	f.Lines = slices.Delete(f.Lines, head, end)
	f.index()
	if len(block) > 0 {
		f.place(block, into)
	}
	var out []*Task
	for _, t := range moved {
		out = append(out, f.Find(t.Key))
	}
	return out, nil
}

type Removed struct {
	Lines   []string `json:"lines"`
	Section string   `json:"section,omitempty"`
	After   string   `json:"after,omitempty"`
	Before  string   `json:"before,omitempty"`
	Parent  string   `json:"parent,omitempty"`
}

func (f *File) Cut(key, version string) (*Task, Removed, error) {
	f.FillKeys()
	t, err := f.Get(key, version)
	if err != nil {
		return nil, Removed{}, err
	}
	r := f.Snapshot(t)
	f.Lines = slices.Delete(f.Lines, t.Line-1, t.End)
	f.index()
	return t, r, nil
}

func (f *File) Snapshot(t *Task) Removed {
	r := Removed{Lines: slices.Clone(f.Lines[t.Line-1 : t.End]), Section: t.Section, Parent: t.Parent}
	var sibs []*Task
	for _, x := range f.Tasks {
		if x.parent == t.parent && x.Section == t.Section {
			sibs = append(sibs, x)
		}
	}
	if i := slices.Index(sibs, t); i >= 0 {
		if i > 0 {
			r.After = sibs[i-1].Key
		}
		if i+1 < len(sibs) {
			r.Before = sibs[i+1].Key
		}
	}
	return r
}

func (f *File) PutBack(key string, r Removed) (*Task, error) {
	if t := f.Find(key); t != nil {
		return nil, fmt.Errorf("%s is already in the list", t.Key)
	}
	if len(r.Lines) == 0 || ParseLine(r.Lines[0]) == nil {
		return nil, fmt.Errorf("there's no copy of %s to bring back", key)
	}
	for _, x := range []struct {
		key   string
		after bool
	}{{r.After, true}, {r.Before, false}} {
		to := f.Find(x.key)
		if x.key == "" || to == nil || !strings.EqualFold(to.Section, r.Section) {
			continue
		}
		block := slices.Clone(r.Lines)
		ind := r.Lines[0][:indentOf(r.Lines[0])]
		for i, l := range block {
			block[i] = to.indent + strings.TrimPrefix(l, ind)
		}
		at := to.Line - 1
		if x.after {
			at = to.End
		}
		f.insert(at, block...)
		f.index()
		return f.Find(key), nil
	}
	block := slices.Clone(r.Lines)
	for i, l := range block {
		block[i] = strings.TrimPrefix(l, r.Lines[0][:indentOf(r.Lines[0])])
	}
	if p := f.Find(r.Parent); r.Parent != "" && p != nil {
		f.underParent(p, block)
		return f.Find(key), nil
	}
	section := r.Section
	if section == "" {
		section = f.defaultSection()
	}
	f.place(block, section)
	return f.Find(key), nil
}

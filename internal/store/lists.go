package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

const listsFile = ".callboard/lists.md"

const listsHead = "# Lists\n\nMore lists besides backlog.md, requests.md and questions.md. Each is a task list with its own key letter. Add one with callboard lists add NAME.\n\n"

var (
	listLineRe = regexp.MustCompile(`^- (\S+\.md) · keys ([A-Z])-\s*$`)
	listNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	reserved   = []string{"backlog", "requests", "questions", "views", "lists", "all", "task", "tasks", "request", "question", "readme", "changelog", "license", "agents", "claude", "foryou", "waiting", "now", "map", "quiet", "recent", "branches", "open", "ready", "done", "claimed", "you"}
)

func (s *Store) Custom() []Kind {
	if s == nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(s.Root, listsFile))
	if err != nil {
		return nil
	}
	var out []Kind
	for _, l := range strings.Split(string(b), "\n") {
		m := listLineRe.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(filepath.Base(m[1])), ".md")
		if !listNameRe.MatchString(name) || slices.Contains(reserved, name) || filepath.Base(m[1]) != m[1] {
			continue
		}
		k := Kind{Name: name, File: m[1], Prefix: m[2] + "-", Noun: Backlog.Noun}
		if k.Prefix == Views.Prefix || slices.ContainsFunc(Kinds, func(o Kind) bool { return o.Name == k.Name || o.Prefix == k.Prefix }) || slices.ContainsFunc(out, func(o Kind) bool { return o.Name == k.Name }) {
			continue
		}
		out = append(out, k)
	}
	return out
}

func NewView(s *Store, files map[string]*board.File, claims map[string]Claim) *View {
	return &View{Files: files, Claims: claims, s: s}
}

func (v *View) Lists() []Kind { return v.s.Lists() }

func (s *Store) Lists() []Kind {
	return append(slices.Clone(Kinds), s.Custom()...)
}

func (k Kind) Own() bool {
	return !slices.Contains(Kinds, k) && k != Views
}

func (s *Store) KindOf(key string) (Kind, bool) {
	if len(key) <= 2 {
		return Kind{}, false
	}
	var same []Kind
	for _, k := range append(s.Lists(), Views) {
		if strings.EqualFold(key[:2], k.Prefix) {
			same = append(same, k)
		}
	}
	if len(same) == 0 {
		return Kind{}, false
	}
	if len(same) > 1 {
		for _, k := range same {
			if b, err := os.ReadFile(s.File(k)); err == nil && holdsKey(string(b), key) {
				return k, true
			}
		}
	}
	return same[0], true
}

func holdsKey(text, key string) bool {
	id := "id:" + strings.ToLower(strings.SplitN(key, "@", 2)[0])
	text = strings.ToLower(text)
	for i := strings.Index(text, id); i >= 0; {
		end := i + len(id)
		if end == len(text) || !isKeyChar(text[end]) {
			return true
		}
		n := strings.Index(text[end:], id)
		if n < 0 {
			break
		}
		i = end + n
	}
	return false
}

func isKeyChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

func (s *Store) Sharing(k Kind) []string {
	var out []string
	for _, o := range s.Custom() {
		if o.Prefix == k.Prefix && o.Name != k.Name {
			out = append(out, o.Name)
		}
	}
	return out
}

func (s *Store) KindNamed(name string) (Kind, error) {
	if k, err := KindNamed(name); err == nil {
		return k, nil
	}
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".md")
	var names []string
	for _, k := range s.Custom() {
		if k.Name == n || strings.EqualFold(strings.TrimSuffix(k.Prefix, "-"), n) {
			return k, nil
		}
		names = append(names, k.Name)
	}
	hint := ""
	if listNameRe.MatchString(n) && !slices.Contains(reserved, n) {
		hint = fmt.Sprintf("; to start a list called %s, make it first (the lists tool with add: %q, or callboard lists add %s)", n, n, n)
	}
	if len(names) == 0 {
		return Kind{}, fmt.Errorf("kind is task, request or question, not %q%s", name, hint)
	}
	return Kind{}, fmt.Errorf("kind is task, request, question or one of your lists (%s), not %q%s", strings.Join(names, ", "), name, hint)
}

func (s *Store) kindFor(key string) (Kind, error) {
	k, ok := s.KindOf(key)
	if !ok {
		return Kind{}, fmt.Errorf("%q isn't a key; keys look like B-k3f9 (tasks), R-2m8a (requests) or Q-7x1c (questions)", key)
	}
	return k, nil
}

func (s *Store) mergeLines(k Kind) {
	if s.Common == "" {
		return
	}
	if d, _ := s.Git("config", "--get", "merge.callboard.driver"); strings.TrimSpace(d) == "" {
		return
	}
	p := filepath.Join(s.Root, ".gitattributes")
	b, _ := os.ReadFile(p)
	text := string(b)
	have := strings.Split(text, "\n")
	for _, l := range []string{k.File + " merge=callboard", listsFile + " merge=union"} {
		if slices.Contains(have, l) {
			continue
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += l + "\n"
	}
	if text != string(b) {
		os.WriteFile(p, []byte(text), 0o644)
	}
}

func (s *Store) AddList(name string, a Actor) (Kind, bool, error) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".md")
	if !listNameRe.MatchString(n) {
		return Kind{}, false, fmt.Errorf("a list's name is a lowercase word like ideas or bugs, not %q", name)
	}
	if slices.Contains(reserved, n) {
		return Kind{}, false, fmt.Errorf("%s is taken; pick another name", n)
	}
	lists := s.Lists()
	for _, k := range lists {
		if k.Name == n {
			return k, false, nil
		}
	}
	used := map[string]bool{"V": true}
	for _, k := range lists {
		used[strings.TrimSuffix(k.Prefix, "-")] = true
	}
	letter := ""
	for _, r := range strings.ToUpper(n) + "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		if c := string(r); r >= 'A' && r <= 'Z' && !used[c] {
			letter = c
			break
		}
	}
	if letter == "" {
		return Kind{}, false, fmt.Errorf("every key letter is taken")
	}
	k := Kind{Name: n, File: n + ".md", Prefix: letter + "-", Noun: Backlog.Noun}
	unlock, err := s.lock()
	if err != nil {
		return Kind{}, false, err
	}
	defer unlock()
	p := filepath.Join(s.Root, listsFile)
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return Kind{}, false, err
	}
	text := string(b)
	if strings.TrimSpace(text) == "" {
		text = listsHead
	} else if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "- " + k.File + " · keys " + letter + "-\n"
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return Kind{}, false, err
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		return Kind{}, false, err
	}
	if _, err := os.Stat(s.File(k)); os.IsNotExist(err) {
		title := strings.ToUpper(n[:1]) + strings.ReplaceAll(n[1:], "-", " ")
		if err := os.WriteFile(s.File(k), []byte("# "+title+"\n"), 0o644); err != nil {
			return Kind{}, false, err
		}
	}
	s.mergeLines(k)
	s.Log(Event{By: a.By, What: "list", Title: k.File, Detail: "keys " + k.Prefix, Session: a.Session})
	return k, true, nil
}

func (s *Store) ownList(name string) (Kind, error) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".md")
	for _, k := range s.Custom() {
		if k.Name == n {
			return k, nil
		}
	}
	if _, err := KindNamed(n); err == nil {
		return Kind{}, fmt.Errorf("%s is one of Callboard's own lists; only lists you made can be renamed or removed", n)
	}
	return Kind{}, fmt.Errorf("there's no list called %s (callboard lists shows them)", n)
}

func (s *Store) editListLines(fn func(line string, k Kind) (string, bool)) error {
	p := filepath.Join(s.Root, listsFile)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if m := listLineRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			k := Kind{Name: strings.TrimSuffix(m[1], ".md"), File: m[1], Prefix: m[2] + "-"}
			nl, keep := fn(l, k)
			if !keep {
				continue
			}
			l = nl
		}
		out = append(out, l)
	}
	return os.WriteFile(p, []byte(strings.Join(out, "\n")), 0o644)
}

func (s *Store) editAttrs(old, new string) {
	p := filepath.Join(s.Root, ".gitattributes")
	b, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l == old+" merge=callboard" {
			if new == "" {
				continue
			}
			l = new + " merge=callboard"
		}
		out = append(out, l)
	}
	if text := strings.Join(out, "\n"); text != string(b) {
		os.WriteFile(p, []byte(text), 0o644)
	}
}

func (s *Store) editViews(fn func(list string) (string, bool)) []string {
	p := s.File(Views)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	f := board.ParseAs(string(b), Views.Prefix)
	var touched []string
	for i := len(f.Tasks) - 1; i >= 0; i-- {
		t := f.Tasks[i]
		to, keep := fn(t.Meta("list"))
		switch {
		case !keep:
			f.Remove(t.Key, "")
		case to != t.Meta("list"):
			f.SetFields(t.Key, "", [][2]string{{"list", to}})
		default:
			continue
		}
		touched = append(touched, t.Title)
	}
	if len(touched) > 0 {
		os.WriteFile(p, []byte(f.String()), 0o644)
	}
	return touched
}

func (s *Store) tracked(file string) bool {
	_, err := s.Git("ls-files", "--error-unmatch", "--", file)
	return err == nil && s.Common != ""
}

func (s *Store) RenameList(from, to string, a Actor) (Kind, error) {
	k, err := s.ownList(from)
	if err != nil {
		return Kind{}, err
	}
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(to)), ".md")
	if !listNameRe.MatchString(n) {
		return Kind{}, fmt.Errorf("a list's name is a lowercase word like ideas or bugs, not %q", to)
	}
	if slices.Contains(reserved, n) {
		return Kind{}, fmt.Errorf("%s is taken; pick another name", n)
	}
	if n == k.Name {
		return k, nil
	}
	for _, o := range s.Lists() {
		if o.Name == n {
			return Kind{}, fmt.Errorf("there's already a list called %s", n)
		}
	}
	nk := Kind{Name: n, File: n + ".md", Prefix: k.Prefix, Noun: k.Noun}
	if _, err := os.Stat(s.File(nk)); err == nil {
		return Kind{}, fmt.Errorf("%s already exists; pick another name or move that file first", nk.File)
	}
	unlock, err := s.lock()
	if err != nil {
		return Kind{}, err
	}
	defer unlock()
	if _, err := os.Stat(s.File(k)); err == nil {
		if s.tracked(k.File) {
			if _, err := s.Git("mv", "--", k.File, nk.File); err != nil {
				return Kind{}, err
			}
		} else if err := os.Rename(s.File(k), s.File(nk)); err != nil {
			return Kind{}, err
		}
		if b, err := os.ReadFile(s.File(nk)); err == nil {
			old := "# " + strings.ToUpper(k.Name[:1]) + strings.ReplaceAll(k.Name[1:], "-", " ") + "\n"
			if strings.HasPrefix(string(b), old) {
				os.WriteFile(s.File(nk), []byte("# "+strings.ToUpper(n[:1])+strings.ReplaceAll(n[1:], "-", " ")+"\n"+string(b)[len(old):]), 0o644)
			}
		}
	}
	if err := s.editListLines(func(l string, o Kind) (string, bool) {
		if o.File == k.File {
			return strings.Replace(l, k.File, nk.File, 1), true
		}
		return l, true
	}); err != nil {
		return Kind{}, err
	}
	s.editAttrs(k.File, nk.File)
	s.editViews(func(list string) (string, bool) {
		if list == k.Name {
			return n, true
		}
		return list, true
	})
	s.moveSeen(k.File, nk.File)
	s.Log(Event{By: a.By, What: "list", Title: nk.File, Detail: "renamed from " + k.File, Session: a.Session})
	return nk, nil
}

func (s *Store) RemoveList(name string, force bool, a Actor) (Kind, []string, error) {
	k, err := s.ownList(name)
	if err != nil {
		return Kind{}, nil, err
	}
	f, err := s.Load(k)
	if err != nil {
		return Kind{}, nil, err
	}
	if open, _ := f.Counts(); open > 0 && !force {
		return Kind{}, nil, fmt.Errorf("%s still has %d open items; finish or move them first, or pass force to remove it anyway", k.File, open)
	}
	unlock, err := s.lock()
	if err != nil {
		return Kind{}, nil, err
	}
	defer unlock()
	if _, err := os.Stat(s.File(k)); err == nil {
		if s.tracked(k.File) {
			if _, err := s.Git("rm", "-q", "-f", "--", k.File); err != nil {
				return Kind{}, nil, err
			}
		} else if err := os.Remove(s.File(k)); err != nil {
			return Kind{}, nil, err
		}
	}
	if err := s.editListLines(func(l string, o Kind) (string, bool) { return l, o.File != k.File }); err != nil {
		return Kind{}, nil, err
	}
	s.editAttrs(k.File, "")
	views := s.editViews(func(list string) (string, bool) { return list, list != k.Name })
	s.moveSeen(k.File, "")
	s.Log(Event{By: a.By, What: "list", Title: k.File, Detail: "removed", Session: a.Session})
	return k, views, nil
}

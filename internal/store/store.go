package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/gitx"
)

type Kind struct {
	Name   string
	File   string
	Prefix string
	Noun   string
}

var (
	Backlog   = Kind{"backlog", "backlog.md", "B-", "task"}
	Requests  = Kind{"requests", "requests.md", "R-", "request"}
	Questions = Kind{"questions", "questions.md", "Q-", "question"}
	Kinds     = []Kind{Backlog, Requests, Questions}
)

const FileName = "backlog.md"

func KindOf(key string) (Kind, bool) {
	for _, k := range append(Kinds, Views) {
		if len(key) > 2 && strings.EqualFold(key[:2], k.Prefix) {
			return k, true
		}
	}
	return Kind{}, false
}

func KindNamed(name string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "task", "tasks", "backlog", "b":
		return Backlog, nil
	case "request", "requests", "r":
		return Requests, nil
	case "question", "questions", "q":
		return Questions, nil
	}
	return Kind{}, fmt.Errorf("kind is task, request or question, not %q", name)
}

type Store struct {
	gitx.Repo
	Path string
}

func Open(dir string) *Store {
	r := gitx.Find(dir)
	return &Store{Repo: r, Path: filepath.Join(r.Root, Backlog.File)}
}

var ErrOff = errors.New("switched off in this repo, so the lists can be read but not changed. Turn Callboard back on with: callboard on")

func (s *Store) InUse() bool {
	return !s.Off() && s.HasLists()
}

func (s *Store) HasLists() bool {
	if s.Common != "" {
		return true
	}
	for _, k := range Kinds {
		if _, err := os.Stat(s.File(k)); err == nil && !s.Foreign(k) {
			return true
		}
	}
	_, err := os.Stat(filepath.Join(s.Root, ".callboard"))
	return err == nil
}

func (s *Store) Foreign(k Kind) bool {
	if !slices.Contains(Kinds, k) {
		return false
	}
	b, err := os.ReadFile(s.File(k))
	return err == nil && board.Foreign(string(b))
}

func (s *Store) ForeignError(k Kind) error {
	return fmt.Errorf("%s here isn't a Callboard list: it has no Callboard items, so it's likely someone else's file, and Callboard leaves it alone. To keep %ss in Callboard, rename or move that file; if it is a checklist meant for Callboard, callboard keys hands its items over", k.File, k.Noun)
}

func (s *Store) File(k Kind) string { return filepath.Join(s.Root, k.File) }

func (s *Store) Stamp() string {
	var b strings.Builder
	paths := []string{filepath.Join(s.Shared(), "claims.json"), filepath.Join(s.Root, listsFile)}
	for _, k := range append(s.Lists(), Views) {
		paths = append(paths, s.File(k))
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			fmt.Fprint(&b, st.ModTime().UnixNano(), st.Size(), ";")
		} else {
			b.WriteString("none;")
		}
	}
	return b.String()
}

func (s *Store) Load(k Kind) (*board.File, error) {
	return s.load(k, false)
}

func (s *Store) load(k Kind, adopt bool) (*board.File, error) {
	b, err := os.ReadFile(s.File(k))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if !adopt && slices.Contains(Kinds, k) && board.Foreign(string(b)) {
		b = nil
	}
	f := board.ParseAs(string(b), k.Prefix)
	if others := s.Sharing(k); len(others) > 0 {
		f.Taken = func(key string) bool {
			for _, o := range others {
				if b, err := os.ReadFile(filepath.Join(s.Root, o+".md")); err == nil && holdsKey(string(b), key) {
					return true
				}
			}
			return false
		}
	}
	return f, nil
}

func (s *Store) LoadAll() (map[string]*board.File, error) {
	out := map[string]*board.File{}
	for _, k := range s.Lists() {
		f, err := s.Load(k)
		if err != nil {
			return nil, err
		}
		out[k.Name] = f
	}
	return out, nil
}

func (s *Store) Update(k Kind, fn func(f *board.File) error) (*board.File, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.updateBy(k, Actor{}, fn)
}

func (s *Store) UpdateBy(k Kind, a Actor, fn func(f *board.File) error) (*board.File, error) {
	unlock, err := s.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return s.updateBy(k, a, fn)
}

func (s *Store) Adopt(k Kind) (int, error) {
	unlock, err := s.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	n := 0
	_, err = s.update(k, Actor{}, true, func(f *board.File) error { n = f.FillKeys(); return nil })
	return n, err
}

func (s *Store) updateBy(k Kind, a Actor, fn func(f *board.File) error) (*board.File, error) {
	return s.update(k, a, false, fn)
}

func (s *Store) update(k Kind, a Actor, adopt bool, fn func(f *board.File) error) (*board.File, error) {
	s.reconcile()
	raw, _ := os.ReadFile(s.File(k))
	if !adopt && slices.Contains(Kinds, k) && board.Foreign(string(raw)) {
		return board.ParseAs("", k.Prefix), s.ForeignError(k)
	}
	f, err := s.load(k, adopt)
	if err != nil {
		return nil, err
	}
	own := s.ownChanges(a.Session)
	f.Also = own.also(f)
	versions := map[string]string{}
	for _, t := range f.Tasks {
		versions[strings.ToUpper(t.Key)] = t.Version
	}
	before := f.String()
	if err := fn(f); err != nil {
		return f, err
	}
	if after := f.String(); after != before {
		if err := writeAtomic(s.File(k), after); err != nil {
			return f, err
		}
		s.noteSeen(k, string(raw), after)
		for _, t := range f.Tasks {
			if old, ok := versions[strings.ToUpper(t.Key)]; ok && old != t.Version {
				own.note(t.Key, old, t.Version)
			}
		}
		s.saveOwnChanges(a.Session, own)
	}
	return f, nil
}

func (s *Store) Locked(fn func() error) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return fn()
}

func writeAtomic(path, content string) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".callboard-tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func By() string {
	if b := os.Getenv("CALLBOARD_BY"); b != "" {
		return b
	}

	if os.Getenv("CODEX_THREAD_ID") != "" {
		return "codex"
	}
	if os.Getenv("CLAUDECODE") == "1" {
		return "claude"
	}
	return "you"
}

func InAgent() bool {
	return os.Getenv("CODEX_THREAD_ID") != "" || os.Getenv("CLAUDECODE") == "1"
}

func AgentSession() string {
	if id := os.Getenv("CODEX_THREAD_ID"); id != "" {
		return id
	}
	return os.Getenv("CLAUDE_CODE_SESSION_ID")
}

func (s *Store) Rel(from string) string {
	if f, err := filepath.EvalSymlinks(from); err == nil {
		from = f
	}
	root := s.Root
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if r, err := filepath.Rel(from, filepath.Join(root, Backlog.File)); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return Tilde(s.Path)
}

func Tilde(p string) string {
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

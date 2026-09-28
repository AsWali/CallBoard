package store

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type Actor struct {
	By      string
	Session string
}

func splitKey(key string) (string, string) {
	k, v, _ := strings.Cut(strings.TrimSpace(key), "@")
	return k, v
}

func (s *Store) Add(k Kind, n board.New, a Actor) (*board.Task, error) {
	n.By = a.By
	if n.Parent != "" {
		n.Parent, _ = splitKey(n.Parent)
		pk, err := s.kindFor(n.Parent)
		if err != nil {
			return nil, err
		}
		k = pk
	}
	var t, reopened *board.Task
	err := s.Locked(func() error {
		if err := s.checkNeeds(n.Needs, ""); err != nil {
			return err
		}
		_, err := s.updateBy(k, a, func(f *board.File) (err error) {
			if t, err = f.AddNew(n); err != nil || n.Parent == "" {
				return err
			}
			if p := f.Find(n.Parent); p != nil && p.Done {
				if reopened, err = f.TickV(p.Key, "", false); err != nil {
					return err
				}
				if st := s.statusAfterTick(f, reopened, false); st != "" {
					reopened, err = setStatus(f, reopened.Key, st)
				}
			}
			return err
		})
		return err
	})
	if err == nil {
		s.Log(Event{By: a.By, What: "added", Key: t.Key, Title: t.Title, Session: a.Session})
		if reopened != nil {
			s.Log(Event{By: a.By, What: "reopened", Key: reopened.Key, Title: reopened.Title, Session: a.Session})
		}
	}
	return t, err
}

func (s *Store) AddTask(title, section, by string) (*board.Task, error) {
	return s.Add(Backlog, board.New{Title: title, Section: section}, Actor{By: by})
}

func (s *Store) Tick(key string, done bool, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	if k == Questions {
		return nil, fmt.Errorf("%s is a question: answer it (answer tool, or on the page) instead of ticking it", key)
	}
	var t *board.Task
	changed := false
	_, err = s.UpdateBy(k, a, func(f *board.File) error {
		if cur := f.Find(key); cur != nil && cur.Done == done {
			return s.already(cur)
		}
		old, err := f.Get(key, version)
		if err != nil {
			return err
		}
		changed = old.Done != done
		if done && changed {
			var open []string
			for _, x := range f.Tasks {
				if strings.EqualFold(x.Parent, old.Key) && !x.Done {
					open = append(open, x.Key)
				}
			}
			if len(open) > 0 {
				return fmt.Errorf("%s still has open subtasks: %s. Tick those first", old.Key, strings.Join(open, ", "))
			}
		}
		if t, err = f.TickBy(key, version, done, a.By); err != nil {
			return err
		}
		if st := s.statusAfterTick(f, t, done); st != "" && changed {
			t, err = setStatus(f, t.Key, st)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if changed {
		what := "ticked"
		if !done {
			what = "reopened"
		}
		s.Log(Event{By: a.By, What: what, Key: t.Key, Title: t.Title, Session: a.Session})
	}
	if done {
		s.Release(t.Key, a.By, a.Session, true)
	}
	return t, nil
}

func (s *Store) Rename(key, title string, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	var t *board.Task
	var old string
	_, err = s.UpdateBy(k, a, func(f *board.File) error {
		o, err := f.Get(key, version)
		if err != nil {
			return err
		}
		old = o.Title
		t, err = f.Rename(key, version, title)
		return err
	})
	if err == nil && old != t.Title {
		s.Log(Event{By: a.By, What: "renamed", Key: t.Key, Title: t.Title, Detail: old, Session: a.Session})
	}
	return t, err
}

func (s *Store) SetNotes(key, notes string, add bool, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	var t *board.Task
	var before []string
	_, err = s.UpdateBy(k, a, func(f *board.File) error {
		o, err := f.Get(key, version)
		if err != nil {
			return err
		}
		before = o.Body
		t, err = f.SetNotes(key, version, notes, add, k == Questions)
		return err
	})
	if err == nil && !slices.Equal(before, t.Body) {
		what := "notes"
		if add {
			what = "noted"
		}
		s.Log(Event{By: a.By, What: what, Key: t.Key, Title: t.Title, Detail: noteLine(notes), Session: a.Session})
	}
	return t, err
}

func noteLine(notes string) string {
	var parts []string
	for _, l := range strings.Split(notes, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	line := []rune(strings.Join(parts, " · "))
	if len(line) > 120 {
		return string(line[:119]) + "…"
	}
	return string(line)
}

func (s *Store) SetNeeds(key string, needs []string, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	var t *board.Task
	err = s.Locked(func() error {
		if err := s.checkNeeds(needs, key); err != nil {
			return err
		}
		_, err := s.updateBy(k, a, func(f *board.File) (err error) {
			t, err = f.SetNeeds(key, version, needs)
			return
		})
		return err
	})
	if err == nil {
		detail := strings.Join(t.Needs, ", ")
		if detail == "" {
			detail = "nothing"
		}
		s.Log(Event{By: a.By, What: "needs", Key: t.Key, Title: t.Title, Detail: detail, Session: a.Session})
	}
	return t, err
}

func (s *Store) checkNeeds(needs []string, self string) error {
	for i, n := range needs {
		n = strings.TrimSpace(n)
		needs[i] = n
		if strings.EqualFold(n, self) {
			return fmt.Errorf("%s can't wait on itself", n)
		}
		k, err := s.kindFor(n)
		if err != nil {
			return err
		}
		f, err := s.Load(k)
		if err != nil {
			return err
		}
		if t := f.Find(n); t != nil {
			needs[i] = t.Key
			continue
		}
		return fmt.Errorf("no item %s to wait on", n)
	}
	return nil
}

func (s *Store) Assume(key string, option int, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	if k, _ := s.KindOf(key); k != Questions {
		return nil, fmt.Errorf("%s isn't a question", key)
	}
	var t *board.Task
	var pick string
	_, err := s.UpdateBy(Questions, a, func(f *board.File) error {
		q, err := f.Get(key, version)
		if err != nil {
			return err
		}
		opts := q.Options()
		if option < 1 || option > len(opts) {
			return fmt.Errorf("%s has %d options; pick 1 to %d", q.Key, len(opts), len(opts))
		}
		pick = opts[option-1].Text
		t, err = f.Update(key, version, func(t *board.Task) error { t.Assumed = option; return nil })
		return err
	})
	if err == nil {
		s.Log(Event{By: a.By, What: "assumed", Key: t.Key, Title: t.Title, Detail: pick, Session: a.Session})
	}
	return t, err
}

type Answered struct {
	Key       string   `json:"key"`
	Question  string   `json:"question"`
	Answer    string   `json:"answer"`
	Why       string   `json:"why,omitempty"`
	Version   string   `json:"version"`
	Unblocked []string `json:"unblocked"`

	Reopened []string `json:"reopened,omitempty"`
	Assumed  string   `json:"assumed,omitempty"`
}

var ErrNoQuestion = errors.New("no such question here")

func (s *Store) AnswerQuestion(key, pick, why string, a Actor) (Answered, error) {
	key, version := splitKey(key)
	if k, _ := s.KindOf(key); k != Questions {
		return Answered{}, fmt.Errorf("%s isn't a question", key)
	}
	var res Answered
	err := s.Locked(func() error {
		_, err := s.updateBy(Questions, a, func(f *board.File) error {
			q := f.Find(key)
			if q == nil {
				return fmt.Errorf("%w: %s", ErrNoQuestion, key)
			}
			if _, err := f.Get(key, version); err != nil {
				return err
			}
			opts := q.Options()
			if q.Assumed > 0 && q.Assumed <= len(opts) {
				res.Assumed = opts[q.Assumed-1].Text
			}
			answer := strings.TrimSpace(pick)
			if answer == "" && q.Assumed > 0 && q.Assumed <= len(opts) {
				answer = opts[q.Assumed-1].Text
			} else if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(opts) {
				answer = opts[n-1].Text
			}
			if answer == "" {
				return fmt.Errorf("give an answer: an option number (1-%d) or your own words", len(opts))
			}
			var err error
			if answer, err = board.CleanValue(answer); err != nil {
				return err
			}
			if why, err = board.CleanValue(why); err != nil {
				return err
			}
			t, err := f.Update(key, version, func(t *board.Task) error {
				t.Done, t.DoneAt, t.Assumed = true, board.Now(), 0
				t.SetMeta("answer", answer)
				t.SetMeta("why", why)
				t.SetMeta("answered-by", a.By)
				t.SetMeta("was", "")
				return nil
			})
			if err != nil {
				return err
			}
			res = Answered{Key: t.Key, Question: t.Title, Answer: answer, Why: why, Version: t.Version, Assumed: res.Assumed}
			return nil
		})
		return err
	})
	if err != nil {
		return Answered{}, err
	}

	res.Unblocked = []string{}
	if v, err := s.View(); err == nil {
		for _, k := range s.Lists() {
			for _, t := range v.Files[k.Name].Tasks {
				if t.Done || !slices.ContainsFunc(t.Needs, func(n string) bool { return strings.EqualFold(n, res.Key) }) {
					continue
				}
				if !v.Item(k, t).Blocked {
					res.Unblocked = append(res.Unblocked, t.Key)
				}
			}
		}
	}
	s.Release(res.Key, a.By, a.Session, true)
	s.Log(Event{By: a.By, What: "answered", Key: res.Key, Title: res.Question, Detail: res.Answer, Session: a.Session})
	if res.Assumed != "" && !strings.EqualFold(res.Assumed, res.Answer) {
		res.Reopened = s.reopenBuiltOn(res, a)
	}
	if res.Assumed == "" || strings.EqualFold(res.Assumed, res.Answer) {
		res.Assumed = ""
	}
	return res, nil
}

func (s *Store) reopenBuiltOn(res Answered, a Actor) []string {
	var out []string
	why := fmt.Sprintf("%s was answered %q, not the assumed %q", res.Key, res.Answer, res.Assumed)
	for _, k := range s.Lists() {
		if k == Questions {
			continue
		}
		var reopened []*board.Task
		s.UpdateBy(k, a, func(f *board.File) error {
			for _, t := range f.Tasks {
				if !t.Done || !slices.ContainsFunc(t.Needs, func(n string) bool { return strings.EqualFold(n, res.Key) }) {
					continue
				}
				nt, err := f.TickV(t.Key, "", false)
				if err != nil {
					return err
				}
				if st := s.statusAfterTick(f, nt, false); st != "" {
					nt, _ = setStatus(f, nt.Key, st)
				}
				reopened = append(reopened, nt)
			}
			return nil
		})
		for _, t := range reopened {
			out = append(out, t.Key)
			s.Log(Event{By: a.By, What: "reopened", Key: t.Key, Title: t.Title, Detail: why, Session: a.Session})
		}
	}
	return out
}

func Everywhere[T any](s *Store, key string, fn func(st *Store) (T, error)) (T, []string, error) {
	var first T
	var firstErr error
	got := false
	var where []string
	dirs := s.Worktrees()
	slices.SortStableFunc(dirs, func(a, b string) int {
		if a == s.Root {
			return -1
		}
		if b == s.Root {
			return 1
		}
		return 0
	})
	for _, d := range dirs {
		st := s
		if d != s.Root {
			st = Open(d)
		}
		if !st.hasKey(strings.SplitN(key, "@", 2)[0]) {
			continue
		}
		r, err := fn(st)
		if err != nil {
			if !got && firstErr == nil {
				firstErr = err
			}
			continue
		}
		where = append(where, st.Branch())
		if !got {
			first, got = r, true
		}
	}
	if !got {
		if firstErr != nil {
			return first, nil, firstErr
		}
		return first, nil, fmt.Errorf("no item %s in any worktree", key)
	}
	return first, where, nil
}

func (s *Store) Reopen(key string, a Actor) (*board.Task, error) {
	key, version := splitKey(key)
	if k, _ := s.KindOf(key); k != Questions {
		return nil, fmt.Errorf("%s isn't a question", key)
	}
	var t *board.Task
	_, err := s.UpdateBy(Questions, a, func(f *board.File) (err error) {
		t, err = f.Update(key, version, func(t *board.Task) error {
			if !t.Done {
				return fmt.Errorf("%s is still open", t.Key)
			}
			t.Done, t.DoneAt = false, ""
			t.SetMeta("was", t.Meta("answer"))
			t.SetMeta("answer", "")
			t.SetMeta("why", "")
			t.SetMeta("answered-by", "")
			return nil
		})
		return err
	})
	if err == nil {
		s.Log(Event{By: a.By, What: "reopened", Key: t.Key, Title: t.Title, Session: a.Session})
	}
	return t, err
}

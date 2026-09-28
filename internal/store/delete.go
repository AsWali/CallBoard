package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

func (s *Store) Delete(key string, a Actor) (*board.Task, []string, error) {
	key, version := splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, nil, err
	}
	var t *board.Task
	var r board.Removed
	if _, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		t, r, err = f.Cut(key, version)
		return err
	}); err != nil {
		return nil, nil, err
	}
	s.withClaims(func(m map[string]Claim) error {
		delete(m, strings.ToUpper(t.Key))
		return nil
	})
	s.Log(Event{By: a.By, What: "removed", Key: t.Key, Title: t.Title, Detail: t.Section, Session: a.Session, Copy: &r})
	var freed []string
	if v, err := s.View(); err == nil {
		for _, kk := range s.Lists() {
			for _, x := range v.Files[kk.Name].Tasks {
				if !x.Done && containsFold(x.Needs, t.Key) {
					freed = append(freed, x.Key)
				}
			}
		}
	}
	return t, freed, nil
}

func (s *Store) Restore(key string, a Actor) (*board.Task, error) {
	key, _ = splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	s.Reconcile()
	var copy *board.Removed
	for _, e := range s.ItemEvents(key, 0) {
		if e.What == "added" || e.What == "restored" {
			break
		}
		if e.What == "removed" && e.Copy != nil {
			copy = e.Copy
			break
		}
	}
	if copy == nil {
		if r, hk, ok := s.fromHistory(key); ok {
			copy, k = r, hk
		}
	}
	if copy == nil {
		return nil, fmt.Errorf("there's no deleted %s to bring back, here or in git history", key)
	}
	var t *board.Task
	if _, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		t, err = f.PutBack(key, *copy)
		return err
	}); err != nil {
		return nil, err
	}
	s.Log(Event{By: a.By, What: "restored", Key: t.Key, Title: t.Title, Detail: t.Section, Session: a.Session})
	return t, nil
}

func containsFold(xs []string, x string) bool {
	for _, y := range xs {
		if strings.EqualFold(y, x) {
			return true
		}
	}
	return false
}

type Gone struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Section string `json:"section,omitempty"`
	By      string `json:"by"`
	At      string `json:"at"`
}

func (s *Store) Gone() []Gone {
	seen := map[string]bool{}
	var out []Gone
	s.backLines(func(line []byte) bool {
		k := lineKey(line)
		if k == "" || seen[k] || !slices.ContainsFunc(backMarks, func(m []byte) bool { return bytes.Contains(line, m) }) {
			return true
		}
		var e Event
		if json.Unmarshal(line, &e) != nil || e.Worktree != s.Root {
			return true
		}
		seen[k] = true
		if e.What == "removed" && e.Copy != nil {
			out = append(out, Gone{Key: e.Key, Title: e.Title, Section: e.Copy.Section, By: e.By, At: e.At})
		}
		return true
	})
	for _, g := range s.gitGone() {
		if !seen[strings.ToUpper(g.Key)] {
			out = append(out, g.Gone)
		}
	}
	return out
}

func (s *Store) Search(l List, q string) List {
	l = l.Find(q)
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return l
	}
	for _, g := range s.Gone() {
		hay := strings.ToLower(g.Key + " " + g.Title)
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			l.Deleted = append(l.Deleted, g)
		}
	}
	return l
}

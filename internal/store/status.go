package store

import (
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

var stageWords = map[string][]string{
	"done":  {"done", "complete", "completed", "closed", "shipped", "finished", "fixed"},
	"doing": {"doing", "in progress", "in-progress", "wip", "started", "active", "working"},
	"todo":  {"todo", "to do", "to-do", "not started", "open", "backlog", "next", "ready"},
}

func stage(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	for st, words := range stageWords {
		for _, w := range words {
			if v == w {
				return st
			}
		}
	}
	return ""
}

func statusWord(f *board.File, st string, s *Store) string {
	words := FieldsInUse(f)["status"]
	for _, v := range s.SavedViews() {
		if v.Group == "status" {
			words = append(words, v.Order...)
		}
	}
	for _, v := range words {
		if stage(v) == st {
			return v
		}
	}
	return st
}

func usesStatus(f *board.File) bool { return len(FieldsInUse(f)["status"]) > 0 }

func (s *Store) statusAfterTick(f *board.File, t *board.Task, done bool) string {
	cur := t.Meta("status")
	switch {
	case cur == "":
		return ""
	case done && stage(cur) != "done":
		return statusWord(f, "done", s)
	case !done && stage(cur) == "done":
		return statusWord(f, "todo", s)
	}
	return ""
}

func setStatus(f *board.File, key, status string) (*board.Task, error) {
	return f.SetFields(key, "", [][2]string{{"status", status}})
}

func (s *Store) statusOnClaim(key string, claimed bool, a Actor) {
	k, ok := s.KindOf(key)
	if !ok || (k != Backlog && k != Requests) {
		return
	}
	var t *board.Task
	var to string
	s.UpdateBy(k, a, func(f *board.File) error {
		it := f.Find(key)
		if it == nil || it.Done {
			return nil
		}
		cur := it.Meta("status")
		switch {
		case claimed && (stage(cur) == "todo" || cur == "" && usesStatus(f)):
			to = statusWord(f, "doing", s)
		case !claimed && stage(cur) == "doing":
			to = statusWord(f, "todo", s)
		default:
			return nil
		}
		var err error
		t, err = setStatus(f, it.Key, to)
		return err
	})
	if t != nil {
		s.Log(Event{By: a.By, What: "set", Key: t.Key, Title: t.Title, Detail: "status: " + to, Session: a.Session})
	}
}

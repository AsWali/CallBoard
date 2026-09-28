package store

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type seenState struct {
	Reflog int64             `json:"reflog"`
	Stash  int64             `json:"stash"`
	Files  map[string]string `json:"files"`
}

func (s *Store) seenPath() string {
	h := fnv.New32a()
	h.Write([]byte(s.Root))
	return filepath.Join(s.Shared(), "seen", fmt.Sprintf("%08x.json", h.Sum32()))
}

func (s *Store) readSeen() (seenState, bool) {
	var st seenState
	b, err := os.ReadFile(s.seenPath())
	if err != nil || json.Unmarshal(b, &st) != nil || st.Files == nil {
		return seenState{Files: map[string]string{}}, false
	}
	return st, true
}

func (s *Store) writeSeen(st seenState) {
	b, _ := json.Marshal(st)
	os.MkdirAll(filepath.Dir(s.seenPath()), 0o755)
	writeAtomic(s.seenPath(), string(b))
}

func (s *Store) noteSeen(k Kind, before, text string) {
	st, ok := s.readSeen()
	if !ok || st.Files[k.File] != before {
		return
	}
	st.Files[k.File] = text
	s.writeSeen(st)
}

func (s *Store) moveSeen(from, to string) {
	st, ok := s.readSeen()
	if !ok {
		return
	}
	if _, had := st.Files[from]; had && to != "" {
		b, _ := os.ReadFile(filepath.Join(s.Root, to))
		st.Files[to] = string(b)
	}
	delete(st.Files, from)
	s.writeSeen(st)
}

func (s *Store) Reconcile() []Event {
	var evs []Event
	s.Locked(func() error { evs = s.reconcile(); return nil })
	return evs
}

func (s *Store) reconcile() []Event {
	st, had := s.readSeen()
	texts := map[string]string{}
	for _, k := range s.Lists() {
		b, err := os.ReadFile(s.File(k))
		if err != nil && !os.IsNotExist(err) {
			return nil
		}
		texts[k.File] = string(b)
	}
	changed := false
	for f, t := range texts {
		if st.Files[f] != t {
			changed = true
		}
	}
	end, stash := s.logSize(s.reflogPath()), s.logSize(s.stashLogPath())
	if !had || !changed {
		if !had || end != st.Reflog || stash != st.Stash {
			s.writeSeen(seenState{Reflog: end, Stash: stash, Files: texts})
		}
		return nil
	}

	for _, t := range texts {
		if len(board.Parse(t).Conflicts()) > 0 {
			return nil
		}
	}
	by, via := "someone", ""
	steps := s.gitSteps(st.Reflog, end)
	if stash != st.Stash {
		steps = append(slices.DeleteFunc(steps, func(x string) bool { return x == "reset to HEAD" }), "stash")
	}
	if len(steps) > 0 {
		by, via = "git", strings.Join(steps, ", then ")
	}
	var evs []Event
	for _, k := range s.Lists() {
		if st.Files[k.File] == texts[k.File] {
			continue
		}
		v := via
		if by == "someone" {
			v = "edited " + k.File
		}
		for _, e := range DiffLists(k, board.ParseAs(st.Files[k.File], k.Prefix), board.ParseAs(texts[k.File], k.Prefix)) {
			e.By, e.Via = by, v
			s.Log(e)
			evs = append(evs, e)
		}
	}
	s.writeSeen(seenState{Reflog: end, Stash: stash, Files: texts})
	return evs
}

func (s *Store) reflogPath() string { return filepath.Join(s.GitDir, "logs", "HEAD") }

func (s *Store) stashLogPath() string { return filepath.Join(s.Common, "logs", "refs", "stash") }

func (s *Store) logSize(p string) int64 {
	if s.GitDir == "" {
		return 0
	}
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func (s *Store) gitSteps(from, to int64) []string {
	if to <= from && to != 0 && from != 0 {
		return nil
	}
	b, err := os.ReadFile(s.reflogPath())
	if err != nil {
		return nil
	}
	if from > int64(len(b)) || from < 0 {
		from = 0
	}
	moved := false
	var steps []string
	for _, l := range strings.Split(strings.TrimSpace(string(b[from:])), "\n") {
		_, subj, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		moved = true
		if w := stepWords(subj); w != "" && !slices.Contains(steps, w) {
			steps = append(steps, w)
		}
	}
	if !moved {
		return nil
	}
	if len(steps) == 0 {
		return nil
	}
	if len(steps) > 3 {
		steps = append(steps[:3], "more")
	}
	return steps
}

var mergeMsg = regexp.MustCompile(`^Merge (?:remote-tracking )?branch '([^']+)'`)

func stepWords(subj string) string {
	what, rest, _ := strings.Cut(subj, ": ")
	switch {
	case what == "commit" || what == "commit (amend)" || what == "commit (initial)":
		return ""
	case what == "commit (merge)":
		if m := mergeMsg.FindStringSubmatch(rest); m != nil {
			return "merge " + m[1]
		}
		return "a merge"
	case strings.HasPrefix(what, "merge "):
		return what
	case what == "checkout":
		if from, to, ok := strings.Cut(strings.TrimPrefix(rest, "moving from "), " to "); ok {
			return "switching from " + from + " to " + to
		}
		return "a checkout"
	case what == "reset":
		return "reset to " + strings.TrimPrefix(rest, "moving to ")
	case what == "cherry-pick":
		return "cherry-pick “" + rest + "”"
	case what == "revert":
		return "revert “" + strings.Trim(strings.TrimPrefix(rest, "Revert "), "\"") + "”"
	case strings.HasPrefix(what, "pull"):
		if strings.Contains(what, "--rebase") || strings.Contains(what, " -r") {
			return "pull --rebase"
		}
		return "pull"
	case strings.HasPrefix(what, "rebase"):
		return "rebase"
	}
	return what
}

func DiffLists(k Kind, a, b *board.File) []Event {
	var evs []Event
	for _, t := range b.Tasks {
		if t.Key == "" {
			continue
		}
		e := Event{Key: t.Key, Title: t.Title}
		o := a.Find(t.Key)
		if o == nil {
			e.What = "added"
			evs = append(evs, e)
			continue
		}
		skip := map[string]bool{}
		if t.Done != o.Done {
			switch {
			case k == Questions && t.Done:
				e.What, e.Detail = "answered", t.Meta("answer")
				if n := t.Assumed; e.Detail == "" && n > 0 {
					e.Detail = fmt.Sprint("option ", n)
				}
			case t.Done:
				e.What = "ticked"
			default:
				e.What = "reopened"
			}
			evs = append(evs, e)
			for _, f := range []string{"status", "answer", "why", "answered-by"} {
				skip[f] = true
			}
		}
		if t.Title != o.Title {
			evs = append(evs, Event{What: "renamed", Key: t.Key, Title: t.Title, Detail: o.Title})
		}
		if t.Section != o.Section {
			evs = append(evs, Event{What: "moved", Key: t.Key, Title: t.Title, Detail: t.Section})
		}
		if !slices.Equal(t.Needs, o.Needs) {
			d := strings.Join(t.Needs, ", ")
			if d == "" {
				d = "nothing"
			}
			evs = append(evs, Event{What: "needs", Key: t.Key, Title: t.Title, Detail: d})
		}
		var set []string
		seen := map[string]bool{}
		for _, x := range []*board.Task{t, o} {
			for _, f := range x.Fields() {
				n := f[0]
				if seen[n] || skip[n] || t.Meta(n) == o.Meta(n) {
					continue
				}
				seen[n] = true
				if v := t.Meta(n); v != "" {
					set = append(set, n+": "+v)
				} else {
					set = append(set, "no "+n)
				}
			}
		}
		if len(set) > 0 {
			evs = append(evs, Event{What: "set", Key: t.Key, Title: t.Title, Detail: strings.Join(set, ", ")})
		}
	}
	for _, t := range a.Tasks {
		if t.Key != "" && b.Find(t.Key) == nil {
			r := a.Snapshot(t)
			evs = append(evs, Event{What: "removed", Key: t.Key, Title: t.Title, Detail: t.Section, Copy: &r})
		}
	}
	return evs
}

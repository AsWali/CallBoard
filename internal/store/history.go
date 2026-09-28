package store

import (
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type pastGone struct {
	Gone
	commit, file string
}

func (s *Store) gitGone() []pastGone {
	if s == nil || s.Common == "" {
		return nil
	}
	args := []string{"log", "-n", "500", "-p", "-U0", "--no-color", "--no-renames", "--no-ext-diff", "--format=@@@ %H%x09%an%x09%aI", "--"}
	for _, k := range s.Lists() {
		args = append(args, k.File)
	}
	out, err := s.Git(args...)
	if err != nil {
		return nil
	}
	present := map[string]bool{}
	if files, err := s.LoadAll(); err == nil {
		for _, f := range files {
			for _, t := range f.Tasks {
				present[strings.ToUpper(t.Key)] = true
			}
		}
	}
	seen := map[string]bool{}
	var res []pastGone
	var commit, by, at, file string
	minus := map[string]pastGone{}
	var order []string
	plus := map[string]bool{}
	flush := func() {
		for _, k := range order {
			if !plus[k] && !seen[k] && !present[k] {
				g := minus[k]
				g.commit, g.By, g.At = commit, by, at
				res = append(res, g)
			}
		}
		for k := range minus {
			seen[k] = true
		}
		for k := range plus {
			seen[k] = true
		}
		minus, plus, order = map[string]pastGone{}, map[string]bool{}, nil
	}
	for _, l := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(l, "@@@ "):
			flush()
			parts := strings.SplitN(l[4:], "\t", 3)
			if len(parts) == 3 {
				commit, by, at = parts[0], parts[1], parts[2]
			}
		case strings.HasPrefix(l, "--- "):
			file = strings.TrimPrefix(strings.TrimPrefix(l, "--- "), "a/")
		case strings.HasPrefix(l, "+++ "):
		case strings.HasPrefix(l, "-"):
			if t := board.ParseLine(l[1:]); t != nil && t.Key != "" {
				k := strings.ToUpper(t.Key)
				if _, ok := minus[k]; !ok {
					order = append(order, k)
				}
				minus[k] = pastGone{Gone: Gone{Key: t.Key, Title: t.Title}, file: file}
			}
		case strings.HasPrefix(l, "+"):
			if t := board.ParseLine(l[1:]); t != nil && t.Key != "" {
				plus[strings.ToUpper(t.Key)] = true
			}
		}
	}
	flush()
	return res
}

func (s *Store) fromHistory(key string) (*board.Removed, Kind, bool) {
	k, ok := s.KindOf(key)
	if !ok {
		return nil, Kind{}, false
	}
	cut := func(rev, file string, k Kind) (*board.Removed, bool) {
		text, err := s.Git("show", rev+":"+file)
		if err != nil || !holdsKey(text, key) {
			return nil, false
		}
		f := board.ParseAs(text, k.Prefix)
		_, r, err := f.Cut(key, "")
		if err != nil {
			return nil, false
		}
		return &r, true
	}
	if r, ok := cut("HEAD", k.File, k); ok {
		return r, k, true
	}
	for _, g := range s.gitGone() {
		if strings.EqualFold(g.Key, key) {
			if r, ok := cut(g.commit+"^", g.file, k); ok {
				return r, k, true
			}
		}
	}
	return nil, Kind{}, false
}

func (s *Store) fillDoneBy(items []Item) {
	var finishers map[string]string
	blamed := map[string]map[string]string{}
	for i, it := range items {
		if !it.Done || it.DoneBy != "" || it.Kind == Questions.Noun {
			continue
		}
		if finishers == nil {
			finishers = s.Finishers()
		}
		if by := finishers[strings.ToUpper(it.Key)]; by != "" && by != "someone" && by != "git" {
			items[i].DoneBy = by
			continue
		}
		k, ok := s.KindOf(it.Key)
		if !ok {
			continue
		}
		if blamed[k.File] == nil {
			blamed[k.File] = s.blame(k.File)
		}
		items[i].DoneBy = blamed[k.File][strings.ToUpper(it.Key)]
	}
}

func (s *Store) DoneBy(t *board.Task) string {
	if t.DoneBy != "" || !t.Done {
		return t.DoneBy
	}
	k, _ := s.KindOf(t.Key)
	items := []Item{{Key: t.Key, Kind: k.Noun, Done: true}}
	s.fillDoneBy(items)
	return items[0].DoneBy
}

func (s *Store) blame(file string) map[string]string {
	out := map[string]string{}
	if s.Common == "" {
		return out
	}
	text, err := s.Git("blame", "--line-porcelain", "--", file)
	if err != nil {
		return out
	}
	author, committed := "", false
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "\t"):
			if t := board.ParseLine(l[1:]); t != nil && t.Key != "" && t.Done && committed && author != "" {
				out[strings.ToUpper(t.Key)] = author
			}
			author, committed = "", false
		case strings.HasPrefix(l, "author "):
			author = strings.TrimPrefix(l, "author ")
		case len(l) > 40 && strings.Count(l[:40], "0") != 40 && !strings.Contains(l[:40], " ") && strings.Count(l, " ") >= 2:
			committed = true
		}
	}
	return out
}

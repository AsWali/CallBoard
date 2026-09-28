package store

import (
	"fmt"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type Merge struct {
	Key      string    `json:"key"`
	Kind     string    `json:"kind"`
	File     string    `json:"file"`
	Versions [2]Merged `json:"versions"`
}

type Merged struct {
	Label string `json:"label"`
	Title string `json:"title"`
	Done  bool   `json:"done"`

	Differs string `json:"differs,omitempty"`
}

func MergesIn(k Kind, f *board.File) []Merge {
	var out []Merge
	for _, c := range f.Conflicts() {
		for _, key := range c.Keys() {
			m := Merge{Key: key, Kind: k.Noun, File: k.File}
			var sides [2]*board.Task
			for i, side := range []int{1, 2} {
				label := c.OursLabel
				if side == 2 {
					label = c.TheirsLabel
				}
				m.Versions[i].Label = label
				m.Versions[i].Title = "(not there)"
				for _, t := range c.Side(side) {
					if strings.EqualFold(t.Key, key) {
						m.Versions[i].Title, m.Versions[i].Done = t.Title, t.Done
						sides[i] = t
					}
				}
			}
			if sides[0] != nil && sides[1] != nil && sides[0].Title == sides[1].Title {
				m.Versions[0].Differs, m.Versions[1].Differs = differs(sides[0], sides[1]), differs(sides[1], sides[0])
			}
			out = append(out, m)
		}
	}
	return out
}

func differs(a, b *board.Task) string {
	var out []string
	if a.Done != b.Done {
		out = append(out, map[bool]string{true: "done", false: "open"}[a.Done])
	}
	seen := map[string]bool{}
	for _, t := range []*board.Task{a, b} {
		for _, f := range t.Fields() {
			if seen[f[0]] || a.Meta(f[0]) == b.Meta(f[0]) {
				continue
			}
			seen[f[0]] = true
			if v := a.Meta(f[0]); v != "" {
				out = append(out, f[0]+" "+v)
			} else {
				out = append(out, "no "+f[0])
			}
		}
	}
	if strings.Join(a.Body, "\n") != strings.Join(b.Body, "\n") {
		out = append(out, "its own notes")
	}
	return strings.Join(out, ", ")
}

func (v *View) Merges() []Merge {
	var out []Merge
	for _, k := range v.s.Lists() {
		out = append(out, MergesIn(k, v.Files[k.Name])...)
	}
	return out
}

func (s *Store) Resolve(key string, side int, a Actor) ([]*board.Task, error) {
	key, _ = splitKey(key)
	k, err := s.kindFor(key)
	if err != nil {
		return nil, err
	}
	var kept []*board.Task
	_, err = s.UpdateBy(k, a, func(f *board.File) (err error) {
		kept, err = f.Resolve(key, side)
		return err
	})
	if err == nil {
		title := ""
		for _, t := range kept {
			if strings.EqualFold(t.Key, key) {
				title = t.Title
			}
		}
		s.Log(Event{By: a.By, What: "resolved", Key: key, Title: title, Detail: fmt.Sprint("kept version ", side), Session: a.Session})
	}
	return kept, err
}

func FormatMerges(ms []Merge) string {
	if len(ms) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Unfinished merges (two branches changed these differently):\n")
	for _, m := range ms {
		side := func(v Merged) string {
			if v.Differs != "" {
				return fmt.Sprintf("%q with %s (%s)", v.Title, v.Differs, v.Label)
			}
			return fmt.Sprintf("%q (%s)", v.Title, v.Label)
		}
		fmt.Fprintf(&b, "  %s in %s: 1 = %s, 2 = %s\n", m.Key, m.File, side(m.Versions[0]), side(m.Versions[1]))
	}
	b.WriteString("Keep one with callboard resolve KEY 1|2 (or the resolve tool). If the choice matters, ask the human.\n")
	return b.String()
}

func inMerge(ms []Merge, key string) bool {
	for _, m := range ms {
		if key != "" && strings.EqualFold(m.Key, key) {
			return true
		}
	}
	return false
}

func InMerge(ms []Merge, key string) bool { return inMerge(ms, key) }

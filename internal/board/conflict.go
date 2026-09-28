package board

import (
	"fmt"
	"strings"
)

type Conflict struct {
	Start, End  int
	Mid         int
	Ours        []string
	Theirs      []string
	OursLabel   string
	TheirsLabel string
}

func (c Conflict) Keys() []string {
	var keys []string
	seen := map[string]bool{}
	for _, side := range [][]string{c.Ours, c.Theirs} {
		for _, t := range Parse(strings.Join(side, "\n")).Tasks {
			if u := strings.ToUpper(t.Key); t.Key != "" && !seen[u] {
				seen[u] = true
				keys = append(keys, t.Key)
			}
		}
	}
	return keys
}

func (c Conflict) Side(n int) []*Task {
	lines := c.Ours
	if n == 2 {
		lines = c.Theirs
	}
	return Parse(strings.Join(lines, "\n")).Tasks
}

func (f *File) Conflicts() []Conflict {
	var out []Conflict
	var cur *Conflict
	part := 0
	for i, l := range f.Lines {
		switch {
		case strings.HasPrefix(l, "<<<<<<<") && cur == nil:
			cur = &Conflict{Start: i, OursLabel: strings.TrimSpace(l[7:])}
			part = 1
		case cur != nil && strings.HasPrefix(l, "|||||||") && part == 1:
			part = 2
		case cur != nil && l == "=======" && part < 3:
			part, cur.Mid = 3, i
		case cur != nil && strings.HasPrefix(l, ">>>>>>>") && part == 3:
			cur.End, cur.TheirsLabel = i+1, strings.TrimSpace(l[7:])
			out = append(out, *cur)
			cur = nil
		case cur != nil && part == 1:
			cur.Ours = append(cur.Ours, l)
		case cur != nil && part == 3:
			cur.Theirs = append(cur.Theirs, l)
		}
	}
	return out
}

func (f *File) conflictOn(key string) (Conflict, bool) {
	for _, c := range f.Conflicts() {
		for _, k := range c.Keys() {
			if strings.EqualFold(k, key) {
				return c, true
			}
		}
	}
	return Conflict{}, false
}

type ErrConflict struct{ Key string }

func (e ErrConflict) Error() string {
	return fmt.Sprintf("%s is in an unfinished merge; keep one version first: callboard resolve %s 1 (or 2)", e.Key, e.Key)
}

func (f *File) Resolve(key string, side int) ([]*Task, error) {
	if side != 1 && side != 2 {
		return nil, fmt.Errorf("keep 1 (the first version) or 2 (the second), not %d", side)
	}
	key = strings.SplitN(strings.TrimSpace(key), "@", 2)[0]
	c, ok := f.conflictOn(key)
	if !ok {
		return nil, fmt.Errorf("%s isn't in an unfinished merge", key)
	}
	keep := c.Ours
	if side == 2 {
		keep = c.Theirs
	}
	lines := append([]string{}, f.Lines[:c.Start]...)
	lines = append(lines, keep...)
	f.Lines = append(lines, f.Lines[c.End:]...)
	f.index()
	return c.Side(side), nil
}

func inTheirs(cs []Conflict, i int) bool {
	for _, c := range cs {
		if i > c.Mid && i < c.End {
			return true
		}
	}
	return false
}

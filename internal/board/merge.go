package board

import (
	"slices"
	"strings"
)

func (t *Task) Block() []string { return append([]string{t.Raw}, t.Body[:t.head]...) }

func Merge3(base, ours, theirs *Task) (lines []string, ok bool) {
	return merge3(base, ours, theirs, false)
}

func Merge3Sides(base, ours, theirs *Task) (o, t []string) {
	o, _ = merge3(base, ours, theirs, false)
	t, _ = merge3(base, ours, theirs, true)
	return o, t
}

func merge3(base, ours, theirs *Task, theirsWins bool) (lines []string, ok bool) {
	pick3 := func(b, o, t string) (string, bool) {
		v, ok := pick3(b, o, t)
		if !ok && theirsWins {
			v = t
		}
		return v, ok
	}
	ob, tb, bb := strings.Join(ours.Block(), "\n"), strings.Join(theirs.Block(), "\n"), strings.Join(base.Block(), "\n")
	switch {
	case ob == tb, tb == bb:
		return ours.Block(), true
	case ob == bb:
		return theirs.Block(), true
	}
	m := *ours
	m.extra = slices.Clone(ours.extra)

	title, ok1 := pick3(base.Title, ours.Title, theirs.Title)
	m.Title = title

	if ours.Done == base.Done && theirs.Done != base.Done {
		m.Done, m.DoneAt = theirs.Done, theirs.DoneAt
	}
	ok3 := true
	switch {
	case ours.Assumed == base.Assumed:
		m.Assumed = theirs.Assumed
	case theirs.Assumed != base.Assumed && theirs.Assumed != ours.Assumed:
		ok3 = false
		if theirsWins {
			m.Assumed = theirs.Assumed
		}
	}
	m.Needs = mergeSet(base.Needs, ours.Needs, theirs.Needs)

	var names []string
	for _, t := range []*Task{base, ours, theirs} {
		for _, e := range t.extra {
			if !slices.Contains(names, e[0]) {
				names = append(names, e[0])
			}
		}
	}
	for _, f := range names {
		v, ok := pick3(base.Meta(f), ours.Meta(f), theirs.Meta(f))
		if !ok {
			switch {
			case f == "status" && ours.Done != theirs.Done && m.Done == theirs.Done:
				v, ok = theirs.Meta(f), true
			case f == "status" && ours.Done != theirs.Done:
				ok = true
			}
		}
		ok3 = ok3 && ok
		m.SetMeta(f, v)
	}

	body, ok2 := mergeBody(base.Body[:base.head], ours.Body[:ours.head], theirs.Body[:theirs.head])
	if !ok2 && theirsWins {
		body = theirs.Body[:theirs.head]
	}
	return append([]string{m.Format()}, body...), ok1 && ok2 && ok3
}

func pick3(b, o, t string) (string, bool) {
	switch {
	case o == t, t == b:
		return o, true
	case o == b:
		return t, true
	}
	return o, false
}

func mergeSet(b, o, t []string) []string {
	has := func(list []string, x string) bool {
		return slices.ContainsFunc(list, func(y string) bool { return strings.EqualFold(x, y) })
	}
	var out []string
	for _, x := range append(slices.Clone(o), t...) {
		removed := has(b, x) && (!has(o, x) || !has(t, x))
		if !removed && !has(out, x) {
			out = append(out, x)
		}
	}
	return out
}

func mergeBody(b, o, t []string) ([]string, bool) {
	switch {
	case slices.Equal(o, t), slices.Equal(t, b):
		return o, true
	case slices.Equal(o, b):
		return t, true
	}
	if len(o) >= len(b) && len(t) >= len(b) && slices.Equal(o[:len(b)], b) && slices.Equal(t[:len(b)], b) {
		out := slices.Clone(o)
		for _, l := range t[len(b):] {
			if !slices.Contains(out[len(b):], l) {
				out = append(out, l)
			}
		}
		return out, true
	}
	return o, false
}

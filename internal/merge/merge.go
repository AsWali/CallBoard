package merge

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

const Rules = 2

type Result struct {
	Text      string
	Conflicts int
}

type unit struct {
	id   string
	text string
	task *board.Task
}

func split(s, prefix string) []unit {
	f := board.ParseAs(s, prefix)
	byLine := map[int]*board.Task{}
	for _, t := range f.Tasks {
		if t.Key != "" {
			byLine[t.Line-1] = t
		}
	}
	var out []unit
	seen := map[string]int{}
	for i := 0; i < len(f.Lines); i++ {
		u := unit{id: "t:" + f.Lines[i], text: f.Lines[i]}
		if t, ok := byLine[i]; ok {
			b := t.Block()
			u = unit{id: "k:" + strings.ToLower(t.Key), text: strings.Join(b, "\n"), task: t}
			i += len(b) - 1
		}

		seen[u.id]++
		if n := seen[u.id]; n > 1 {
			u.id += "\x00" + strconv.Itoa(n)
		}
		out = append(out, u)
	}
	return out
}

func index(us []unit) map[string]int {
	m := make(map[string]int, len(us))
	for i, u := range us {
		m[u.id] = i
	}
	return m
}

func Merge(base, ours, theirs string, labels ...string) Result {
	return MergeAs("B-", base, ours, theirs, labels...)
}

func PrefixFor(path string) string {
	switch strings.ToLower(filepath.Base(path)) {
	case "questions.md":
		return "Q-"
	case "views.md":
		return "V-"
	case "requests.md":
		return "R-"
	}
	return "B-"
}

func MergeAs(prefix, base, ours, theirs string, labels ...string) Result {
	lo, lt := "ours", "theirs"
	if len(labels) > 0 && labels[0] != "" {
		lo = labels[0]
	}
	if len(labels) > 1 && labels[1] != "" {
		lt = labels[1]
	}
	B, O, T := split(base, prefix), split(ours, prefix), split(theirs, prefix)
	bi, oi, ti := index(B), index(O), index(T)
	get := func(us []unit, ix map[string]int, id string) (unit, bool) {
		if i, ok := ix[id]; ok {
			return us[i], true
		}
		return unit{}, false
	}
	isItem := func(id string) bool { return strings.HasPrefix(id, "k:") }

	var res []string
	conflicts := 0
	conflict := func(o, t string) {
		res = append(res, marker(o, t, lo, lt))
		conflicts++
	}
	emit := func(id string, o unit, oHas bool) {
		b, bHas := get(B, bi, id)
		t, tHas := get(T, ti, id)
		switch {
		case !bHas && oHas && !tHas:
			res = append(res, o.text)
		case !bHas && !oHas && tHas:
			res = append(res, t.text)
		case !bHas:
			if o.text == t.text || !isItem(id) {
				res = append(res, o.text)
			} else {
				conflict(o.text, t.text)
			}
		case oHas && tHas:
			switch {
			case o.text == t.text, t.text == b.text:
				res = append(res, o.text)
			case o.text == b.text:
				res = append(res, t.text)
			case isItem(id):
				if lines, ok := board.Merge3(b.task, o.task, t.task); ok {
					res = append(res, strings.Join(lines, "\n"))
				} else {
					ol, tl := board.Merge3Sides(b.task, o.task, t.task)
					conflict(strings.Join(ol, "\n"), strings.Join(tl, "\n"))
				}
			default:
				res = append(res, o.text)
			}
		case oHas && !tHas:
			if o.text != b.text && isItem(id) {
				conflict(o.text, "")
			}
		case !oHas && tHas:
			if t.text != b.text && isItem(id) {
				conflict("", t.text)
			}
		}
	}

	moved := map[string]bool{}
	for _, o := range O {
		b, bHas := get(B, bi, o.id)
		t, tHas := get(T, ti, o.id)
		if o.task != nil && bHas && tHas && b.task != nil && t.task != nil &&
			o.task.Section == b.task.Section && t.task.Section != b.task.Section {
			moved[o.id] = true
		}
	}

	elsewhere := func(u unit) bool {
		o, ok := get(O, oi, u.id)
		return ok && !moved[u.id] && u.task != nil && o.task != nil && o.task.Section != u.task.Section
	}
	after := map[string][]string{}
	for i, u := range T {
		if _, inO := oi[u.id]; !inO || moved[u.id] {
			prev := ""
			for j := i - 1; j >= 0; j-- {
				if !elsewhere(T[j]) {
					prev = T[j].id
					break
				}
			}
			after[prev] = append(after[prev], u.id)
		}
	}
	placed := map[string]bool{}
	var placeAfter func(id string)
	placeAfter = func(id string) {
		for _, tid := range after[id] {
			if placed[tid] {
				continue
			}
			placed[tid] = true
			if moved[tid] {
				emit(tid, O[oi[tid]], true)
			} else {
				emit(tid, unit{}, false)
			}
			placeAfter(tid)
		}
	}

	oursNew := func(id string) bool {
		_, inB := bi[id]
		_, inT := ti[id]
		return !inB && !inT
	}
	pending := []string{""}
	for _, u := range O {
		if moved[u.id] {
			continue
		}
		if !oursNew(u.id) {
			for _, p := range pending {
				placeAfter(p)
			}
			pending = pending[:0]
		}
		emit(u.id, u, true)
		pending = append(pending, u.id)
	}
	for _, p := range pending {
		placeAfter(p)
	}

	for _, u := range T {
		if _, inO := oi[u.id]; (!inO || moved[u.id]) && !placed[u.id] {
			placed[u.id] = true
			if moved[u.id] {
				emit(u.id, O[oi[u.id]], true)
			} else {
				emit(u.id, unit{}, false)
			}
		}
	}

	text := strings.Join(res, "\n")
	if len(res) > 0 && (strings.HasSuffix(ours, "\n") || ours == "") {
		text += "\n"
	}
	return Result{Text: text, Conflicts: conflicts}
}

func marker(ours, theirs, lo, lt string) string {
	var b strings.Builder
	b.WriteString("<<<<<<< " + lo + "\n")
	if ours != "" {
		b.WriteString(ours + "\n")
	}
	b.WriteString("=======\n")
	if theirs != "" {
		b.WriteString(theirs + "\n")
	}
	b.WriteString(">>>>>>> " + lt)
	return b.String()
}

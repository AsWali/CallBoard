package store

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

type ViewCheck struct {
	Shows    string   `json:"shows"`
	Warnings []string `json:"warnings,omitempty"`
}

func NormVal(f, v string) string {
	x := strings.ToLower(strings.TrimSpace(v))
	switch f {
	case "done":
		switch x {
		case "done", "yes", "true", "x":
			return "done"
		case "open", "no", "false", "":
			return "open"
		}
	case "blocked":
		switch x {
		case "waiting", "yes", "true":
			return "waiting"
		case "ready", "no", "false", "":
			return "ready"
		}
	}
	return x
}

func ItemVal(it Item, f string) string {
	switch f {
	case "section":
		return it.Section
	case "done":
		if it.Done {
			return "done"
		}
		return "open"
	case "by":
		return it.By
	case "at":
		return prefix(it.At, 10)
	case "doneat":
		return prefix(it.DoneAt, 10)
	case "kind":
		return it.Kind
	case "blocked":
		if it.Blocked {
			return "waiting"
		}
		return "ready"
	case "claimed":
		if it.Claim != nil && !it.Done {
			return it.Claim.By
		}
		return ""
	case "title":
		return it.Title
	case "key":
		return it.Key
	case "needs":
		return strings.Join(it.WaitingOn, ",")
	}
	return it.Fields[f]
}

func prefix(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var FieldWords = map[string]string{
	"section": "the heading it's under", "done": "open or done", "blocked": "waiting or ready",
	"claimed": "who is working on it", "by": "who added it", "at": "the day it was added",
	"doneat": "the day it was done", "needs": "what it waits on", "title": "its title", "key": "its key", "kind": "task, request or question",
}

func (s *Store) CheckView(v BoardView) ViewCheck {
	k, err := s.KindNamed(v.List)
	if err != nil {
		return ViewCheck{Shows: "a view of an unknown list " + v.List}
	}
	vw, err := s.View()
	if err != nil {
		return ViewCheck{Shows: err.Error()}
	}
	var items []Item
	inUse := map[string]map[string]bool{}
	for _, t := range vw.Files[k.Name].Tasks {
		it := vw.Item(k, t)
		items = append(items, it)
		for f, x := range it.Fields {
			if inUse[f] == nil {
				inUse[f] = map[string]bool{}
			}
			inUse[f][strings.ToLower(x)] = true
		}
	}
	noun := k.Noun
	var fieldNames []string
	for f := range inUse {
		fieldNames = append(fieldNames, f)
	}
	sort.Strings(fieldNames)
	var warn []string
	known := func(f string) bool { _, b := FieldWords[f]; return b || inUse[f] != nil }
	checked := map[string]bool{}
	checkField := func(f, use string) bool {
		if known(f) {
			return true
		}
		if !checked[f] {
			checked[f] = true
			w := fmt.Sprintf("no %s has a %q field, so %s it does nothing", noun, f, use)
			if m := closest(f, fieldNames); m != "" {
				w += fmt.Sprintf("; did you mean %q?", m)
			}
			if len(fieldNames) > 0 {
				w += "; fields in use: " + strings.Join(fieldNames, ", ")
			}
			warn = append(warn, w+"; built in: section, done, blocked, claimed, by, at, doneat")
		}
		return false
	}

	valuesOf := func(f string) []string {
		seen := map[string]bool{}
		for _, it := range items {
			if x := NormVal(f, ItemVal(it, f)); x != "" {
				seen[x] = true
			}
		}
		var out []string
		for x := range seen {
			out = append(out, x)
		}
		sort.Strings(out)
		return out
	}
	missing := func(f, x string) {
		if x == "" || !known(f) || f == "done" || f == "blocked" || f == "title" {
			return
		}
		have := valuesOf(f)
		if slices.Contains(have, NormVal(f, x)) {
			return
		}
		w := fmt.Sprintf("no %s has %s=%s", noun, f, x)
		if len(have) > 0 {
			w += "; values in use: " + strings.Join(have, ", ")
		}
		warn = append(warn, w)
	}

	shown := items
	var only []string
	for _, fl := range v.Filter {
		f, not, vals, err := ParseFilter(fl)
		if err != nil {
			warn = append(warn, err.Error())
			continue
		}
		if !checkField(f, "filtering by") {
			continue
		}
		for _, x := range vals {
			missing(f, x)
		}
		var keep []Item
		for _, it := range shown {
			have := NormVal(f, ItemVal(it, f))
			hit := slices.ContainsFunc(vals, func(x string) bool { return NormVal(f, x) == have })
			if hit != not {
				keep = append(keep, it)
			}
		}
		shown = keep
		op := "is"
		if not {
			op = "is not"
		}
		only = append(only, fmt.Sprintf("%s %s %s", f, op, strings.Join(vals, " or ")))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s of %s: %d of %d %ss", strings.ToUpper(v.Layout[:1])+v.Layout[1:], k.File, len(shown), len(items), noun)
	if len(only) > 0 {
		b.WriteString(" (only where " + strings.Join(only, " and ") + ")")
	}
	group := v.Group
	if group == "" && v.Layout == "board" {
		group = "section"
	}
	if group != "" && checkField(group, "grouping by") {
		counts := map[string]int{}
		var order []string
		for _, o := range v.Order {
			missing(group, o)
			if _, ok := counts[strings.ToLower(o)]; !ok {
				counts[strings.ToLower(o)] = 0
				order = append(order, o)
			}
		}
		var rest []string
		for _, it := range shown {
			x := NormVal(group, ItemVal(it, group))
			if _, ok := counts[x]; !ok {
				rest = append(rest, x)
			}
			counts[x]++
		}
		sort.Strings(rest)
		var parts []string
		for _, g := range append(order, slices.Compact(rest)...) {
			name := g
			if name == "" {
				name = "no " + group
				if group == "claimed" {
					name = "nobody on it"
				}
			}
			parts = append(parts, fmt.Sprintf("%s %d", name, counts[strings.ToLower(g)]))
		}
		word := "grouped by"
		if v.Layout == "board" {
			word = "columns by"
		}
		fmt.Fprintf(&b, "; %s %s: %s", word, group, strings.Join(parts, ", "))
	}
	var sorts []string
	for _, x := range v.Sort {
		f := strings.TrimPrefix(x, "-")
		if checkField(f, "sorting by") {
			dir := "low to high"
			if strings.HasPrefix(x, "-") {
				dir = "high to low (most important or newest first)"
			}
			sorts = append(sorts, f+" "+dir)
		}
	}
	if len(sorts) > 0 {
		b.WriteString("; sorted by " + strings.Join(sorts, ", then "))
	}
	for _, f := range v.Show {
		checkField(f, "showing")
	}
	if len(v.Show) > 0 {
		b.WriteString("; showing " + strings.Join(v.Show, ", "))
	}
	if len(items) > 0 && len(shown) == 0 {
		warn = append(warn, "it shows nothing right now: no "+noun+" passes its filters")
	}
	return ViewCheck{Shows: b.String(), Warnings: warn}
}

func closest(f string, names []string) string {
	for _, n := range names {
		if strings.HasPrefix(f, n) || strings.HasPrefix(n, f) || strings.EqualFold(strings.TrimRight(f, "s"), n) {
			return n
		}
	}
	return ""
}

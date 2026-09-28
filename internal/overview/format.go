package overview

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/store"
)

func Format(p Branches) string {
	var b strings.Builder
	var inMain []string
	fmt.Fprintf(&b, "Branches against %s, in the lists only:\n", p.Main)
	for _, c := range p.Branches {
		if c.InMain {
			inMain = append(inMain, c.Name)
			continue
		}
		here := ""
		if c.Current {
			here = " (this worktree)"
		}
		if c.Fork == nil {
			fmt.Fprintf(&b, "%s%s: no history shared with %s\n", c.Name, here, p.Main)
		} else {
			fmt.Fprintf(&b, "%s%s: forked from %s %s, at %q\n", c.Name, here, p.Main, ago(c.Fork.At), c.Fork.Subject)
		}
		if s := Since(c); s != "" {
			fmt.Fprintf(&b, "  since then on %s: %s\n", p.Main, s)
		}
		fmt.Fprintf(&b, "  it changes %d item(s)\n", len(c.Brings))
		if len(c.Conflicts) == 0 {
			fmt.Fprintf(&b, "  merging into %s now: no conflicts in the lists\n", p.Main)
		} else {
			fmt.Fprintf(&b, "  merging into %s now: %s\n", p.Main, Clashes(c))
		}
	}
	if len(inMain) > 0 {
		fmt.Fprintf(&b, "Already in %s: %s\n", p.Main, strings.Join(inMain, ", "))
	}
	if len(p.Branches) == 0 {
		fmt.Fprintf(&b, "No branches besides %s.\n", p.Main)
	}
	return b.String()
}

func Since(c Card) string {
	n := len(c.Since) + c.Older
	if n == 0 {
		return ""
	}
	var steps []string
	for _, s := range c.Since {
		name := fmt.Sprintf("%q", s.Subject)
		if s.Merged != "" {
			name = "merge " + s.Merged
		}
		var its []string
		for _, it := range s.Items {
			x := it.What + " " + fmt.Sprintf("%q", it.Title)
			if it.Detail != "" && it.What != "renamed" {
				x += " (" + it.Detail + ")"
			}
			its = append(its, x)
		}
		steps = append(steps, name+": "+strings.Join(its, ", "))
	}
	more := ""
	if c.Older > 0 {
		more = fmt.Sprintf("; %d older", c.Older)
	}
	return fmt.Sprintf("%d change(s) to the lists (%s%s)", n, strings.Join(steps, "; "), more)
}

func Clashes(c Card) string {
	var xs []string
	for _, m := range c.Conflicts {
		side := func(v store.Merged) string {
			switch {
			case v.Title == "(not there)":
				return "deleted on " + v.Label
			case v.Differs != "":
				return v.Differs + " on " + v.Label
			}
			return fmt.Sprintf("%q on %s", v.Title, v.Label)
		}
		title := m.Versions[0].Title
		if title == "(not there)" {
			title = m.Versions[1].Title
		}
		xs = append(xs, fmt.Sprintf("%s %q (%s, %s)", m.Key, title, side(m.Versions[0]), side(m.Versions[1])))
	}
	return fmt.Sprintf("%d item(s) would conflict: %s", len(c.Conflicts), strings.Join(xs, "; "))
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

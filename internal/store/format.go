package store

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
)

func FormatItem(it Item) string {
	key := it.Key
	if key == "" {
		key = "(no key)"
	} else {
		key += "@" + it.Version
	}
	line := fmt.Sprintf("%-10s %s", key, it.Title)
	fields := it.Fields
	var notes []string
	if who := strings.TrimSpace(fields[ForField]); who != "" {
		fields = maps.Clone(fields)
		delete(fields, ForField)
		switch {
		case OnlyForYou(it):
			notes = append(notes, "for the human, not for agents")
		case Assigned(it):
			notes = append(notes, "for "+ForText(who))
		}
	}
	if fs := formatFields(fields); fs != "" {
		line += "  " + fs
	}
	if n := len(it.Subtasks); n > 0 {
		notes = append(notes, fmt.Sprintf("%d of %d subtasks done", n-it.SubsOpen, n))
	}
	if it.Done && it.Kind != "question" {
		d := "done"
		if it.DoneBy != "" {
			d += " by " + it.DoneBy
		}
		if day, _, _ := strings.Cut(it.DoneAt, "T"); day != "" {
			d += " on " + day
		}
		notes = append(notes, d)
	}
	if due := DueWords(it, time.Now()); due != "" {
		notes = append(notes, due)
	}
	if len(it.WaitingOn) > 0 && !it.Done {
		notes = append(notes, "waits on "+strings.Join(it.WaitingOn, ", "))
	}
	assumed := slices.IndexFunc(it.Options, func(o board.Option) bool { return o.Text == it.Assumed })
	if it.Assumed != "" && (assumed < 0 || it.Done) {
		notes = append(notes, "assumed: "+it.Assumed)
	}
	if it.Claim != nil && !it.Done {
		c := "claimed by " + it.Claim.By
		if it.Claim.Branch != "" {
			c += " on " + it.Claim.Branch
		}
		notes = append(notes, c)
	}
	for _, na := range it.Answers {
		notes = append(notes, na.Key+" answered: "+strconvQuote(na.Answer))
	}
	if len(notes) > 0 {
		line += "  (" + strings.Join(notes, "; ") + ")"
	}
	if !it.Done && len(it.Options) > 0 {
		var opts []string
		for i, o := range it.Options {
			x := fmt.Sprintf("%d) %s", i+1, o.Text)
			switch {
			case o.Recommended && i == assumed:
				x += " (recommended, assumed)"
			case o.Recommended:
				x += " (recommended)"
			case i == assumed:
				x += " (assumed)"
			}
			opts = append(opts, x)
		}
		line += "  options: " + strings.Join(opts, "  ")
	}
	return line
}

func formatFields(m map[string]string) string {
	names := make([]string, 0, len(m))
	for n, v := range m {
		if v != "" && !slices.Contains(questionMeta, n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var parts []string
	for _, n := range names {
		parts = append(parts, n+":"+board.Quote(m[n]))
	}
	return strings.Join(parts, " ")
}

var questionMeta = []string{"answer", "why", "answered-by", "was", "reopens"}

var importance = map[string]int{"urgent": 5, "critical": 5, "p0": 5, "high": 4, "p1": 4, "medium": 3, "med": 3, "normal": 3, "p2": 3, "low": 1, "p3": 1, "lowest": 0, "p4": 0}

func rank(t *board.Task) int {
	return rankOf(t.Meta("prio"), t.Meta("priority"))
}

func Importance(it Item) int {
	return rankOf(it.Fields["prio"], it.Fields["priority"])
}

func PriorityWord(x string) (int, bool) {
	r, ok := importance[strings.ToLower(x)]
	return r, ok
}

func rankOf(vals ...string) int {
	for _, x := range vals {
		if r, ok := importance[strings.ToLower(x)]; ok {
			return r
		}
	}
	return 2
}

func (v *View) NextUp(a Actor) *Item {
	f := v.Files[Backlog.Name]
	var best *board.Task
	bestMine := false
	merges := v.Merges()
	match := v.s.Matcher(a)
	for _, t := range f.Tasks {
		if t.Done || t.Key == "" || InMerge(merges, t.Key) {
			continue
		}
		it := v.Item(Backlog, t)
		mine, other := match(it.Fields[ForField])
		if it.Blocked || it.Claim != nil || other || a.By == "you" && Assigned(it) && !mine {
			continue
		}
		if best == nil || mine && !bestMine || mine == bestMine && rank(t) > rank(best) {
			best, bestMine = t, mine
		}
	}
	if best == nil {
		return nil
	}
	it := v.Item(Backlog, best)
	return &it
}

func FormatList(l List) string {
	var b strings.Builder
	head := l.Repo
	if l.Branch != "" {
		head += " · " + l.Branch
	}
	fmt.Fprintf(&b, "%s · %d open, %d done\n", head, l.Open, l.Done)
	kindNow, sec := "", "\x00"
	for _, it := range l.Items {
		if it.List != kindNow {
			fmt.Fprintf(&b, "%s\n", it.List+".md")
			kindNow, sec = it.List, "\x00"
		}
		if it.Section != sec && it.Section != "" {
			fmt.Fprintf(&b, "  ## %s\n", it.Section)
		}
		sec = it.Section
		box := " "
		if it.Done {
			box = "x"
		}
		pad := strings.Repeat("  ", it.Depth)
		fmt.Fprintf(&b, "  %s[%s] %s\n", pad, box, FormatItem(it))
		if ns := notes(it); !it.Done && len(ns) > 0 {
			first := []rune(ns[0])
			line := string(first)
			if len(first) > noteRunes {
				line = strings.TrimSpace(string(first[:noteRunes])) + "…"
			}
			if len(ns) > 1 {
				line += fmt.Sprintf(" (+%d more)", len(ns)-1)
			}
			b.WriteString("      " + pad + line + "\n")
		}
		if it.Answer != "" {
			why := ""
			if it.Why != "" {
				why = " (why: " + it.Why + ")"
			}
			fmt.Fprintf(&b, "               → %s%s\n", it.Answer, why)
		}
	}
	if len(l.Items) == 0 && len(l.Merges) == 0 && len(l.Deleted) == 0 {
		b.WriteString("  (nothing to show)\n")
	}
	if l.Total > 0 {
		shown := len(l.Items) + len(l.Deleted)
		end := l.From + shown
		switch {
		case shown == 0:
			fmt.Fprintf(&b, "There are only %d items; start from 0.\n", l.Total)
		case end < l.Total:
			fmt.Fprintf(&b, "Showing items %d–%d of %d. The next ones: from %d. Or narrow it with find, kind, section or ready.\n", l.From+1, end, l.Total, end)
		default:
			fmt.Fprintf(&b, "Showing items %d–%d of %d, the last ones.\n", l.From+1, end, l.Total)
		}
	}
	if len(l.Deleted) > 0 {
		b.WriteString("Deleted; edit's restore brings one back where it was (callboard restore KEY):\n")
		for _, g := range l.Deleted {
			where := ""
			if g.Section != "" {
				where = ", was under ## " + g.Section
			}
			day, _, _ := strings.Cut(g.At, "T")
			fmt.Fprintf(&b, "  %-10s %s  (deleted by %s on %s%s)\n", g.Key, g.Title, g.By, day, where)
		}
	}
	b.WriteString(FormatMerges(l.Merges))
	return b.String()
}

const noteRunes = 100

func notes(it Item) []string {
	var out []string
	for _, l := range it.Body {
		l = strings.TrimSpace(l)
		if l == "" || len(it.Options) > 0 && strings.HasPrefix(l, "- ") {
			continue
		}
		out = append(out, l)
	}
	return out
}

func (s *Store) Line(t *board.Task) string {
	k, _ := s.KindOf(t.Key)
	v, err := s.View()
	if err != nil {
		return t.Key + "@" + t.Version + " " + t.Title
	}
	line := FormatItem(v.Item(k, t))
	if t.Parent != "" {
		line += "  (subtask of " + t.Parent + ")"
	}
	if t.Done {
		line = "[x] " + line
	}
	return line
}

func (v BoardView) Describe() string {
	d := fmt.Sprintf("%s@%s  %s: %s of %s", v.Key, v.Version, v.Name, v.Layout, v.List)
	if v.Group != "" {
		d += ", grouped by " + v.Group
		if len(v.Order) > 0 {
			d += " (" + strings.Join(v.Order, ", ") + ")"
		}
	}
	if len(v.Sort) > 0 {
		d += ", sorted by " + strings.Join(v.Sort, ", ")
	}
	if len(v.Filter) > 0 {
		d += ", only " + strings.Join(v.Filter, " and ")
	}
	if len(v.Show) > 0 {
		d += ", showing " + strings.Join(v.Show, ", ")
	}
	if v.Suggested {
		d += fmt.Sprintf(" (suggested by %s, waiting for the human to keep or dismiss it: %s)", v.By, v.Why)
	}
	return d
}

func (s *Store) Checked(v BoardView) string {
	c := s.CheckView(v)
	out := v.Describe() + "\n  shows: " + c.Shows
	for _, w := range c.Warnings {
		out += "\n  warning: " + w
	}
	return out
}

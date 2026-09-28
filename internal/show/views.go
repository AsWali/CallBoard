package show

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/store"
)

const quietAfter = 7 * 24 * time.Hour

func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	for _, f := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (d *drawer) now() time.Time {
	if !d.Now.IsZero() {
		return d.Now
	}
	return time.Now()
}

func (d *drawer) ago(t time.Time) string {
	x := d.now().Sub(t)
	switch {
	case t.IsZero():
		return "never"
	case x < time.Minute:
		return "just now"
	case x < time.Hour:
		return plur(int(x.Minutes()), "minute") + " ago"
	case x < 24*time.Hour:
		return plur(int(x.Hours()), "hour") + " ago"
	}
	return plur(int(x.Hours()/24), "day") + " ago"
}

func plur(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

func (d *drawer) dayName(day string) string {
	t := parseTime(day)
	now := d.now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch {
	case t.IsZero():
		return day
	case !t.Before(today):
		return "Today"
	case !t.Before(today.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Year() == now.Year():
		return t.Format("Mon 2 Jan")
	}
	return t.Format("Mon 2 Jan 2006")
}

func (d *drawer) downstream() map[string][]string {
	if d.down != nil {
		return d.down
	}
	waiters := map[string][]string{}
	for _, it := range d.items {
		if it.Done {
			continue
		}
		for _, n := range it.WaitingOn {
			waiters[strings.ToUpper(n)] = append(waiters[strings.ToUpper(n)], strings.ToUpper(it.Key))
		}
	}
	out := map[string][]string{}
	for k := range waiters {
		seen := map[string]bool{k: true}
		var got []string
		queue := []string{k}
		for len(queue) > 0 {
			x := queue[0]
			queue = queue[1:]
			for _, w := range waiters[x] {
				if !seen[w] {
					seen[w] = true
					got = append(got, w)
					queue = append(queue, w)
				}
			}
		}
		out[k] = got
	}
	d.down = out
	return out
}

func (d *drawer) forYou() []store.Item {
	var its []store.Item
	for _, it := range d.items {
		if !it.Done && it.Kind != "task" {
			its = append(its, it)
		}
	}
	down := d.downstream()
	sort.SliceStable(its, func(i, j int) bool {
		a, b := len(down[strings.ToUpper(its[i].Key)]), len(down[strings.ToUpper(its[j].Key)])
		if a != b {
			return a > b
		}
		return its[i].Blocked != its[j].Blocked && !its[i].Blocked
	})
	return its
}

func (d *drawer) unblocksText(keys []string) string {
	if len(keys) == 0 {
		return ""
	}
	return fmt.Sprintf("unblocks %d", len(keys))
}

func recommended(it store.Item) string {
	for _, o := range it.Options {
		if o.Recommended {
			return o.Text
		}
	}
	return ""
}

func (d *drawer) waitingOnYou() {
	its := d.forYou()
	d.line(d.c(bold, "Waiting on you"), d.c(faint, "  your requests and questions, the ones that free most first"))
	d.line()
	if len(its) == 0 {
		d.line("Nothing. No open requests or questions.")
		return
	}
	down := d.downstream()
	for _, it := range its {
		keys := down[strings.ToUpper(it.Key)]
		d.row("  ", it, d.unblocksText(keys))
		lead := "           "
		if r := recommended(it); r != "" && it.Assumed == "" {
			d.long(lead, "recommended: "+plainText(r))
		}
		if len(keys) > 0 {
			var names []string
			for _, k := range keys {
				names = append(names, d.byKey[k].Key+" "+d.byKey[k].Title)
			}
			d.long(lead, "frees "+strings.Join(names, " · "))
		}
	}
	d.line()
	d.line(d.c(faint, "Answer a question: callboard answer Q-KEY \"your answer\" · mark a request done: callboard tick R-KEY"))
}

func (d *drawer) claimGroups() (names []string, groups map[string][]store.Item) {
	groups = map[string][]store.Item{}
	for _, it := range d.items {
		if it.Done || it.Claim == nil {
			continue
		}
		n := d.who(it.Claim) + " on " + orDetached(it.Claim.Branch)
		if _, ok := groups[n]; !ok {
			names = append(names, n)
		}
		groups[n] = append(groups[n], it)
	}
	sort.Strings(names)
	return names, groups
}

func (d *drawer) readyTasks() (now, later []store.Item) {
	for _, it := range d.items {
		if it.Kind != "task" || it.Done || it.Blocked || it.Claim != nil {
			continue
		}
		if strings.EqualFold(it.Section, "later") {
			later = append(later, it)
		} else {
			now = append(now, it)
		}
	}
	sort.SliceStable(now, func(i, j int) bool { return store.Importance(now[i]) > store.Importance(now[j]) })
	sort.SliceStable(later, func(i, j int) bool { return store.Importance(later[i]) > store.Importance(later[j]) })
	return now, later
}

func (d *drawer) assumedAnswers() []store.Item {
	var its []store.Item
	for _, it := range d.items {
		if !it.Done && it.Kind == "question" && it.Assumed != "" {
			its = append(its, it)
		}
	}
	return its
}

func (d *drawer) rightNow() {
	d.line(d.c(bold, "Right now"))
	d.line()
	names, groups := d.claimGroups()
	d.line(d.c(bold, "Being worked on"))
	if len(names) == 0 {
		d.line(d.c(faint, "  Nobody has claimed anything."))
	}
	for _, n := range names {
		d.line("  ", d.c(blue, n))
		for _, it := range groups[n] {
			d.row("    ", it, "since "+d.ago(parseTime(it.Claim.At)))
		}
	}
	d.line()
	now, later := d.readyTasks()
	d.line(d.c(bold, "Ready, nobody on it"), d.c(faint, "  most important first"))
	if len(now)+len(later) == 0 {
		d.line(d.c(faint, "  No task is ready: each one is claimed, waiting or done."))
	}
	for _, it := range now {
		d.row("  ", it, "")
	}
	if len(later) > 0 {
		d.line(d.c(faint, fmt.Sprintf("  and %s under Later", plur(len(later), "task"))))
	}
	if as := d.assumedAnswers(); len(as) > 0 {
		d.line()
		d.line(d.c(bold, "Answered for you, needs your OK"), d.c(faint, "  an agent picked an answer and went on"))
		for _, it := range as {
			d.row("  ", it, "")
		}
		d.line(d.c(faint, "  Keep it or change it: callboard answer Q-KEY \"your answer\""))
	}
}

func (d *drawer) lastTouch(it store.Item, touched map[string]store.Touch) time.Time {
	var best time.Time
	for _, s := range []string{touched[strings.ToUpper(it.Key)].At, it.At, it.DoneAt} {
		if t := parseTime(s); t.After(best) {
			best = t
		}
	}
	if it.Claim != nil {
		if t := parseTime(it.Claim.Seen); t.After(best) {
			best = t
		}
	}
	return best
}

type stale struct {
	it   store.Item
	at   time.Time
	last store.Touch
}

func (d *drawer) staleItems() []stale {
	var keys []string
	for _, it := range d.items {
		if !it.Done && it.Key != "" {
			keys = append(keys, it.Key)
		}
	}
	touched := d.st.Touched(keys)
	var out []stale
	for _, it := range d.items {
		if !it.Done && it.Key != "" {
			out = append(out, stale{it, d.lastTouch(it, touched), touched[strings.ToUpper(it.Key)]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out
}

func (d *drawer) quietRow(s stale) {
	extra := "untouched for " + strings.TrimSuffix(d.ago(s.at), " ago")
	if s.at.IsZero() {
		extra = "no date on it"
	}
	if s.last.What != "" {
		extra += " · last: " + s.last.By + " " + happened(store.Event{What: s.last.What, Detail: s.last.Detail})
	}
	d.row("  ", s.it, extra)
}

func (d *drawer) goneQuiet() {
	all := d.staleItems()
	var quiet []stale
	for _, s := range all {
		if d.now().Sub(s.at) >= quietAfter {
			quiet = append(quiet, s)
		}
	}
	d.line(d.c(bold, "Gone quiet"), d.c(faint, "  open items nobody has touched in a week or more, the longest first"))
	d.line()
	switch {
	case len(all) == 0:
		d.line("Nothing is open.")
	case len(quiet) == 0:
		d.line("Nothing has gone quiet: every open item had something happen this past week.")
		d.line()
		d.line(d.c(bold, "Longest untouched"))
		for _, s := range all[:min(3, len(all))] {
			d.quietRow(s)
		}
	default:
		for _, s := range quiet {
			d.quietRow(s)
		}
	}
}

func (d *drawer) doneDays() (days []string, by map[string][]store.Item, undated int) {
	by = map[string][]store.Item{}
	for _, it := range d.items {
		if !it.Done {
			continue
		}
		t := parseTime(it.DoneAt)
		if t.IsZero() {
			undated++
			continue
		}
		day := t.Format("2006-01-02")
		if _, ok := by[day]; !ok {
			days = append(days, day)
		}
		by[day] = append(by[day], it)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	for _, day := range days {
		its := by[day]
		sort.SliceStable(its, func(i, j int) bool { return its[i].DoneAt > its[j].DoneAt })
	}
	return days, by, undated
}

func (d *drawer) recentlyDone(maxDays int) {
	days, by, undated := d.doneDays()
	d.line(d.c(bold, "Recently done"), d.c(faint, "  what was ticked each day"))
	d.line()
	if len(days) == 0 {
		d.line("Nothing done yet.")
	}
	for i, day := range days {
		if i == maxDays {
			d.line(d.c(faint, fmt.Sprintf("and %s before that: %s", plur(len(days)-maxDays, "more day"), d.see("done"))))
			break
		}
		d.line(d.c(bold, d.dayName(day)), d.c(faint, "  "+plur(len(by[day]), "item")))
		for _, it := range by[day] {
			it.DoneAt = ""
			d.row("  ", it, "")
		}
		d.line()
	}
	if undated > 0 {
		d.line(d.c(faint, fmt.Sprintf("%s done with no date on it.", plur(undated, "item"))))
	}
}

func plainText(s string) string {
	return strings.NewReplacer("**", "", "`", "").Replace(s)
}

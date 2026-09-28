package store

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var DueFields = []string{"due", "deadline", "due-date", "due_date", "duedate", "by-date"}

var dateLayouts = []string{
	"2006-01-02", "2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339, "2006-01-02 15:04",
	"2006/01/02", "2006.01.02", "Jan 2 2006", "Jan 2, 2006", "January 2 2006", "January 2, 2006",
	"2 Jan 2006", "2 January 2006", "Mon Jan 2 2006", "Mon, 2 Jan 2006",
}

var yearless = []string{"Jan 2", "January 2", "2 Jan", "2 January"}

func ParseDate(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" || !unicode.IsDigit(rune(v[0])) && !unicode.IsLetter(rune(v[0])) {
		return time.Time{}, false
	}
	for _, l := range dateLayouts {
		if t, err := time.ParseInLocation(l, v, time.Local); err == nil {
			return t, true
		}
	}
	for _, l := range yearless {
		if t, err := time.ParseInLocation(l, v, time.Local); err == nil {
			return t.AddDate(time.Now().Year(), 0, 0), true
		}
	}
	return time.Time{}, false
}

func DueOf(it Item) (string, time.Time, bool) {
	for _, f := range DueFields {
		if v := it.Fields[f]; v != "" {
			if t, ok := ParseDate(v); ok {
				return v, t, true
			}
		}
	}
	return "", time.Time{}, false
}

const SoonDays = 3

func DueWords(it Item, now time.Time) string {
	if it.Done {
		return ""
	}
	_, t, ok := DueOf(it)
	if !ok {
		return ""
	}
	day := func(x time.Time) time.Time { y, m, d := x.Date(); return time.Date(y, m, d, 0, 0, 0, 0, time.Local) }
	n := int(math.Round(day(t).Sub(day(now)).Hours() / 24))
	switch {
	case n < -1:
		return fmt.Sprintf("overdue by %d days", -n)
	case n == -1:
		return "overdue since yesterday"
	case n == 0:
		return "due today"
	case n == 1:
		return "due tomorrow"
	case n <= SoonDays:
		return fmt.Sprintf("due in %d days", n)
	}
	return ""
}

func CompareValues(a, b string) int {
	ra, oka := PriorityWord(a)
	rb, okb := PriorityWord(b)
	if oka && okb {
		return ra - rb
	}
	if oka != okb {
		if oka {
			return 1
		}
		return -1
	}
	if ta, ok := ParseDate(a); ok {
		if tb, ok := ParseDate(b); ok {
			return ta.Compare(tb)
		}
	}
	fa, ea := strconv.ParseFloat(a, 64)
	fb, eb := strconv.ParseFloat(b, 64)
	if ea == nil && eb == nil {
		return cmp.Compare(fa, fb)
	}
	return Natural(a, b)
}

func Natural(a, b string) int {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		na, ra := numPrefix(a)
		nb, rb := numPrefix(b)
		if na != "" && nb != "" {
			fa, _ := strconv.ParseFloat(na, 64)
			fb, _ := strconv.ParseFloat(nb, 64)
			if fa != fb {
				if fa < fb {
					return -1
				}
				return 1
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func numPrefix(s string) (string, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

package store

import (
	"slices"
	"testing"
	"time"
)

func TestDueWords(t *testing.T) {
	now := time.Date(2026, 9, 28, 15, 0, 0, 0, time.Local)
	cases := map[string]string{
		"2026-09-20":         "overdue by 8 days",
		"2026-09-27":         "overdue since yesterday",
		"2026-09-28T09:00":   "due today",
		"2026-09-29":         "due tomorrow",
		"Oct 1 2026":         "due in 3 days",
		"2026-10-02":         "",
		"soon":               "",
		"2 October 2026":     "",
		"September 26, 2026": "overdue by 2 days",
	}
	for v, want := range cases {
		if got := DueWords(Item{Fields: map[string]string{"due": v}}, now); got != want {
			t.Errorf("%s: got %q, want %q", v, got, want)
		}
	}
	if got := DueWords(Item{Done: true, Fields: map[string]string{"due": "2026-09-01"}}, now); got != "" {
		t.Errorf("a done item is overdue: %q", got)
	}
	if got := DueWords(Item{Fields: map[string]string{"deadline": "2026-09-28"}}, now); got != "due today" {
		t.Errorf("deadline: %q", got)
	}
}

func TestCompareValues(t *testing.T) {
	vals := []string{"12d", "3d", "Oct 2 2026", "2026-09-30", "v1.10", "v1.9", "-5", "-10", "2.5"}
	sorted := slices.Clone(vals)
	slices.SortStableFunc(sorted, CompareValues)
	pos := func(x string) int { return slices.Index(sorted, x) }
	for _, p := range [][2]string{{"3d", "12d"}, {"2026-09-30", "Oct 2 2026"}, {"v1.9", "v1.10"}, {"-10", "-5"}, {"-5", "2.5"}} {
		if pos(p[0]) > pos(p[1]) {
			t.Errorf("%s should sort before %s: %v", p[0], p[1], sorted)
		}
	}
}

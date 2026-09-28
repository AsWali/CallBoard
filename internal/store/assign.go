package store

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
)

const ForField = "for"

var humanWords = []string{"you", "me", "human", "user"}

func ForNames(who string) []string {
	w := strings.ToLower(who)
	for _, sep := range []string{" or ", " and ", "/", "&", "+", ";"} {
		w = strings.ReplaceAll(w, sep, ",")
	}
	var out []string
	for _, n := range strings.Split(w, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func anyone(names []string) bool {
	return len(names) == 0 || slices.Contains(names, "anyone") || slices.Contains(names, "anybody")
}

func (s *Store) Matcher(a Actor) func(who string) (mine, other bool) {
	by := strings.ToLower(a.By)
	var own []string
	if a.Session != "" && by != "you" {
		for _, x := range s.Agents(0) {
			if x.Session == a.Session {
				own = append(own, strings.ToLower(x.Name), strings.ToLower(x.Title))
			}
		}
	}
	return func(who string) (bool, bool) {
		names := ForNames(who)
		if anyone(names) {
			return false, false
		}
		if by == "you" {
			return slices.ContainsFunc(names, func(n string) bool { return slices.Contains(humanWords, n) }), false
		}
		for _, n := range names {
			if n == by || strings.HasPrefix(by, n+" ") || !slices.Contains(humanWords, n) && n != "claude" && n != "codex" && slices.Contains(own, n) {
				return true, false
			}
		}
		return false, true
	}
}

func (s *Store) ForSomeoneElse(who string, a Actor) bool {
	_, other := s.Matcher(a)(who)
	return other
}

func Assigned(it Item) bool {
	return !anyone(ForNames(it.Fields[ForField]))
}

func IsForYou(it Item) bool {
	return slices.ContainsFunc(ForNames(it.Fields[ForField]), func(n string) bool { return slices.Contains(humanWords, n) })
}

func OnlyForYou(it Item) bool {
	names := ForNames(it.Fields[ForField])
	return len(names) > 0 && !slices.ContainsFunc(names, func(n string) bool { return !slices.Contains(humanWords, n) })
}

func ForText(who string) string {
	names := ForNames(who)
	var out []string
	for _, n := range names {
		if slices.Contains(humanWords, n) {
			n = "the human"
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return strings.Join(out, " or ")
}

func (s *Store) checkFor(key, who string, a Actor) error {
	if !s.ForSomeoneElse(who, a) {
		return nil
	}
	if names := ForNames(who); !slices.ContainsFunc(names, func(n string) bool { return !slices.Contains(humanWords, n) }) {
		return fmt.Errorf("%s is for the human; pick another item, or pass force if they asked you to do it", key)
	}
	return fmt.Errorf("%s is for %s; pick another item, or pass force to take it anyway", key, ForText(who))
}

func (s *Store) already(t *board.Task) error {
	if !t.Done {
		return fmt.Errorf("%s is already open", t.Key)
	}
	who := ""
	if by := s.DoneBy(t); by != "" {
		who = " by " + by
	}
	return fmt.Errorf("%s %q is already done%s; pick another task (the list tool with status ready)", t.Key, t.Title, who)
}

func (s *Store) sessionRuns(c Claim, live map[string]bool) bool {
	if live[c.Session] {
		return true
	}
	if strings.HasPrefix(c.By, "codex") {
		if t := transcriptOf(c.Session, "codex", ""); t != nil && time.Since(t.mod) < ClaimTTL {
			return true
		}
	}
	return false
}

func runningSessions() map[string]bool {
	m := map[string]bool{}
	for _, r := range liveSessions() {
		m[r.SessionID] = true
	}
	return m
}

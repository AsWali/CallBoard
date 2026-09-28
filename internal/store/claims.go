package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AsWali/CallBoard/internal/gitx"
)

var ClaimTTL = 30 * time.Minute

type Claim struct {
	Key      string `json:"key"`
	By       string `json:"by"`
	Session  string `json:"session"`
	Worktree string `json:"worktree"`
	Branch   string `json:"branch,omitempty"`
	At       string `json:"at"`
	Seen     string `json:"seen"`
}

func (c Claim) Live() bool {
	t, err := time.Parse(time.RFC3339, c.Seen)
	return err == nil && time.Since(t) < ClaimTTL
}

func (s *Store) claimsPath() string { return filepath.Join(s.Shared(), "claims.json") }

func (s *Store) readClaims() map[string]Claim {
	m := map[string]Claim{}
	b, err := os.ReadFile(s.claimsPath())
	if err == nil {
		json.Unmarshal(b, &m)
	}
	var live map[string]bool
	for k, c := range m {
		if !c.Live() && live == nil {
			live = runningSessions()
		}
		switch {
		case !c.Live() && !s.sessionRuns(c, live):
			delete(m, k)
		case c.Worktree != "" && !gitx.Exists(c.Worktree):
			delete(m, k)
		case s.Common != "" && c.Worktree != "":

			c.Branch = gitx.BranchAt(c.Worktree)
			m[k] = c
		}
	}
	return m
}

func (s *Store) writeClaims(m map[string]Claim) error {
	os.MkdirAll(s.Shared(), 0o755)
	b, _ := json.MarshalIndent(m, "", "  ")
	return writeAtomic(s.claimsPath(), string(b)+"\n")
}

func (s *Store) Claims() map[string]Claim { return s.readClaims() }

func (s *Store) withClaims(fn func(m map[string]Claim) error) error {
	unlock, err := s.lockNamed("claims")
	if err != nil {
		return err
	}
	defer unlock()
	m := s.readClaims()
	if err := fn(m); err != nil {
		return err
	}
	return s.writeClaims(m)
}

func (s *Store) ClaimItem(key, by, session string, force bool) (Claim, error) {
	k, ok := s.KindOf(key)
	if !ok {
		return Claim{}, fmt.Errorf("no item %s", key)
	}
	f, err := s.Load(k)
	if err != nil {
		return Claim{}, err
	}
	t := f.Find(key)
	if t == nil {
		return Claim{}, fmt.Errorf("no item %s here", key)
	}
	if t.Done {
		return Claim{}, s.already(t)
	}
	if !force {
		if err := s.checkFor(t.Key, t.Meta(ForField), Actor{By: by, Session: session}); err != nil {
			return Claim{}, err
		}
	}
	now := time.Now().Format(time.RFC3339)
	var c Claim
	err = s.withClaims(func(m map[string]Claim) error {
		if old, ok := m[strings.ToUpper(t.Key)]; ok && old.Session != session && !force {
			return fmt.Errorf("%s is claimed by %s on %s since %s; pick another item, or pass force to take it over", t.Key, old.By, branchOr(old.Branch), ago(old.At))
		}
		c = Claim{Key: t.Key, By: by, Session: session, Worktree: s.Root, Branch: s.Branch(), At: now, Seen: now}
		m[strings.ToUpper(t.Key)] = c
		return nil
	})
	if err == nil {
		s.Log(Event{By: by, What: "claimed", Key: t.Key, Title: t.Title, Session: session})
		s.statusOnClaim(t.Key, true, Actor{By: by, Session: session})
	}
	return c, err
}

func (s *Store) Release(key, by, session string, force bool) error {
	released := false
	err := s.withClaims(func(m map[string]Claim) error {
		k := strings.ToUpper(key)
		if c, ok := m[k]; ok && (c.Session == session || force) {
			delete(m, k)
			released = true
		}
		return nil
	})
	if released {
		s.Log(Event{By: by, What: "released", Key: key, Session: session})
		s.statusOnClaim(key, false, Actor{By: by, Session: session})
	}
	return err
}

func (s *Store) Touch(session string) {
	if session == "" {
		return
	}
	m := s.readClaims()
	mine := false
	for _, c := range m {
		mine = mine || c.Session == session
	}
	if !mine {
		return
	}
	now := time.Now().Format(time.RFC3339)
	s.withClaims(func(m map[string]Claim) error {
		for k, c := range m {
			if c.Session == session {
				c.Seen = now
				m[k] = c
			}
		}
		return nil
	})
}

func (s *Store) ClaimList() []Claim {
	var out []Claim
	for _, c := range s.readClaims() {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func branchOr(b string) string {
	if b == "" {
		return "a detached worktree"
	}
	return b
}

func ago(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return rfc
	}
	d := time.Since(t).Round(time.Minute)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

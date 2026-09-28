package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type ownChain struct {
	To   string   `json:"to"`
	From []string `json:"from"`
}

type ownMap map[string]ownChain

func (s *Store) ownPath(session string) string {
	return filepath.Join(s.Shared(), "own", safeName(session)+".json")
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '.' {
			return '_'
		}
		return r
	}, s)
}

func (s *Store) ownChanges(session string) ownMap {
	m := ownMap{}
	if session == "" || s.Common == "" {
		return nil
	}
	if b, err := os.ReadFile(s.ownPath(session)); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func (s *Store) saveOwnChanges(session string, m ownMap) {
	if session == "" || m == nil {
		return
	}
	b, _ := json.Marshal(m)
	p := s.ownPath(session)
	os.MkdirAll(filepath.Dir(p), 0o755)
	writeAtomic(p, string(b))
}

func (m ownMap) also(f *board.File) map[string][]string {
	out := map[string][]string{}
	for _, t := range f.Tasks {
		if c, ok := m[strings.ToUpper(t.Key)]; ok && c.To == t.Version {
			out[strings.ToUpper(t.Key)] = c.From
		}
	}
	return out
}

func (m ownMap) note(key, from, to string) {
	if m == nil {
		return
	}
	k := strings.ToUpper(key)
	c := m[k]
	if c.To != from {
		c.From = nil
	}
	c.From = append(c.From, from)
	if len(c.From) > 20 {
		c.From = c.From[len(c.From)-20:]
	}
	c.To = to
	m[k] = c
}

package store

import (
	"fmt"
	"strings"

	"github.com/AsWali/CallBoard/internal/board"
)

type SectionChange struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	To     string `json:"to,omitempty"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Remove bool   `json:"remove,omitempty"`
	Into   string `json:"into,omitempty"`
}

func (s *Store) ChangeSection(c SectionChange, a Actor) (string, error) {
	k, err := s.KindNamed(c.Kind)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(strings.TrimLeft(c.Name, "#"))
	if name == "" {
		return "", fmt.Errorf("which section?")
	}
	n := 0
	for _, x := range []bool{c.To != "", c.Before != "" || c.After != "", c.Remove} {
		if x {
			n++
		}
	}
	if n != 1 || c.Before != "" && c.After != "" {
		return "", fmt.Errorf("give one of: to (a new name), before or after (another section), or remove")
	}
	var detail string
	var moved []*board.Task
	_, err = s.UpdateBy(k, a, func(f *board.File) error {
		switch {
		case c.To != "":
			old, err := f.RenameSection(name, c.To)
			if err != nil {
				return err
			}
			name, detail = strings.TrimSpace(strings.TrimLeft(c.To, "#")), "renamed the section "+old+" to "+strings.TrimSpace(strings.TrimLeft(c.To, "#"))
		case c.Remove:
			ts, err := f.RemoveSection(name, c.Into)
			if err != nil {
				return err
			}
			moved, detail = ts, "removed the section "+name
			if len(ts) > 0 {
				detail += fmt.Sprintf(" and moved its %d items to %s", len(ts), ts[0].Section)
			}
		default:
			target, after := c.Before, false
			if c.After != "" {
				target, after = c.After, true
			}
			if err := f.MoveSection(name, target, after); err != nil {
				return err
			}
			detail = "moved the section " + name + map[bool]string{false: " before ", true: " after "}[after] + strings.TrimSpace(strings.TrimLeft(target, "#"))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	s.Log(Event{By: a.By, What: "section", Title: k.File, Detail: detail, Session: a.Session})
	for _, t := range moved {
		s.Log(Event{By: a.By, What: "moved", Key: t.Key, Title: t.Title, Detail: t.Section, Session: a.Session})
	}
	return detail + " in " + k.File, nil
}

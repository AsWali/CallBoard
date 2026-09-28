package store

import (
	"path/filepath"
	"strings"
)

func (l List) Find(q string) List {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return l
	}
	var keep []Item
	for _, it := range l.Items {
		hay := strings.ToLower(it.Key + " " + it.Title + " " + strings.Join(it.Body, " ") + " " + formatFields(it.Fields) + " " + it.Answer)
		all := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				all = false
				break
			}
		}
		if all {
			keep = append(keep, it)
		}
	}
	l.Items = keep
	if keep == nil {
		l.Items = []Item{}
	}
	return l
}

func RealPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(r, filepath.Base(p))
	}
	return filepath.Clean(p)
}

func (s *Store) ListFile(path string) (Kind, bool) {
	p := RealPath(path)
	for _, k := range append(s.Lists(), Views) {
		if p == RealPath(s.File(k)) {
			return k, !s.Foreign(k)
		}
	}
	return Kind{}, false
}

func (s *Store) ReadInstead(k Kind, find string) string {
	var b strings.Builder
	if k == Views {
		b.WriteString(".callboard/views.md is kept by Callboard; don't read it directly. The saved views (the view tool, or callboard views):\n")
		for _, v := range s.SavedViews() {
			b.WriteString(s.Checked(v) + "\n")
		}
		return b.String()
	}
	status := ""
	if find != "" {
		status = "all"
	}
	l, err := s.List(k.Name, status, "")
	if err != nil {
		return k.File + " is kept by Callboard; use the list tool (or callboard list) instead of reading it."
	}
	if find != "" {
		l = s.Search(l, find)
		b.WriteString(k.File + " is kept by Callboard; don't read or search it directly. The list tool's find gives the same (callboard list --find \"" + find + "\"):\n")
	} else {
		b.WriteString(k.File + " is kept by Callboard; don't read it directly. Its open items, as the list tool gives them (callboard list --kind " + k.Name + "; status done or all for finished ones):\n")
	}
	b.WriteString(FormatList(l))
	b.WriteString("One item's notes, what it waits on and frees, and its history: the list tool with key (callboard show B-k3f9). Change items with the tools, not by editing the file.\n")
	return b.String()
}

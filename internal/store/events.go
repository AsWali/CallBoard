package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
)

type Event struct {
	At       string         `json:"at"`
	Worktree string         `json:"worktree"`
	Branch   string         `json:"branch,omitempty"`
	By       string         `json:"by"`
	What     string         `json:"what"`
	Key      string         `json:"key,omitempty"`
	Title    string         `json:"title,omitempty"`
	Detail   string         `json:"detail,omitempty"`
	Session  string         `json:"session,omitempty"`
	Via      string         `json:"via,omitempty"`
	Copy     *board.Removed `json:"copy,omitempty"`
}

func (e *Event) UnmarshalJSON(b []byte) error {
	type plain Event
	if err := json.Unmarshal(b, (*plain)(e)); err != nil {
		return err
	}
	for _, p := range []*string{&e.By, &e.Title, &e.Detail, &e.Via, &e.Branch} {
		*p = board.StripControl(*p)
	}
	return nil
}

func (s *Store) logPath() string { return filepath.Join(s.Shared(), "events.jsonl") }

var maxLog, keepLog = 8 << 20, 4 << 20

const keptRemovals = 1000

func (s *Store) Log(e Event) {
	e.At = time.Now().Format(time.RFC3339)
	e.Worktree, e.Branch = s.Root, s.Branch()
	unlock, err := s.lockNamed("events")
	if err != nil {
		return
	}
	defer unlock()
	f, err := os.OpenFile(s.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	b, _ := json.Marshal(e)
	f.Write(append(b, '\n'))
	st, err := f.Stat()
	f.Close()
	if err == nil && st.Size() > int64(maxLog) {
		s.trimLog()
	}
}

var (
	removedMark = []byte(`"what":"removed"`)
	backMarks   = [][]byte{[]byte(`"what":"added"`), []byte(`"what":"restored"`), removedMark}
)

func eachLine(data []byte, fn func(line []byte)) {
	for len(data) > 0 {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			fn(data)
			return
		}
		fn(data[:i])
		data = data[i+1:]
	}
}

func (s *Store) trimLog() {
	p := s.logPath()
	data, err := os.ReadFile(p)
	if err != nil || len(data) <= keepLog {
		return
	}
	cut := len(data) - keepLog
	i := bytes.IndexByte(data[cut:], '\n')
	if i < 0 {
		return
	}
	cut += i + 1
	old, kept := data[:cut], data[cut:]
	later := map[string]bool{}
	eachLine(kept, func(line []byte) {
		if k := lineKey(line); k != "" && slices.ContainsFunc(backMarks, func(m []byte) bool { return bytes.Contains(line, m) }) {
			later[k] = true
		}
	})
	removed := map[string]int{}
	var lines [][]byte
	eachLine(old, func(line []byte) {
		k := lineKey(line)
		switch {
		case k == "" || later[k]:
		case bytes.Contains(line, removedMark) && bytes.Contains(line, []byte(`"copy":`)):
			removed[k] = len(lines)
			lines = append(lines, line)
		case slices.ContainsFunc(backMarks, func(m []byte) bool { return bytes.Contains(line, m) }):
			delete(removed, k)
		}
	})
	var at []int
	for _, n := range removed {
		at = append(at, n)
	}
	slices.Sort(at)
	if len(at) > keptRemovals {
		at = at[len(at)-keptRemovals:]
	}
	size, from := 0, len(at)
	for from > 0 && size+len(lines[at[from-1]])+1 <= keepLog/2 {
		from--
		size += len(lines[at[from]]) + 1
	}
	var carried []byte
	for _, n := range at[from:] {
		carried = append(append(carried, lines[n]...), '\n')
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, append(carried, kept...), 0o644) != nil {
		os.Remove(tmp)
		return
	}
	if os.Rename(tmp, p) != nil {
		os.Remove(tmp)
		return
	}
	dir := filepath.Join(s.Shared(), "cursors")
	entries, _ := os.ReadDir(dir)
	for _, en := range entries {
		cp := filepath.Join(dir, en.Name())
		b, err := os.ReadFile(cp)
		if err != nil {
			continue
		}
		c, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if c < int64(cut) {
			os.Remove(cp)
			continue
		}
		os.WriteFile(cp, []byte(strconv.FormatInt(c-int64(cut)+int64(len(carried)), 10)), 0o644)
	}
}

func (s *Store) readLog(from int64) ([]Event, int64) {
	f, err := os.Open(s.logPath())
	if err != nil {
		return nil, 0
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil {
		return nil, 0
	} else if from > st.Size() {
		from = st.Size()
	}
	f.Seek(from, io.SeekStart)
	rd := bufio.NewReader(f)
	var evs []Event
	for {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			return evs, from
		}
		var e Event
		if json.Unmarshal(line, &e) == nil {
			evs = append(evs, e)
		}
		from += int64(len(line))
	}
}

func (s *Store) backLines(fn func(line []byte) bool) {
	f, err := os.Open(s.logPath())
	if err != nil {
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return
	}
	var rest []byte
	for end := st.Size(); end > 0; {
		n := min(64<<10, end)
		end -= n
		b := make([]byte, n, n+int64(len(rest)))
		if _, err := f.ReadAt(b, end); err != nil && err != io.EOF {
			return
		}
		b = append(b, rest...)
		for {
			i := bytes.LastIndexByte(b, '\n')
			if i < 0 {
				break
			}
			line := b[i+1:]
			b = b[:i]
			if len(line) > 0 && !fn(line) {
				return
			}
		}
		rest = b
	}
	if len(rest) > 0 {
		fn(rest)
	}
}

var keyMark = []byte(`"key":"`)

func lineKey(line []byte) string {
	i := bytes.Index(line, keyMark)
	if i < 0 {
		return ""
	}
	rest := line[i+len(keyMark):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return strings.ToUpper(string(rest[:j]))
}

func (s *Store) Events(limit int) []Event {
	root, _ := json.Marshal(s.Root)
	mark := append([]byte(`"worktree":`), root...)
	var out []Event
	s.backLines(func(line []byte) bool {
		var e Event
		if bytes.Contains(line, mark) && json.Unmarshal(line, &e) == nil && e.Worktree == s.Root {
			out = append(out, e)
		}
		return limit <= 0 || len(out) < limit
	})
	return out
}

type Touch struct {
	At     string `json:"at"`
	By     string `json:"by"`
	What   string `json:"what"`
	Detail string `json:"detail,omitempty"`
}

type touchIndex struct {
	mu   sync.Mutex
	file os.FileInfo
	end  int64
	last map[string]Touch
	done map[string]string
}

var finishMarks = [][]byte{[]byte(`"what":"ticked"`), []byte(`"what":"answered"`), []byte(`"what":"reopened"`)}

func (s *Store) Finishers() map[string]string {
	s.Touched(nil)
	touchMu.Lock()
	idx := touchIdx[s.logPath()]
	touchMu.Unlock()
	if idx == nil {
		return nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return maps.Clone(idx.done)
}

var (
	touchMu  sync.Mutex
	touchIdx = map[string]*touchIndex{}
)

var sessionMark = []byte(`"what":"session"`)

func (s *Store) Touched(keys []string) map[string]Touch {
	out := map[string]Touch{}
	p := s.logPath()
	touchMu.Lock()
	idx := touchIdx[p]
	if idx == nil {
		idx = &touchIndex{}
		touchIdx[p] = idx
	}
	touchMu.Unlock()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	f, err := os.Open(p)
	if err != nil {
		return out
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return out
	}
	if idx.file == nil || !os.SameFile(idx.file, st) || st.Size() < idx.end {
		idx.end, idx.last, idx.done = 0, map[string]Touch{}, map[string]string{}
	}
	idx.file = st
	if st.Size() > idx.end {
		data := make([]byte, st.Size()-idx.end)
		if _, err := f.ReadAt(data, idx.end); err != nil && err != io.EOF {
			return out
		}
		if n := bytes.LastIndexByte(data, '\n'); n >= 0 {
			data = data[:n+1]
			lines := map[string][]byte{}
			for rest := data; len(rest) > 0; {
				i := bytes.IndexByte(rest, '\n')
				line := rest[:i]
				rest = rest[i+1:]
				if k := lineKey(line); k != "" && !bytes.Contains(line, sessionMark) {
					lines[k] = line
					if slices.ContainsFunc(finishMarks, func(m []byte) bool { return bytes.Contains(line, m) }) {
						var e Event
						if json.Unmarshal(line, &e) == nil {
							if e.What == "reopened" {
								delete(idx.done, k)
							} else {
								idx.done[k] = e.By
							}
						}
					}
				}
			}
			for k, line := range lines {
				var e Event
				if json.Unmarshal(line, &e) == nil {
					idx.last[k] = Touch{e.At, e.By, e.What, e.Detail}
				}
			}
			idx.end += int64(len(data))
		}
	}
	for _, k := range keys {
		if t, ok := idx.last[strings.ToUpper(k)]; ok {
			out[strings.ToUpper(k)] = t
		}
	}
	return out
}

func (s *Store) ItemEvents(key string, limit int) []Event {
	key = strings.ToUpper(key)
	var out []Event
	s.backLines(func(line []byte) bool {
		var e Event
		if lineKey(line) == key && json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
		return limit <= 0 || len(out) < limit
	})
	return out
}

var unsafeRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (s *Store) cursorPath(session string) string {
	return filepath.Join(s.Shared(), "cursors", unsafeRe.ReplaceAllString(session, "_"))
}

func (s *Store) MarkRead(session string) {
	if session == "" {
		return
	}
	unlock, err := s.lockNamed("events")
	if err != nil {
		return
	}
	defer unlock()
	s.markRead(session)
}

func (s *Store) markRead(session string) {
	if s.Off() {
		return
	}
	var end int64
	if st, err := os.Stat(s.logPath()); err == nil {
		end = st.Size()
	}
	os.MkdirAll(filepath.Dir(s.cursorPath(session)), 0o755)
	os.WriteFile(s.cursorPath(session), []byte(strconv.FormatInt(end, 10)), 0o644)
}

func (s *Store) News(session string) []Event {
	if session == "" {
		return nil
	}
	s.Reconcile()
	unlock, err := s.lockNamed("events")
	if err != nil {
		unlock = func() {}
	}
	b, err := os.ReadFile(s.cursorPath(session))
	if err != nil {
		s.markRead(session)
		unlock()
		return nil
	}
	from, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	evs, end := s.readLog(from)
	if end != from {
		os.WriteFile(s.cursorPath(session), []byte(strconv.FormatInt(end, 10)), 0o644)
	}
	unlock()
	var out []Event
	answered := map[string]bool{}
	viewAt := map[string]int{}
	for _, e := range evs {
		if e.Session == session || e.What == "session" {
			continue
		}
		if e.What == "answered" {
			if answered[strings.ToUpper(e.Key)+e.Detail] {
				continue
			}
			answered[strings.ToUpper(e.Key)+e.Detail] = true
		}
		mine := e.Worktree == s.Root
		outside := e.By == "git" || e.By == "someone"
		if (mine && (e.By == "you" || outside)) || e.What == "answered" && (mine || s.hasKey(e.Key)) {
			if n, ok := viewAt[e.Key]; ok && e.What == "view" {
				out[n] = e
				continue
			}
			if e.What == "view" {
				viewAt[e.Key] = len(out)
			}
			out = append(out, e)
		}
	}
	return out
}

func (s *Store) hasKey(key string) bool {
	k, ok := s.KindOf(key)
	if !ok {
		return false
	}
	f, err := s.Load(k)
	return err == nil && f.Find(key) != nil
}

func FormatNews(evs []Event) string {
	if len(evs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Callboard news (what changed since you last looked):\n")
	for _, e := range evs {
		who := e.By
		switch who {
		case "you":
			who = "The human"
		case "git":
			who = "Git"
		case "someone":
			who = "Someone"
		}
		if e.Via != "" {
			who += " (" + e.Via + ")"
		}
		switch e.What {
		case "answered":
			b.WriteString("- " + who + " answered " + e.Key + " \"" + e.Title + "\": " + e.Detail + "\n")
		case "renamed":
			b.WriteString("- " + who + " renamed " + e.Key + " to \"" + e.Title + "\" (was \"" + e.Detail + "\")\n")
		case "needs":
			b.WriteString("- " + who + " set " + e.Key + " \"" + e.Title + "\" to wait on " + e.Detail + "\n")
		case "reopened":
			if e.Detail != "" {
				b.WriteString("- " + who + " opened " + e.Key + " \"" + e.Title + "\" again: " + e.Detail + ". Redo it for the real answer.\n")
			} else {
				b.WriteString("- " + who + " opened " + e.Key + " \"" + e.Title + "\" again\n")
			}
		case "set":
			b.WriteString("- " + who + " set " + e.Key + " \"" + e.Title + "\": " + e.Detail + "\n")
		case "noted":
			b.WriteString("- " + who + " added a note to " + e.Key + " \"" + e.Title + "\": " + e.Detail + "\n")
		case "notes":
			if e.Detail == "" {
				b.WriteString("- " + who + " removed the notes of " + e.Key + " \"" + e.Title + "\"\n")
			} else {
				b.WriteString("- " + who + " changed the notes of " + e.Key + " \"" + e.Title + "\" to: " + e.Detail + "\n")
			}
		case "removed":
			b.WriteString("- " + who + " deleted " + e.Key + " \"" + e.Title + "\" (callboard restore " + e.Key + " brings it back)\n")
		case "restored":
			b.WriteString("- " + who + " brought back " + e.Key + " \"" + e.Title + "\"\n")
		case "list":
			switch {
			case e.Detail == "removed":
				b.WriteString("- " + who + " removed the list " + e.Title + "\n")
			case strings.HasPrefix(e.Detail, "renamed from "):
				b.WriteString("- " + who + " renamed the list " + strings.TrimPrefix(e.Detail, "renamed from ") + " to " + e.Title + "; use kind " + strings.TrimSuffix(e.Title, ".md") + " now\n")
			default:
				b.WriteString("- " + who + " added the list " + e.Title + " (" + e.Detail + "); the list tool shows it\n")
			}
		case "section":
			b.WriteString("- " + who + " " + e.Detail + " in " + e.Title + "\n")
		case "moved":
			b.WriteString("- " + who + " moved " + e.Key + " \"" + e.Title + "\" to ## " + e.Detail + "\n")
		case "view":
			if e.Detail == "removed" {
				b.WriteString("- " + who + " deleted the view \"" + e.Title + "\" (" + e.Key + ")\n")
			} else if e.Detail == "added" {
				b.WriteString("- " + who + " made the view \"" + e.Title + "\" (" + e.Key + "); see it with callboard views\n")
			} else {
				b.WriteString("- " + who + " changed the view \"" + e.Title + "\" (" + e.Key + "); see it with callboard views\n")
			}
		default:
			b.WriteString("- " + who + " " + e.What + " " + e.Key + " \"" + e.Title + "\"\n")
		}
	}
	return b.String()
}

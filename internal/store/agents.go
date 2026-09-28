package store

import (
	"github.com/AsWali/CallBoard/internal/board"

	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Agent struct {
	Session   string `json:"session"`
	Tool      string `json:"tool"`
	By        string `json:"by"`
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	Color     string `json:"color,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	How       string `json:"how,omitempty"`
	Started   string `json:"started,omitempty"`
	Active    string `json:"active,omitempty"`
	Live      bool   `json:"live"`
	Status    string `json:"status,omitempty"`
	Subagents int    `json:"subagents,omitempty"`
}

func (s *Store) Agents(limit int) []Agent {
	by := map[string]*Agent{}
	var order []string
	get := func(id string) *Agent {
		if a, ok := by[id]; ok {
			return a
		}
		by[id] = &Agent{Session: id, Tool: "claude", By: "claude"}
		order = append(order, id)
		return by[id]
	}
	for _, e := range s.Events(500) {
		if e.Session == "" || strings.HasPrefix(e.Session, "mcp-") {
			continue
		}
		if strings.HasPrefix(e.By, "codex") {
			a := get(strings.SplitN(e.Session, ":", 2)[0])
			a.Tool, a.By = "codex", "codex"
			if e.What != "session" {
				continue
			}
		}
		if e.What != "session" {
			continue
		}
		if parent, _, sub := strings.Cut(e.Session, ":"); sub {
			get(parent).Subagents++
			continue
		}
		a := get(e.Session)
		if a.How == "" {
			a.How = e.Title
		}
		a.Started = localTime(e.At)
	}
	for _, t := range codexThreads() {
		if !s.holds(t.Cwd) || t.Review {
			continue
		}
		if t.Parent != "" {
			get(t.Parent).Subagents++
			continue
		}
		a := get(t.ID)
		a.Tool, a.By, a.Started = "codex", "codex", t.Started
		if a.How == "" && t.Exec {
			a.How = "exec"
		}
	}
	names := codexNames()
	for _, r := range liveSessions() {
		if !s.holds(r.Cwd) {
			continue
		}
		a := get(r.SessionID)
		a.Live, a.Status = true, r.status()
		if r.NameSource == "user" {
			a.Name = r.Name
		}
		if at, err := time.Parse(time.RFC3339, a.Started); r.StartedAt > 0 && (err != nil || time.UnixMilli(r.StartedAt).Before(at)) {
			a.Started = time.UnixMilli(r.StartedAt).Local().Format(time.RFC3339)
		}
	}
	var out []Agent
	for _, id := range order {
		a := by[id]
		if t := transcriptOf(id, a.Tool, s.Root); t != nil {
			if n := t.info.Name; n != "" {
				a.Name = board.StripControl(n)
			}
			a.Title, a.Color, a.Prompt = t.info.Title, t.info.Color, t.info.Prompt
			a.Active = t.mod.Local().Format(time.RFC3339)
			if a.Tool == "codex" && time.Since(t.mod) < 20*time.Second {
				a.Live, a.Status = true, "busy"
			}
		}
		if n := names[id]; n != "" && a.Title == "" {
			a.Title = n
		}
		out = append(out, *a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Live != out[j].Live {
			return out[i].Live
		}
		return latest(out[i]) > latest(out[j])
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func latest(a Agent) string {
	if a.Active > a.Started {
		return a.Active
	}
	return a.Started
}

func (s *Store) holds(dir string) bool {
	root := realPath(s.Root)
	dir = realPath(dir)
	if dir == root {
		return true
	}
	if !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return false
	}
	for d := dir; d != root && len(d) > len(root); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return false
		}
	}
	return true
}

func realPath(p string) string {
	realMu.Lock()
	defer realMu.Unlock()
	if r, ok := reals[p]; ok {
		return r
	}
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if len(reals) > 2000 {
		clear(reals)
	}
	reals[p] = r
	return r
}

var (
	realMu sync.Mutex
	reals  = map[string]string{}
)

func claudeDirs() []string {
	var cands []string
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		cands = append(cands, d)
	}
	if home, err := os.UserHomeDir(); err == nil {
		cands = append(cands, filepath.Join(home, ".claude"))
		more, _ := filepath.Glob(filepath.Join(home, ".claude-*"))
		cands = append(cands, more...)
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range cands {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			continue
		}
		if r := realPath(d); !seen[r] {
			seen[r] = true
			out = append(out, d)
		}
	}
	return out
}

type liveSession struct {
	Pid        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	Name       string `json:"name"`
	NameSource string `json:"nameSource"`
	Status     string `json:"status"`
	StartedAt  int64  `json:"startedAt"`
}

func (r liveSession) status() string {
	if r.Status == "busy" {
		return "busy"
	}
	return "idle"
}

func liveSessions() []liveSession {
	var out []liveSession
	seen := map[string]bool{}
	for _, d := range claudeDirs() {
		files, _ := filepath.Glob(filepath.Join(d, "sessions", "*.json"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			var r liveSession
			if json.Unmarshal(b, &r) != nil || r.SessionID == "" || seen[r.SessionID] || !alive(r.Pid) {
				continue
			}
			seen[r.SessionID] = true
			out = append(out, r)
		}
	}
	return out
}

type transcriptInfo struct {
	Name, Title, Color, Prompt string
}

type transcript struct {
	path  string
	codex bool
	off   int64
	mod   time.Time
	info  transcriptInfo
}

var (
	transcriptsMu sync.Mutex
	transcripts   = map[string]*transcript{}
	missing       = map[string]time.Time{}
)

var notAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

func transcriptOf(session, tool, root string) *transcript {
	transcriptsMu.Lock()
	defer transcriptsMu.Unlock()
	t := transcripts[session]
	if t != nil {
		if _, err := os.Stat(t.path); err != nil {
			t = nil
		}
	}
	if t == nil && time.Since(missing[session]) < 30*time.Second {
		return nil
	}
	if t == nil {
		if tool == "codex" {
			if p := codexRollout(session); p != "" {
				t = &transcript{path: p, codex: true}
			}
		}
		for _, d := range claudeDirs() {
			if t != nil || tool == "codex" {
				break
			}
			if p := filepath.Join(d, "projects", notAlnum.ReplaceAllString(root, "-"), session+".jsonl"); root != "" && isFile(p) {
				t = &transcript{path: p}
			} else if m, _ := filepath.Glob(filepath.Join(d, "projects", "*", session+".jsonl")); len(m) > 0 {
				t = &transcript{path: m[0]}
			}
		}
		if t == nil {
			missing[session] = time.Now()
			return nil
		}
		delete(missing, session)
		transcripts[session] = t
	}
	st, err := os.Stat(t.path)
	if err != nil {
		return nil
	}
	if st.Size() < t.off {
		t.off, t.info = 0, transcriptInfo{}
	}
	if st.Size() > t.off {
		t.read()
	}
	t.mod = st.ModTime()
	return t
}

func (t *transcript) read() {
	f, err := os.Open(t.path)
	if err != nil {
		return
	}
	defer f.Close()
	f.Seek(t.off, io.SeekStart)
	rd := bufio.NewReaderSize(f, 64<<10)
	head := []byte(`{"type":"`)
	for {
		line, err := rd.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			t.off += int64(len(line))
			for err == bufio.ErrBufferFull {
				line, err = rd.ReadSlice('\n')
				t.off += int64(len(line))
			}
			if err != nil {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		t.off += int64(len(line))
		if t.codex {
			t.readCodex(line)
			continue
		}
		if !bytes.HasPrefix(line, head) {
			continue
		}
		var e struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			AgentName   string `json:"agentName"`
			AgentColor  string `json:"agentColor"`
			AITitle     string `json:"aiTitle"`
			LastPrompt  string `json:"lastPrompt"`
		}
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		switch e.Type {
		case "custom-title":
			t.info.Name = e.CustomTitle
		case "agent-name":
			if e.AgentName != "" {
				t.info.Name = e.AgentName
			}
		case "agent-color":
			t.info.Color = e.AgentColor
		case "ai-title":
			t.info.Title = e.AITitle
		case "last-prompt":
			t.info.Prompt = e.LastPrompt
		}
	}
}

func (t *transcript) readCodex(line []byte) {
	if !bytes.Contains(line, []byte(`"user_message"`)) && !bytes.Contains(line, []byte(`"role":"user"`)) {
		return
	}
	var e struct {
		Payload struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Message string `json:"message"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	p := e.Payload
	switch {
	case p.Type == "user_message" && p.Message != "":
		t.info.Prompt = p.Message
	case p.Role == "user" && len(p.Content) > 0:

		if x := p.Content[0].Text; x != "" && !strings.HasPrefix(x, "#") && !strings.HasPrefix(x, "<") {
			t.info.Prompt = x
		}
	}
}

func codexHome() string {
	if d := os.Getenv("CODEX_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

type codexThread struct {
	ID, Cwd, Started, Path string
	Exec                   bool
	Parent                 string
	Review                 bool
}

var (
	codexMu    sync.Mutex
	codexMetas = map[string]*codexThread{}
)

func codexThreads() []codexThread {
	codexMu.Lock()
	defer codexMu.Unlock()
	var out []codexThread
	week := map[string]*codexThread{}
	for d := 0; d < 7; d++ {
		day := time.Now().AddDate(0, 0, -d).Format("2006/01/02")
		files, _ := filepath.Glob(filepath.Join(codexHome(), "sessions", filepath.FromSlash(day), "rollout-*.jsonl"))
		for _, f := range files {
			m, ok := codexMetas[f]
			if !ok {
				m = readCodexMeta(f)
			}
			week[f] = m
			if m != nil {
				out = append(out, *m)
			}
		}
	}
	codexMetas = week
	return out
}

func readCodexMeta(path string) *codexThread {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	line, err := bufio.NewReaderSize(f, 64<<10).ReadSlice('\n')
	if err != nil && len(line) == 0 {
		return nil
	}
	var e struct {
		Type    string `json:"type"`
		Payload struct {
			ID         string `json:"id"`
			Cwd        string `json:"cwd"`
			Timestamp  string `json:"timestamp"`
			Originator string `json:"originator"`
			Source     string `json:"thread_source"`
			Parent     string `json:"parent_thread_id"`
		} `json:"payload"`
	}

	if json.Unmarshal(line, &e) != nil {
		e.Payload.ID = jsonField(line, "id")
		e.Payload.Cwd = jsonField(line, "cwd")
		e.Payload.Timestamp = jsonField(line, "timestamp")
		e.Payload.Originator = jsonField(line, "originator")
		e.Payload.Source = jsonField(line, "thread_source")
		e.Payload.Parent = jsonField(line, "parent_thread_id")
	}
	if e.Payload.ID == "" || e.Payload.Cwd == "" {
		return nil
	}
	return &codexThread{ID: e.Payload.ID, Cwd: e.Payload.Cwd, Started: localTime(e.Payload.Timestamp), Path: path, Exec: e.Payload.Originator == "codex_exec",
		Parent: e.Payload.Parent, Review: strings.Contains(e.Payload.Source, "review")}
}

func jsonField(line []byte, name string) string {
	key := []byte(`"` + name + `":"`)
	i := bytes.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := line[i+len(key):]
	j := bytes.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return string(rest[:j])
}

func codexRollout(id string) string {
	for _, t := range codexThreads() {
		if t.ID == id {
			return t.Path
		}
	}
	return ""
}

func codexNames() map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(filepath.Join(codexHome(), "session_index.jsonl"))
	if err != nil {
		return out
	}
	for _, l := range bytes.Split(b, []byte("\n")) {
		var e struct {
			ID   string `json:"id"`
			Name string `json:"thread_name"`
		}
		if json.Unmarshal(l, &e) == nil && e.ID != "" && e.Name != "" {
			out[e.ID] = e.Name
		}
	}
	return out
}

func localTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.Local().Format(time.RFC3339)
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

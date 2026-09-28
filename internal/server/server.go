package server

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/overview"
	"github.com/AsWali/CallBoard/internal/setup"
	"github.com/AsWali/CallBoard/internal/store"
)

//go:embed web
var web embed.FS

func DefaultPort(root string) int {
	h := fnv.New32a()
	h.Write([]byte(root))
	return 4700 + int(h.Sum32()%300)
}

type Section struct {
	Name  string       `json:"name"`
	Level int          `json:"level"`
	Items []store.Item `json:"items"`
}

type List struct {
	Kind     string        `json:"kind"`
	File     string        `json:"file"`
	Own      bool          `json:"own,omitempty"`
	Exists   bool          `json:"exists"`
	Open     int           `json:"open"`
	Done     int           `json:"done"`
	Sections []Section     `json:"sections"`
	Merges   []store.Merge `json:"merges"`

	Fields map[string][]string `json:"fields"`
}

type Board struct {
	App      string        `json:"app"`
	Repo     string        `json:"repo"`
	Branch   string        `json:"branch"`
	Worktree string        `json:"worktree"`
	File     string        `json:"file"`
	ReadOnly bool          `json:"readOnly,omitempty"`
	Off      bool          `json:"off,omitempty"`
	Lists    []List        `json:"lists"`
	Last     *store.Event  `json:"last"`
	Agents   []store.Agent `json:"agents"`
	Claims   []store.Claim `json:"claims"`
	Branches []string      `json:"branches"`
	MainName string        `json:"main"`

	Views []store.BoardView `json:"views"`

	Touched map[string]store.Touch `json:"touched"`
}

func sections(v *store.View, k store.Kind, f *board.File) []Section {
	var out []Section
	byName := map[string]int{}
	if len(f.Tasks) > 0 && f.Tasks[0].Section == "" {
		out = append(out, Section{Name: "", Items: []store.Item{}})
		byName[""] = 0
	}
	hasSub := false
	for _, h := range f.Headings() {
		hasSub = hasSub || h.Level > 1
	}
	for _, h := range f.Headings() {
		if _, dup := byName[h.Name]; dup {
			continue
		}
		byName[h.Name] = len(out)
		out = append(out, Section{Name: h.Name, Level: h.Level, Items: []store.Item{}})
	}
	merges := store.MergesIn(k, f)
	for _, t := range f.Tasks {
		if store.InMerge(merges, t.Key) {
			continue
		}
		s := &out[byName[t.Section]]
		s.Items = append(s.Items, v.Item(k, t))
	}

	kept := out[:0]
	for _, s := range out {
		if !(s.Level == 1 && hasSub && len(s.Items) == 0) {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		kept = []Section{{Name: "", Items: []store.Item{}}}
	}
	return kept
}

func BoardOf(st *store.Store) (Board, error) {
	v, err := st.View()
	if err != nil {
		return Board{}, err
	}
	b := Board{App: "callboard", Repo: st.Name, Branch: st.Branch(), Worktree: st.Root, File: st.Path, Agents: st.Agents(8), Claims: st.ClaimList(), Branches: []string{}, Views: st.SavedViews()}
	b.Off = st.Off()
	b.ReadOnly = b.Off
	if b.Views == nil {
		b.Views = []store.BoardView{}
	}
	if b.Agents == nil {
		b.Agents = []store.Agent{}
	}
	if b.Claims == nil {
		b.Claims = []store.Claim{}
	}
	b.Lists = lists(v, func(k store.Kind) bool { return fileExists(st.File(k)) && !st.Foreign(k) })
	var open []string
	for _, l := range b.Lists {
		for _, sec := range l.Sections {
			for _, it := range sec.Items {
				if !it.Done && it.Key != "" {
					open = append(open, it.Key)
				}
			}
		}
	}
	b.Touched = st.Touched(open)
	for _, e := range st.Events(300) {
		e := e
		switch {
		case e.What == "session" || e.What == "claimed" || e.What == "released":
		case b.Last == nil:
			b.Last = &e
		}
	}
	if st.Common != "" {
		b.MainName = st.DefaultBranch()
		for _, br := range st.Branches() {
			if br != b.Branch {
				b.Branches = append(b.Branches, br)
			}
		}
	}
	return b, nil
}

func BranchBoard(st *store.Store, branch string) (Board, error) {
	known := false
	for _, b := range st.Branches() {
		known = known || b == branch
	}
	if !known {
		return Board{}, fmt.Errorf("no branch %q", branch)
	}
	wt := st.WorktreeBranches()[branch]
	if wt != "" {
		b, err := BoardOf(store.Open(wt))
		b.ReadOnly = true
		return b, err
	}
	v := store.NewView(st, map[string]*board.File{}, st.Claims())
	b := Board{App: "callboard", Repo: st.Name, Branch: branch, ReadOnly: true, Agents: []store.Agent{}, Claims: []store.Claim{}, Branches: []string{}, Views: []store.BoardView{}, Touched: map[string]store.Touch{}}
	for _, k := range st.Lists() {
		f := board.ParseAs(overview.Read(st.Repo, branch, "", k), k.Prefix)
		f.Prefix = k.Prefix
		v.Files[k.Name] = f
	}
	b.Lists = lists(v, func(k store.Kind) bool { return v.Files[k.Name].String() != "" })
	return b, nil
}

func lists(v *store.View, exists func(store.Kind) bool) []List {
	var out []List
	for _, k := range v.Lists() {
		f := v.Files[k.Name]
		l := List{Kind: k.Name, File: k.File, Own: k.Own(), Exists: exists(k), Sections: sections(v, k, f), Merges: orEmpty(store.MergesIn(k, f)), Fields: store.FieldsInUse(f)}
		l.Open, l.Done = f.Counts()
		out = append(out, l)
	}
	return out
}

func orEmpty(m []store.Merge) []store.Merge {
	if m == nil {
		return []store.Merge{}
	}
	return m
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

type Server struct {
	st      *store.Store
	mu      sync.Mutex
	clients map[chan [2]string]bool
}

func New(dir string) *Server {
	return &Server{st: store.Open(dir), clients: map[chan [2]string]bool{}}
}

func (s *Server) Store() *store.Store { return s.st }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", staticFiles())
	mux.HandleFunc("GET /api/board", s.board)
	mux.HandleFunc("GET /api/branch", s.branch)
	mux.HandleFunc("GET /api/branches", s.branches)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/log", s.log)
	mux.HandleFunc("GET /api/agents", s.agents)
	mux.HandleFunc("POST /api/items", s.add)
	mux.HandleFunc("POST /api/tasks", s.add)
	mux.HandleFunc("PATCH /api/items/{key}", s.change)
	mux.HandleFunc("PATCH /api/tasks/{key}", s.change)
	mux.HandleFunc("POST /api/items/{key}/answer", s.answer)
	mux.HandleFunc("POST /api/items/{key}/reopen", s.reopen)
	mux.HandleFunc("POST /api/items/{key}/resolve", s.resolve)
	mux.HandleFunc("DELETE /api/items/{key}", s.deleteItem)
	mux.HandleFunc("POST /api/items/{key}/restore", s.restore)
	mux.HandleFunc("POST /api/views", s.saveView)
	mux.HandleFunc("PATCH /api/views/{key}", s.saveView)
	mux.HandleFunc("DELETE /api/views/{key}", s.deleteView)
	mux.HandleFunc("POST /api/sections", s.section)
	mux.HandleFunc("POST /api/lists", s.addList)
	mux.HandleFunc("PATCH /api/lists/{name}", s.renameList)
	mux.HandleFunc("DELETE /api/lists/{name}", s.removeList)
	return sameOrigin(mux)
}

func staticFiles() http.Handler {
	static, _ := fs.Sub(web, "web")
	tags := map[string]string{}
	fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if b, err := fs.ReadFile(static, p); err == nil && !d.IsDir() {
			sum := sha256.Sum256(b)
			tags["/"+p] = fmt.Sprintf(`"%x"`, sum[:8])
		}
		return nil
	})
	tags["/"] = tags["/index.html"]
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if t, ok := tags[r.URL.Path]; ok {
			w.Header().Set("ETag", t)
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func localHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
}

func sameOrigin(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localHost(r.Host) {
			http.Error(w, "this page answers only on localhost", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					http.Error(w, "cross-site request refused", http.StatusForbidden)
					return
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	var changed board.ErrChanged
	if errors.As(err, &changed) {
		code = http.StatusConflict
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(v)
}

var you = store.Actor{By: "you"}

func (s *Server) board(w http.ResponseWriter, r *http.Request) {
	b, err := BoardOf(s.st)
	if err != nil {
		fail(w, 500, err)
		return
	}
	writeJSON(w, 200, b)
}

func (s *Server) branch(w http.ResponseWriter, r *http.Request) {
	b, err := BranchBoard(s.st, r.URL.Query().Get("name"))
	if err != nil {
		fail(w, 404, err)
		return
	}
	writeJSON(w, 200, b)
}

func (s *Server) branches(w http.ResponseWriter, r *http.Request) {
	if s.st.Common == "" {
		fail(w, 404, errors.New("not a git repo: there are no branches to compare"))
		return
	}
	writeJSON(w, 200, overview.BuildBranches(s.st.Repo, s.st.Branch()))
}

func (s *Server) log(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	ev := s.st.Events(limit)
	if key := r.URL.Query().Get("key"); key != "" {
		ev = s.st.ItemEvents(key, limit)
	}
	if ev == nil {
		ev = []store.Event{}
	}
	writeJSON(w, 200, ev)
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	a := s.st.Agents(8)
	if a == nil {
		a = []store.Agent{}
	}
	writeJSON(w, 200, a)
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind, Title, Section, Parent string
		Body, Options, Needs         []string
		Recommended                  int
		Fields                       map[string]string
	}
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"title": "…", "kind": "task", "section": "…"}`))
		return
	}
	k, err := s.st.KindNamed(in.Kind)
	if err != nil {
		fail(w, 400, err)
		return
	}
	kv := store.FieldList(in.Fields)
	for _, f := range kv {
		if err := board.CheckField(f[0]); err != nil {
			fail(w, 400, err)
			return
		}
		if _, err := board.CleanValue(f[1]); err != nil {
			fail(w, 400, err)
			return
		}
	}
	body := []string{}
	for i, o := range in.Options {
		if o = strings.TrimSpace(o); o != "" {
			if i+1 == in.Recommended {
				o += " (recommended)"
			}
			body = append(body, "- "+o)
		}
	}
	body = append(body, in.Body...)
	t, err := s.st.Add(k, board.New{Title: in.Title, Section: in.Section, Body: body, Needs: in.Needs, Parent: in.Parent}, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	if len(kv) > 0 {
		key := t.Key
		if t, err = s.st.SetFields(key, kv, you); err != nil {
			fail(w, 400, fmt.Errorf("added %s, but its fields weren't set: %w", key, err))
			return
		}
	}
	writeJSON(w, 201, map[string]string{"key": t.Key, "version": t.Version, "line": t.Raw})
}

func (s *Server) change(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Done    *bool
		Title   *string
		Needs   *[]string
		Fields  map[string]string
		Section *string
		Before  string
		After   string
		Version string
	}
	if err := decode(r, &in); err != nil || (in.Done == nil && in.Title == nil && in.Needs == nil && len(in.Fields) == 0 && in.Section == nil && in.Before == "" && in.After == "") {
		fail(w, 400, fmt.Errorf(`send JSON: {"done": true}, {"title": "…"}, {"needs": ["Q-7x1c"]}, {"fields": {"status": "doing"}}, {"section": "Later"} or {"before": "B-k3f9"} / {"after": "B-k3f9"}, with "version" if you have it`))
		return
	}
	key := r.PathValue("key")
	ref := key
	if in.Version != "" {
		ref = key + "@" + in.Version
	}

	var steps []func(ref string) (*board.Task, error)
	if in.Title != nil {
		steps = append(steps, func(ref string) (*board.Task, error) { return s.st.Rename(ref, *in.Title, you) })
	}
	if in.Needs != nil {
		steps = append(steps, func(ref string) (*board.Task, error) { return s.st.SetNeeds(ref, *in.Needs, you) })
	}
	if len(in.Fields) > 0 {
		steps = append(steps, func(ref string) (*board.Task, error) { return s.st.SetFields(ref, store.FieldList(in.Fields), you) })
	}
	if in.Section != nil {
		steps = append(steps, func(ref string) (*board.Task, error) { return s.st.Move(ref, *in.Section, you) })
	}
	if in.Before != "" || in.After != "" {
		steps = append(steps, func(ref string) (*board.Task, error) {
			if in.Before != "" {
				return s.st.Place(ref, in.Before, false, you)
			}
			return s.st.Place(ref, in.After, true, you)
		})
	}
	var t *board.Task
	var err error
	for _, step := range steps {
		if t, err = step(ref); err != nil {
			fail(w, 400, err)
			return
		}
		ref = key
	}
	if in.Done != nil {
		if k, _ := store.KindOf(key); k == store.Requests {
			t, _, err = store.Everywhere(s.st, ref, func(w *store.Store) (*board.Task, error) { return w.Tick(ref, *in.Done, you) })
		} else {
			t, err = s.st.Tick(ref, *in.Done, you)
		}
		if err != nil {
			fail(w, 404, err)
			return
		}
	}
	fields := map[string]string{}
	for _, kv := range t.Fields() {
		fields[kv[0]] = kv[1]
	}
	writeJSON(w, 200, map[string]any{"key": t.Key, "done": t.Done, "title": t.Title, "section": t.Section, "fields": fields, "version": t.Version})
}

func (s *Server) saveView(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name, List, Layout, Group *string
		Order, Sort, Filter, Show *[]string
		Keep                      bool
	}
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"name": "By status", "list": "backlog", "layout": "board", "group": "status"}`))
		return
	}
	key := r.PathValue("key")
	if key == "" && (in.Name == nil || strings.TrimSpace(*in.Name) == "") {
		fail(w, 400, errors.New("a new view needs a name"))
		return
	}
	v, err := s.st.SaveView(store.ViewSpec{Key: key, Name: in.Name, List: in.List, Layout: in.Layout, Group: in.Group, Order: in.Order, Sort: in.Sort, Filter: in.Filter, Show: in.Show, Keep: in.Keep}, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	code := 200
	if key == "" {
		code = 201
	}
	writeJSON(w, code, v)
}

func (s *Server) deleteView(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteView(r.PathValue("key"), you); err != nil {
		fail(w, 404, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"removed": true})
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	t, freed, err := s.st.Delete(r.PathValue("key"), you)
	if err != nil {
		fail(w, 404, err)
		return
	}
	if freed == nil {
		freed = []string{}
	}
	writeJSON(w, 200, map[string]any{"key": t.Key, "title": t.Title, "freed": freed})
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	t, err := s.st.Restore(r.PathValue("key"), you)
	if err != nil {
		fail(w, 404, err)
		return
	}
	writeJSON(w, 200, map[string]string{"key": t.Key, "version": t.Version})
}

func (s *Server) section(w http.ResponseWriter, r *http.Request) {
	var in store.SectionChange
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"kind": "task", "name": "Later", and "to", "before", "after" or "remove"}`))
		return
	}
	msg, err := s.st.ChangeSection(in, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"done": msg})
}

func (s *Server) addList(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"name": "ideas"}`))
		return
	}
	k, _, err := s.st.AddList(in.Name, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"kind": k.Name, "file": k.File, "prefix": k.Prefix})
}

func (s *Server) renameList(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"name": "someday"}`))
		return
	}
	k, err := s.st.RenameList(r.PathValue("name"), in.Name, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"kind": k.Name, "file": k.File, "prefix": k.Prefix})
}

func (s *Server) removeList(w http.ResponseWriter, r *http.Request) {
	k, views, err := s.st.RemoveList(r.PathValue("name"), r.URL.Query().Get("force") == "1", you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"kind": k.Name, "file": k.File, "views": views})
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var in struct{ Answer, Why, Version string }
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"answer": "1" or "your words", "why": "…"}`))
		return
	}
	key := r.PathValue("key")
	ref := key
	if in.Version != "" {
		ref = key + "@" + in.Version
	}
	res, where, err := store.Everywhere(s.st, key, func(w *store.Store) (store.Answered, error) {
		if w == s.st {
			return w.AnswerQuestion(ref, in.Answer, in.Why, you)
		}
		return w.AnswerQuestion(key, in.Answer, in.Why, you)
	})
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"answer": res.Answer, "why": res.Why, "version": res.Version, "unblocked": res.Unblocked, "reopened": res.Reopened, "assumed": res.Assumed, "branches": where})
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var in struct{ Keep int }
	if err := decode(r, &in); err != nil {
		fail(w, 400, fmt.Errorf(`send JSON: {"keep": 1} or {"keep": 2}`))
		return
	}
	kept, err := s.st.Resolve(r.PathValue("key"), in.Keep, you)
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]any{"kept": len(kept)})
}

func (s *Server) reopen(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	t, _, err := store.Everywhere(s.st, key, func(w *store.Store) (*board.Task, error) { return w.Reopen(key, you) })
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, map[string]string{"key": t.Key, "version": t.Version})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	ch := make(chan [2]string, 16)
	s.mu.Lock()
	s.clients[ch] = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.clients, ch); s.mu.Unlock() }()
	fmt.Fprint(w, "retry: 1000\nevent: hello\ndata: {}\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", m[0], m[1])
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) broadcast(event, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- [2]string{event, data}:
		default:
		}
	}
}

func (s *Server) watch(ctx context.Context) {
	logPath := filepath.Join(s.st.Shared(), "events.jsonl")
	var logOff int64
	if st, err := os.Stat(logPath); err == nil {
		logOff = st.Size()
	}
	last := s.st.Stamp()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if now := s.st.Stamp(); now != last {
			last = now

			for _, l := range setup.LineMerged(s.st.Repo) {
				log.Print(l)
			}
			s.st.Reconcile()
			s.broadcast("board", "{}")
		}
		st, err := os.Stat(logPath)
		if err != nil || st.Size() == logOff {
			continue
		}
		if st.Size() < logOff {
			logOff = st.Size()
			continue
		}
		f, err := os.Open(logPath)
		if err != nil {
			continue
		}
		f.Seek(logOff, io.SeekStart)
		data, _ := io.ReadAll(f)
		f.Close()
		for {
			i := strings.IndexByte(string(data), '\n')
			if i < 0 {
				break
			}
			line := data[:i+1]
			data = data[i+1:]
			logOff += int64(len(line))
			var e store.Event
			if json.Unmarshal(line, &e) == nil && e.Worktree == s.st.Root {
				s.broadcast("change", strings.TrimSpace(string(line)))
			}
		}
	}
}

func Probe(port int, root string) string {
	c := http.Client{Timeout: 700 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/board", port))
	if err != nil {
		ln, lerr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if lerr != nil {
			return "other"
		}
		ln.Close()
		return "free"
	}
	defer resp.Body.Close()
	var b Board
	if json.NewDecoder(resp.Body).Decode(&b) == nil && b.App == "callboard" && b.Worktree == root {
		return "ours"
	}
	return "other"
}

func (s *Server) Serve(ctx context.Context, port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	go s.watch(ctx)
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

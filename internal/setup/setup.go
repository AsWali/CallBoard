package setup

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/AsWali/CallBoard/internal/gitx"
	"github.com/AsWali/CallBoard/internal/store"
)

const (
	driverCmd = "if callboard merge --rules " + rulesVer + " >/dev/null 2>&1; then callboard merge %O %A %B %P %X %Y; " +
		"else git merge-file -L %X'" + lineMark + "' -L base -L %Y'" + lineMark + "' %A %O %B; fi"
	rulesVer   = "2"
	lineMark   = " (merged line by line, without callboard)"
	serverName = "callboard"
)

var hooks = [][2]string{
	{"SessionStart", "callboard prime"},
	{"UserPromptSubmit", "callboard hook prompt"},
	{"PostToolUse", "callboard hook tool"},
	{"PreToolUse", "callboard hook read"},
	{"SubagentStart", "callboard hook subagent"},
}

var attrLines = []string{
	"backlog.md merge=callboard",
	"requests.md merge=callboard",
	"questions.md merge=callboard",
	".callboard/views.md merge=callboard",
}

var oldAttrLines = []string{".callboard/answers.md merge=union"}

type Step struct {
	W   io.Writer
	Err bool
}

func (s *Step) ok(format string, a ...any)   { fmt.Fprintf(s.W, "  ✓ "+format+"\n", a...) }
func (s *Step) same(format string, a ...any) { fmt.Fprintf(s.W, "  · "+format+"\n", a...) }
func (s *Step) bad(format string, a ...any) {
	s.Err = true
	fmt.Fprintf(s.W, "  ✗ "+format+"\n", a...)
}

func LinkPath() string {
	home, _ := os.UserHomeDir()
	name := "callboard"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".local", "bin", name)
}

func linkBinary(s *Step, exe string) {
	link := LinkPath()
	fi, err := os.Lstat(link)
	switch {
	case err == nil && fi.Mode()&os.ModeSymlink != 0:
		if cur, _ := os.Readlink(link); cur == exe {
			s.same("%s already points here", tilde(link))
			return
		}
		os.Remove(link)
	case err == nil:
		if sameContent(link, exe) {
			s.same("%s is this build", tilde(link))
		} else {
			s.same("%s is an installed callboard; the hooks use it (callboard install puts this build there)", tilde(link))
		}
		return
	}
	os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(exe, link); err == nil {
		s.ok("linked %s → %s (undo: callboard uninstall)", tilde(link), tilde(exe))
	} else if err := copyFile(exe, link); err == nil {
		s.ok("copied callboard to %s (undo: callboard uninstall)", tilde(link))
	} else {
		s.bad("couldn't put callboard at %s: %v", tilde(link), err)
	}
}

func Install(w io.Writer, exe string) error {
	s := &Step{W: w}
	link := LinkPath()
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink == 0 && sameContent(link, exe) {
		s.same("%s is already this build", tilde(link))
	} else {
		os.MkdirAll(filepath.Dir(link), 0o755)
		os.Remove(link)
		if err := copyFile(exe, link); err != nil {
			s.bad("couldn't copy callboard to %s: %v", tilde(link), err)
			return err
		}
		s.ok("installed callboard at %s (undo: callboard uninstall)", tilde(link))
	}
	pathHint(s, filepath.Dir(link))
	return nil
}

func Uninstall(w io.Writer) error {
	s := &Step{W: w}
	link := LinkPath()
	if _, err := os.Lstat(link); err != nil {
		s.same("%s isn't there", tilde(link))
		return nil
	}
	if err := os.Remove(link); err != nil {
		s.bad("couldn't remove %s: %v", tilde(link), err)
		return err
	}
	s.ok("removed %s; projects set up with callboard setup stop working until it's back (callboard install)", tilde(link))
	if readString(StrategyPath()) == strategyScript {
		s.same("left %s: without callboard it hands merges of several branches to git's own octopus", tilde(StrategyPath()))
	}
	return nil
}

func pathHint(s *Step, dir string) {
	if onPath(dir) {
		return
	}
	if _, err := exec.LookPath("callboard"); err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		s.bad("%s isn't on your PATH, so the hooks can't find callboard. Add it: setx PATH \"%%PATH%%;%s\", then open a new terminal", dir, dir)
	} else {
		s.bad("%s isn't on your PATH, so the hooks can't find callboard. Add it: export PATH=\"$HOME/.local/bin:$PATH\"", tilde(dir))
	}
}

func sameContent(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	tmp := to + ".callboard-tmp"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, to)
}

func Claude(w io.Writer, r gitx.Repo, exe string) error {
	s := &Step{W: w}
	root := r.Root

	linkBinary(s, exe)
	pathHint(s, filepath.Dir(LinkPath()))

	changed, err := editJSON(filepath.Join(root, ".mcp.json"), func(m *Obj) {
		srv := newObj()
		srv.Set("command", "callboard")
		srv.Set("args", []any{"mcp"})
		if cur := m.Child("mcpServers").ChildIf(serverName); cur == nil || cur.Get("command") != "callboard" {
			m.Child("mcpServers").Set(serverName, srv)
		}
	})
	report(s, changed, err, ".mcp.json: MCP server \"callboard\" (callboard mcp)")

	changed, err = editJSON(filepath.Join(root, ".claude", "settings.json"), addHooks)
	report(s, changed, err, ".claude/settings.json: hooks for session start, prompts, tools, reads of the lists and subagents")

	changed, err = editJSON(filepath.Join(root, ".claude", "settings.local.json"), func(m *Obj) {
		list := m.List("enabledMcpjsonServers")
		if !slices.Contains(list, any(serverName)) {
			m.Set("enabledMcpjsonServers", append(list, serverName))
		}
	})
	report(s, changed, err, ".claude/settings.local.json: callboard server allowed without asking (yours; undo: callboard disconnect)")

	if r.Common == "" {
		s.same("not a git repo: skipped the merge driver")
	} else {
		cur, _ := r.Git("config", "--get", "merge.callboard.driver")
		if cur == driverCmd {
			s.same("git merge driver already set")
		} else if !setDriver(r) {
			s.bad("couldn't set the git merge driver: git config merge.callboard.driver %q", driverCmd)
		} else {
			s.ok("git config: merge driver \"callboard\" for this repo")
		}
		setOctopus(s, localConfig(r), "this repo's git config")
		changed, err = editLines(filepath.Join(root, ".gitattributes"), func(lines []string) []string {
			lines = slices.DeleteFunc(lines, func(l string) bool { return slices.Contains(oldAttrLines, l) })
			for _, a := range append(slices.Clone(attrLines), ownAttrLines(r)...) {
				if !slices.Contains(lines, a) {
					lines = append(lines, a)
				}
			}
			return lines
		})
		report(s, changed, err, ".gitattributes: the lists and views merge per item")
		for _, l := range Remerge(r) {
			s.ok("%s", strings.TrimPrefix(l, "callboard: "))
		}
	}
	if s.Err {
		return fmt.Errorf("setup didn't finish; see ✗ above")
	}
	return nil
}

func Disconnect(w io.Writer, r gitx.Repo, exe string) error {
	s := &Step{W: w}
	root := r.Root
	changed, err := editJSON(filepath.Join(root, ".mcp.json"), func(m *Obj) {
		if servers := m.ChildIf("mcpServers"); servers != nil {
			servers.Delete(serverName)
			if servers.Len() == 0 {
				m.Delete("mcpServers")
			}
		}
	})
	reportOff(s, changed, err, ".mcp.json: removed the callboard server")
	changed, err = editJSON(filepath.Join(root, ".claude", "settings.json"), dropHooks)
	reportOff(s, changed, err, ".claude/settings.json: removed the hooks")
	changed, err = editJSON(filepath.Join(root, ".claude", "settings.local.json"), func(m *Obj) {
		list := m.List("enabledMcpjsonServers")
		if !slices.Contains(list, any(serverName)) {
			return
		}
		list = slices.DeleteFunc(slices.Clone(list), func(v any) bool { return v == serverName })
		if len(list) == 0 {
			m.Delete("enabledMcpjsonServers")
		} else {
			m.Set("enabledMcpjsonServers", list)
		}
	})
	reportOff(s, changed, err, ".claude/settings.local.json: removed the callboard permission")
	if r.Common != "" {
		if cur, _ := r.Git("config", "--get", "merge.callboard.driver"); cur != "" {
			r.Git("config", "--remove-section", "merge.callboard")
			s.ok("git config: removed the merge driver")
		}
		dropOctopus(s, func(args ...string) (string, error) { return r.Git(append([]string{"config", "--local"}, args...)...) }, "this repo's git config")
		changed, err = editLines(filepath.Join(root, ".gitattributes"), func(lines []string) []string {
			own := ownAttrLines(r)
			return slices.DeleteFunc(lines, func(l string) bool {
				return slices.Contains(attrLines, l) || slices.Contains(oldAttrLines, l) || slices.Contains(own, l)
			})
		})
		reportOff(s, changed, err, ".gitattributes: removed the Callboard merge lines")
	}
	disconnectCodex(s, filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".codex"), relTo(root))
	s.same("left %s, since other projects may use it (callboard uninstall removes it)", tilde(LinkPath()))
	fmt.Fprintln(w, "  The lists and the event log stay. Undo this: callboard setup")
	return nil
}

func Status(r gitx.Repo) (done bool, missing []string) {
	root := r.Root
	m, _ := readJSON(filepath.Join(root, ".mcp.json"))
	if m == nil || m.Child("mcpServers").ChildIf(serverName) == nil {
		missing = append(missing, ".mcp.json")
	}
	m, _ = readJSON(filepath.Join(root, ".claude", "settings.json"))
	for _, h := range hooks {
		if m == nil || !hasHook(m, h[0], h[1]) {
			missing = append(missing, h[0]+" hook")
		}
	}
	m, _ = readJSON(filepath.Join(root, ".claude", "settings.local.json"))
	if m == nil || !slices.Contains(m.List("enabledMcpjsonServers"), any(serverName)) {
		missing = append(missing, "settings.local.json")
	}
	if _, err := os.Stat(LinkPath()); err != nil {
		missing = append(missing, "callboard on PATH")
	}
	if r.Common != "" {
		if cur, _ := r.Git("config", "--get", "merge.callboard.driver"); cur != driverCmd {
			missing = append(missing, "merge driver")
		}
		if !octopusReady(localConfig(r)) {
			missing = append(missing, "merges of several branches")
		}
		b, _ := os.ReadFile(filepath.Join(root, ".gitattributes"))
		for _, a := range attrLines {
			if !slices.Contains(strings.Split(string(b), "\n"), a) {
				missing = append(missing, ".gitattributes")
				break
			}
		}
	}
	return len(missing) == 0, missing
}

func report(s *Step, changed bool, err error, what string) {
	switch {
	case err != nil:
		s.bad("%s: %v", strings.SplitN(what, ":", 2)[0], err)
	case changed:
		s.ok("%s", what)
	default:
		s.same("%s (already there)", strings.SplitN(what, " (", 2)[0])
	}
}

func reportOff(s *Step, changed bool, err error, what string) {
	switch {
	case err != nil:
		s.bad("%s: %v", strings.SplitN(what, ":", 2)[0], err)
	case changed:
		s.ok("%s", what)
	}
}

func addHooks(m *Obj) {
	for _, h := range hooks {
		if hasHook(m, h[0], h[1]) {
			continue
		}
		hs := m.Child("hooks")
		cmd := newObj()
		cmd.Set("type", "command")
		cmd.Set("command", h[1])
		entry := newObj()
		entry.Set("hooks", []any{cmd})
		hs.Set(h[0], append(hs.List(h[0]), entry))
	}
}

func dropHooks(m *Obj) {
	hs := m.ChildIf("hooks")
	if hs == nil {
		return
	}
	for _, h := range hooks {
		list := hs.List(h[0])
		if list == nil {
			continue
		}
		keep := slices.DeleteFunc(slices.Clone(list), func(e any) bool { return isOurHook(e, h[1]) })
		if len(keep) == 0 {
			hs.Delete(h[0])
		} else if len(keep) != len(list) {
			hs.Set(h[0], keep)
		}
	}
	if hs.Len() == 0 {
		m.Delete("hooks")
	}
}

func hasHook(m *Obj, event, cmd string) bool {
	hs := m.ChildIf("hooks")
	if hs == nil {
		return false
	}
	return slices.ContainsFunc(hs.List(event), func(e any) bool { return isOurHook(e, cmd) })
}

func isOurHook(e any, cmd string) bool {
	em, _ := e.(*Obj)
	if em == nil {
		return false
	}
	for _, h := range em.List("hooks") {
		if hm, _ := h.(*Obj); hm != nil {
			if c, _ := hm.Get("command").(string); strings.Contains(c, cmd) {
				return true
			}
		}
	}
	return false
}

func readJSON(path string) (*Obj, error) {
	o, _, err := readJSONRaw(path)
	return o, err
}

func readJSONRaw(path string) (*Obj, []byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newObj(), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return newObj(), b, nil
	}
	o, err := parseObj(b)
	if err != nil {
		return nil, nil, fmt.Errorf("%s isn't valid JSON (%v); fix it and run again", filepath.Base(path), err)
	}
	return o, b, nil
}

func editJSON(path string, fn func(*Obj)) (bool, error) {
	o, raw, err := readJSONRaw(path)
	if err != nil {
		return false, err
	}
	indent := indentOf(raw)
	var before strings.Builder
	encode(&before, o, indent, "")
	fn(o)
	var after strings.Builder
	encode(&after, o, indent, "")
	if before.String() == after.String() {
		return false, nil
	}
	if o.Len() == 0 {
		return true, os.Remove(path)
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	return true, os.WriteFile(path, []byte(after.String()+"\n"), 0o644)
}

func editLines(path string, fn func([]string) []string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	var lines []string
	if s := strings.TrimRight(string(b), "\n"); s != "" {
		lines = strings.Split(s, "\n")
	}
	out := fn(slices.Clone(lines))
	if slices.Equal(out, lines) {
		return false, nil
	}
	if len(out) == 0 {
		return true, os.Remove(path)
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	return true, os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == filepath.Clean(dir) {
			return true
		}
	}
	return false
}

func tilde(p string) string {
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func EnsureDriver(r gitx.Repo) []string {
	if r.Common == "" {
		return nil
	}
	attrs, err := os.ReadFile(filepath.Join(r.Root, ".gitattributes"))
	if err != nil || !strings.Contains(string(attrs), "merge=callboard") {
		return nil
	}
	cur, _ := r.Git("config", "--get", "merge.callboard.driver")
	if cur == driverCmd {
		return append(ensureOctopus(r), LineMerged(r)...)
	}
	if !setDriver(r) {
		return []string{fmt.Sprintf("callboard: couldn't set the merge driver; run: git config merge.callboard.driver %q", driverCmd)}
	}
	var out []string
	if cur == "" {
		out = append(out, "callboard: set the merge driver in this clone's git config, so the lists merge per item (undo: callboard disconnect)")
	} else {
		out = append(out, "callboard: updated the merge driver in git config, so merges follow this callboard's rules")
	}
	return append(append(out, ensureOctopus(r)...), Remerge(r)...)
}

func localConfig(r gitx.Repo) gitConfig {
	return func(args ...string) (string, error) { return r.Git(append([]string{"config"}, args...)...) }
}

func ensureOctopus(r gitx.Repo) []string {
	cfg := localConfig(r)
	if octopusReady(cfg) {
		return nil
	}
	if cur, _ := cfg("--get", "pull.octopus"); cur != "" && cur != "callboard" || !onPath(filepath.Dir(StrategyPath())) {
		return nil
	}
	if _, err := writeStrategy(); err != nil {
		return nil
	}
	if cur, _ := cfg("--get", "pull.octopus"); cur == "callboard" {
		return nil
	}
	if _, err := cfg("pull.octopus", "callboard"); err != nil {
		return nil
	}
	return []string{"callboard: merges of several branches at once (git merge a b) now merge the lists per item too (undo: callboard disconnect)"}
}

func setDriver(r gitx.Repo) bool {
	_, err1 := r.Git("config", "merge.callboard.name", "Callboard: merge the lists per item")
	_, err2 := r.Git("config", "merge.callboard.driver", driverCmd)
	return err1 == nil && err2 == nil
}

func ownAttrLines(r gitx.Repo) []string {
	own := store.Open(r.Root).Custom()
	if len(own) == 0 {
		return nil
	}
	lines := []string{".callboard/lists.md merge=union"}
	for _, k := range own {
		lines = append(lines, k.File+" merge=callboard")
	}
	return lines
}

func listFiles(r gitx.Repo) []string {
	var files []string
	for _, a := range attrLines {
		files = append(files, strings.Fields(a)[0])
	}
	for _, k := range store.Open(r.Root).Custom() {
		files = append(files, k.File)
	}
	return files
}

func LineMerged(r gitx.Repo) []string {
	var files []string
	for _, file := range listFiles(r) {
		if b, err := os.ReadFile(filepath.Join(r.Root, file)); err == nil && strings.Contains(string(b), lineMark+"\n") {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		return nil
	}
	return remerge(r, files, "git merged it line by line without callboard; ")
}

func Remerge(r gitx.Repo) []string {
	return remerge(r, listFiles(r), "")
}

func remerge(r gitx.Repo, files []string, why string) []string {
	var out []string
	for _, file := range files {
		if u, _ := r.Git("ls-files", "-u", "--", file); u == "" {
			continue
		}
		if _, err := r.Git("checkout", "-m", "--", file); err != nil {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(r.Root, file))
		if n := strings.Count(string(b), "\n<<<<<<< ") + map[bool]int{true: 1}[strings.HasPrefix(string(b), "<<<<<<< ")]; n > 0 {
			out = append(out, fmt.Sprintf("callboard: %sredid the merge of %s per item; %d item(s) still changed two ways (callboard list shows them)", why, file, n))
		} else {
			r.Git("add", "--", file)
			out = append(out, fmt.Sprintf("callboard: %sredid the merge of %s per item; no conflicts left, and it's staged", why, file))
		}
	}
	return out
}

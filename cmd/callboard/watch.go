package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/AsWali/CallBoard/internal/show"
	"github.com/AsWali/CallBoard/internal/store"
)

type tab struct {
	name  string
	words []string
}

var watchTabs = []tab{
	{"Overview", nil},
	{"You", []string{"you"}},
	{"Now", []string{"now"}},
	{"Map", []string{"graph"}},
	{"Quiet", []string{"quiet"}},
	{"Recent", []string{"recent"}},
}

func watch(args []string) error {
	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	if !term.IsTerminal(in) || !term.IsTerminal(out) {
		return errors.New("callboard watch needs a terminal; callboard show draws the lists once")
	}
	st := store.Open(wd())
	if len(args) > 0 && args[0] == "--panel" {
		args = args[1:]
		defer markPanel(st)()
	}
	tabs := tabsFor(st, args)

	old, err := term.MakeRaw(in)
	if err != nil {
		return err
	}
	restoreVT := enableVT()
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[?7l")
	done := func() {
		fmt.Print("\x1b[?7h\x1b[?25h\x1b[?1049l")
		restoreVT()
		term.Restore(in, old)
	}
	defer done()

	keys := make(chan string)
	go func() {
		buf := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- string(buf[:n])
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sig)

	cur, top := 0, 0
	var lines []string
	stamp, w, h := "", 0, 0
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	drawn := time.Time{}
	for {
		nw, nh, _ := term.GetSize(out)
		if nw <= 0 {
			nw, nh = 100, 30
		}
		now := st.Stamp()
		if now != stamp || nw != w || nh != h || lines == nil || time.Since(drawn) > 30*time.Second {
			if now != stamp && stamp != "" {
				st.Reconcile()
				name := tabs[cur].name
				tabs, cur = tabsFor(st, args), 0
				for i, t := range tabs {
					if t.name == name {
						cur = i
					}
				}
			}
			stamp, w, h, drawn = now, nw, nh, time.Now()
			lines = strings.Split(strings.TrimRight(body(st, tabs, cur, w), "\n"), "\n")
			paint(tabs, cur, lines, &top, st, w, h)
		}
		select {
		case <-sig:
			return nil
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			page := h - 3
			switch k {
			case "q", "Q", "\x03", "\x1b", "\x04":
				return nil
			case "\t", "l", "\x1b[C":
				cur, top, lines = (cur+1)%len(tabs), 0, nil
			case "\x1b[Z", "h", "\x1b[D":
				cur, top, lines = (cur+len(tabs)-1)%len(tabs), 0, nil
			case "j", "\x1b[B", "\x1bOB":
				top++
			case "k", "\x1b[A", "\x1bOA":
				top--
			case " ", "\x1b[6~", "f":
				top += page
			case "b", "\x1b[5~":
				top -= page
			case "g", "\x1b[H":
				top = 0
			case "G", "\x1b[F":
				top = len(lines)
			case "r":
				lines = nil
			default:
				if len(k) == 1 && k[0] >= '1' && k[0] <= '9' && int(k[0]-'1') < len(tabs) {
					cur, top, lines = int(k[0]-'1'), 0, nil
				}
			}
			if lines != nil {
				paint(tabs, cur, lines, &top, st, w, h)
			}
		case <-tick.C:
		}
	}
}

func tabsFor(st *store.Store, args []string) []tab {
	var tabs []tab
	if len(args) > 0 {
		tabs = append(tabs, tab{strings.Join(args, " "), args})
	}
	tabs = append(tabs, watchTabs...)
	for _, v := range st.SavedViews() {
		tabs = append(tabs, tab{v.Name, []string{"view:" + v.Name}})
	}
	return tabs
}

func body(st *store.Store, tabs []tab, cur, w int) string {
	if !st.HasLists() {
		return "Callboard isn't used here: this folder isn't a git repo and has no backlog.md, requests.md or questions.md."
	}
	keys := map[string]string{}
	for i, t := range tabs[:min(len(tabs), 9)] {
		if len(t.words) > 0 {
			keys[strings.Join(t.words, " ")] = fmt.Sprint(i + 1)
		}
	}
	return show.Render(st, tabs[cur].words, show.Opts{Color: true, Width: w - 1, Wrap: true, Keys: keys})
}

func paint(tabs []tab, cur int, lines []string, top *int, st *store.Store, w, h int) {
	room := max(h-3, 1)
	*top = min(max(*top, 0), max(len(lines)-room, 0))
	var b bytes.Buffer
	b.WriteString("\x1b[H")
	name := st.Name
	if br := st.Branch(); br != "" {
		name += " · " + br
	}
	bar := ""
	for i, t := range tabs {
		label := t.name
		if i < 9 {
			label = fmt.Sprintf("%d %s", i+1, t.name)
		}
		if i == cur {
			bar += "\x1b[7m " + label + " \x1b[0m"
		} else {
			bar += "\x1b[2m " + label + " \x1b[0m"
		}
	}
	head := "\x1b[1m" + name + "\x1b[0m  " + bar
	if show.Width(head) > w {
		head = fmt.Sprintf("\x1b[7m %s \x1b[0m\x1b[2m  %d of %d · %s\x1b[0m", tabs[cur].name, cur+1, len(tabs), name)
	}
	b.WriteString(head + "\x1b[K\r\n\x1b[K\r\n")
	end := min(*top+room, len(lines))
	for _, l := range lines[*top:end] {
		b.WriteString(l + "\x1b[0m\x1b[K\r\n")
	}
	for i := end - *top; i < room; i++ {
		b.WriteString("\x1b[K\r\n")
	}
	foot := "Tab next view · 1-9 pick one · q quit"
	if len(lines) > room {
		foot = fmt.Sprintf("%d-%d of %d · ↑↓ scroll · ", *top+1, end, len(lines)) + foot
	}
	if w < 60 {
		foot = "Tab view · q quit"
		if len(lines) > room {
			foot = fmt.Sprintf("%d-%d of %d · ↑↓ · ", *top+1, end, len(lines)) + foot
		}
	}
	b.WriteString("\x1b[2m" + foot + "\x1b[0m\x1b[K\x1b[J")
	os.Stdout.Write(b.Bytes())
}

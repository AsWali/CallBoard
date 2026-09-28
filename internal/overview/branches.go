package overview

import (
	"slices"
	"sync"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/gitx"
	"github.com/AsWali/CallBoard/internal/merge"
	"github.com/AsWali/CallBoard/internal/store"
)

type Item struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	What   string `json:"what"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Done   bool   `json:"done,omitempty"`
}

type Step struct {
	gitx.Commit
	Items []Item `json:"items"`
}

type Card struct {
	Name     string       `json:"name"`
	Worktree string       `json:"worktree,omitempty"`
	Current  bool         `json:"current,omitempty"`
	Tip      gitx.Commit  `json:"tip"`
	Fork     *gitx.Commit `json:"fork,omitempty"`
	InMain   bool         `json:"inMain"`

	Since []Step `json:"since"`
	Older int    `json:"older,omitempty"`

	Brings []Item `json:"brings"`

	Conflicts []store.Merge `json:"conflicts"`
}

type Branches struct {
	Main     string `json:"main"`
	Current  string `json:"current"`
	Branches []Card `json:"branches"`
}

const maxSteps = 30

var (
	sinceMu    sync.Mutex
	sinceCache = map[string][]Step{}
	olderCache = map[string]int{}
)

func BuildBranches(r gitx.Repo, current string, only ...string) Branches {
	main := r.DefaultBranch()
	wts := r.WorktreeBranches()
	out := Branches{Main: main, Current: current, Branches: []Card{}}
	mainTip := r.Commit("refs/heads/" + main)
	mainLists := map[string]string{}
	kinds := store.Open(r.Root).Lists()
	for _, k := range kinds {
		mainLists[k.File] = Read(r, main, wts[main], k)
	}
	for _, b := range r.Branches() {
		if b == main || (len(only) > 0 && !slices.Contains(only, b)) {
			continue
		}
		c := Card{Name: b, Worktree: wts[b], Current: b == current, Tip: r.Commit("refs/heads/" + b), Since: []Step{}, Brings: []Item{}, Conflicts: []store.Merge{}}
		base := r.MergeBase("refs/heads/"+main, "refs/heads/"+b)
		baseLists := map[string]string{}
		if base != "" {
			f := r.Commit(base)
			c.Fork = &f
			c.InMain = base == c.Tip.Sha
			specs := []string{}
			for _, k := range kinds {
				specs = append(specs, base+":"+k.File)
			}
			got := r.ShowMany(specs)
			for _, k := range kinds {
				baseLists[k.File] = got[base+":"+k.File]
			}
			if !c.InMain {
				c.Since, c.Older = since(r, base, mainTip.Sha)
			}
		}
		for _, k := range kinds {
			bt := Read(r, b, wts[b], k)
			c.Brings = append(c.Brings, items(k, baseLists[k.File], bt)...)
			if c.InMain {
				continue
			}
			res := merge.MergeAs(k.Prefix, baseLists[k.File], mainLists[k.File], bt, main, b)
			if res.Conflicts > 0 {
				c.Conflicts = append(c.Conflicts, store.MergesIn(k, board.ParseAs(res.Text, k.Prefix))...)
			}
		}
		out.Branches = append(out.Branches, c)
	}

	slices.SortStableFunc(out.Branches, func(a, b Card) int {
		switch {
		case a.Current != b.Current:
			return map[bool]int{true: -1, false: 1}[a.Current]
		case a.InMain != b.InMain:
			return map[bool]int{true: 1, false: -1}[a.InMain]
		}
		return b.Tip.At.Compare(a.Tip.At)
	})
	return out
}

func since(r gitx.Repo, base, mainTip string) ([]Step, int) {
	id := base + ".." + mainTip
	sinceMu.Lock()
	if s, ok := sinceCache[id]; ok {
		sinceMu.Unlock()
		return s, olderCache[id]
	}
	sinceMu.Unlock()

	kinds := store.Open(r.Root).Lists()
	cs := r.Log("--first-parent", "-500", id)
	var specs []string
	for _, c := range cs {
		if len(c.Parents) == 0 {
			continue
		}
		for _, k := range kinds {
			specs = append(specs, c.Sha+":"+k.File, c.Parents[0]+":"+k.File)
		}
	}
	got := r.ShowMany(specs)
	steps, older := []Step{}, 0
	for _, c := range cs {
		if len(c.Parents) == 0 {
			continue
		}
		var its []Item
		for _, k := range kinds {
			its = append(its, items(k, got[c.Parents[0]+":"+k.File], got[c.Sha+":"+k.File])...)
		}
		if len(its) == 0 {
			continue
		}
		if len(steps) == maxSteps {
			older++
			continue
		}
		steps = append(steps, Step{Commit: c, Items: its})
	}
	sinceMu.Lock()
	if len(sinceCache) > 200 {
		sinceCache, olderCache = map[string][]Step{}, map[string]int{}
	}
	sinceCache[id], olderCache[id] = steps, older
	sinceMu.Unlock()
	return steps, older
}

func items(k store.Kind, from, to string) []Item {
	if from == to {
		return nil
	}
	var out []Item
	tf := board.ParseAs(to, k.Prefix)
	for _, e := range store.DiffLists(k, board.ParseAs(from, k.Prefix), tf) {
		it := Item{Kind: k.Noun, Key: e.Key, What: e.What, Title: e.Title, Detail: e.Detail}
		if t := tf.Find(e.Key); e.What == "added" && t != nil {
			it.Done = t.Done
		}
		out = append(out, it)
	}
	return out
}

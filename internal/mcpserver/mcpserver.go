package mcpserver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AsWali/CallBoard/internal/board"
	"github.com/AsWali/CallBoard/internal/show"
	"github.com/AsWali/CallBoard/internal/store"
)

type listIn struct {
	Kind    string `json:"kind,omitempty" jsonschema:"task, request, question, one of your own lists by name, or all (default)"`
	Status  string `json:"status,omitempty" jsonschema:"open (default), ready (open, waits on nothing), done or all"`
	Section string `json:"section,omitempty" jsonschema:"only items under this heading"`
	Key     string `json:"key,omitempty" jsonschema:"one item, e.g. B-k3f9: its notes, options, what it waits on and frees, and its history"`
	Find    string `json:"find,omitempty" jsonschema:"words that must all be in an item's title, notes or fields; searches done items too unless status is set"`
	From    int    `json:"from,omitempty" jsonschema:"start at this item, counting from 0, to see the next ones"`
	Limit   int    `json:"limit,omitempty" jsonschema:"how many items to show, 50 by default"`
}

type addIn struct {
	Title       string            `json:"title" jsonschema:"one line"`
	Kind        string            `json:"kind,omitempty" jsonschema:"task (default), request (only the human can do it), question (the human decides), or one of your own lists by name"`
	Section     string            `json:"section,omitempty" jsonschema:"heading to put it under; made if missing"`
	Body        []string          `json:"body,omitempty" jsonschema:"lines under it: a request's exact steps, a task's notes"`
	Options     []string          `json:"options,omitempty" jsonschema:"a question's answers to choose from"`
	Recommended int               `json:"recommended,omitempty" jsonschema:"the option you recommend (1-based)"`
	Needs       []string          `json:"needs,omitempty" jsonschema:"keys it waits on"`
	Fields      map[string]string `json:"fields,omitempty" jsonschema:"fields, e.g. {\"prio\": \"high\"}"`
	Parent      string            `json:"parent,omitempty" jsonschema:"a task or request key: add this as its subtask"`
}

type tickIn struct {
	Key  string `json:"key" jsonschema:"B-k3f9@7c: the key, with the version you read"`
	Done *bool  `json:"done,omitempty" jsonschema:"false opens it again"`
}

type editIn struct {
	Key     string            `json:"key" jsonschema:"B-k3f9@7c: the key, with the version you read"`
	Title   *string           `json:"title,omitempty" jsonschema:"a new title"`
	Section *string           `json:"section,omitempty" jsonschema:"the heading to move it under; made if missing"`
	Fields  map[string]string `json:"fields,omitempty" jsonschema:"fields to set, e.g. {\"prio\": \"high\"}; \"\" removes one"`
	Needs   *[]string         `json:"needs,omitempty" jsonschema:"every key it waits on (replaces the old ones); [] for none"`
	AddNote string            `json:"add_note,omitempty" jsonschema:"text to add under the item, after its notes; new lines for more than one line"`
	Notes   *string           `json:"notes,omitempty" jsonschema:"text that replaces all of its notes (a question keeps its options); \"\" removes them"`
	Before  string            `json:"before,omitempty" jsonschema:"a key in the same list: put this item just before it"`
	After   string            `json:"after,omitempty" jsonschema:"a key in the same list: put this item just after it"`
	Delete  bool              `json:"delete,omitempty" jsonschema:"remove the item"`
	Restore bool              `json:"restore,omitempty" jsonschema:"bring back a deleted item, where it was"`
}

type listsIn struct {
	Add    string `json:"add,omitempty" jsonschema:"a lowercase name like ideas: makes ideas.md, a task list with its own key letter"`
	Rename string `json:"rename,omitempty" jsonschema:"one of your lists to rename; give the new name as to"`
	To     string `json:"to,omitempty" jsonschema:"the new name, with rename"`
	Remove string `json:"remove,omitempty" jsonschema:"one of your lists to remove, file and all; refused while it has open items unless force"`
	Force  bool   `json:"force,omitempty"`
}

type sectionIn struct {
	Kind   string `json:"kind,omitempty" jsonschema:"task (default), request, question or one of your own lists"`
	Name   string `json:"name" jsonschema:"the heading"`
	To     string `json:"to,omitempty" jsonschema:"a new name"`
	Before string `json:"before,omitempty" jsonschema:"another heading: move it just before that one"`
	After  string `json:"after,omitempty" jsonschema:"another heading: move it just after that one"`
	Remove bool   `json:"remove,omitempty" jsonschema:"remove the heading"`
	Into   string `json:"into,omitempty" jsonschema:"with remove: the heading its items go under"`
}

type claimIn struct {
	Key     string `json:"key" jsonschema:"the item's key"`
	Force   bool   `json:"force,omitempty" jsonschema:"take it over from another agent, or take an item that is for someone else"`
	Release bool   `json:"release,omitempty" jsonschema:"drop your claim without finishing it"`
}

type assumeIn struct {
	Key    string `json:"key" jsonschema:"the question's key"`
	Option int    `json:"option" jsonschema:"the option you picked (1-based)"`
}

type answerIn struct {
	Key    string `json:"key" jsonschema:"the question's key"`
	Answer string `json:"answer" jsonschema:"the human's answer: an option number or their words"`
	Why    string `json:"why,omitempty" jsonschema:"their reason, if they gave one"`
}

type resolveIn struct {
	Key  string `json:"key" jsonschema:"the item's key"`
	Keep int    `json:"keep" jsonschema:"1 or 2: the version to keep"`
}

type viewIn struct {
	Key     string    `json:"key,omitempty" jsonschema:"the view to change (V-…); leave out for a new one"`
	Delete  bool      `json:"delete,omitempty" jsonschema:"remove the view"`
	Name    *string   `json:"name,omitempty" jsonschema:"its name on the page"`
	List    *string   `json:"list,omitempty" jsonschema:"backlog (default), requests or questions"`
	Layout  *string   `json:"layout,omitempty" jsonschema:"list, board or table"`
	Group   *string   `json:"group,omitempty" jsonschema:"field to group by; a board's columns"`
	Order   *[]string `json:"order,omitempty" jsonschema:"the groups in order"`
	Sort    *[]string `json:"sort,omitempty" jsonschema:"fields to sort by; -prio is high to low"`
	Filter  *[]string `json:"filter,omitempty" jsonschema:"all must hold: field=value, field!=value, field=a|b"`
	Show    *[]string `json:"show,omitempty" jsonschema:"fields shown on each item"`
	Suggest *string   `json:"suggest,omitempty" jsonschema:"only for a view nobody asked for: why it helps; the human keeps or dismisses it"`
}

const viewHelp = `Make, change or remove a saved view of a list on the page; no arguments lists them. E.g. kanban: layout board, group status, order [todo, doing, done]; most important first: layout table, sort [-prio]; blocked: filter [blocked=waiting]. Fields: the items' own (reuse their names) or section, done, blocked, claimed, by, at, doneat, needs. The reply says what the view shows; fix what it warns about. Use suggest only for a view nobody asked for.`

func actorOf(req *mcp.CallToolRequest) store.Actor {
	a := store.Actor{By: store.By(), Session: fmt.Sprintf("mcp-%d", os.Getppid())}
	if id := store.AgentSession(); id != "" {
		a.Session = id
	}
	if req != nil && req.Params != nil {
		for _, k := range []string{"threadId", "sessionId"} {
			if id, ok := req.Params.GetMeta()[k].(string); ok && id != "" {
				a.Session = id
				break
			}
		}
	}
	if req != nil && req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			n := strings.ToLower(p.ClientInfo.Name)
			switch {
			case strings.Contains(n, "claude"):
				a.By = "claude"
			case strings.Contains(n, "codex"):
				a.By = "codex"
			case n != "":
				a.By = strings.Fields(n)[0]
			}
		}
	}
	return a
}

type worktree string

func (w worktree) actor(req *mcp.CallToolRequest) store.Actor {
	a := actorOf(req)
	if strings.HasPrefix(a.Session, "mcp-") {
		if sess := store.Open(string(w)).SessionOfProcess(os.Getppid()); sess != "" {
			a.Session = sess
		}
	}
	return a
}

func (w worktree) store(req *mcp.CallToolRequest) *store.Store {
	x := store.Open(string(w))
	x.Touch(w.actor(req).Session)
	return x
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func tool[In any](s *mcp.Server, name, desc string, do func(*mcp.CallToolRequest, In) (string, error)) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: desc},
		func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
			out, err := do(req, in)
			if err != nil {
				return nil, nil, err
			}
			return text(out), nil, nil
		})
}

func itemTool[In any](s *mcp.Server, w worktree, name, desc string, do func(*store.Store, In, store.Actor) (*board.Task, error)) {
	tool(s, name, desc, func(req *mcp.CallToolRequest, in In) (string, error) {
		x := w.store(req)
		t, err := do(x, in, w.actor(req))
		if err != nil {
			return "", err
		}
		return x.Line(t), nil
	})
}

func New(dir, version string) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "callboard", Version: version}, &mcp.ServerOptions{
		Instructions: "Callboard keeps this worktree's tasks (backlog.md), requests for the human (requests.md) and questions for the human (questions.md).\n" + store.Rule,
	})
	w := worktree(dir)
	actor, st := w.actor, w.store

	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, r mcp.Request) (mcp.Result, error) {
			if c, ok := r.(*mcp.CallToolRequest); ok && c.Params != nil && c.Params.Name != "list" && c.Params.Name != "news" && store.Open(dir).Off() {
				res := text("Callboard is switched off in this repo, so its lists can be read (the list tool) but not changed. Keep the work as the repo itself does. The human turns it back on with: callboard on")
				res.IsError = true
				return res, nil
			}
			res, err := next(ctx, method, r)
			req, ok := r.(*mcp.CallToolRequest)
			out, ok2 := res.(*mcp.CallToolResult)
			if err != nil || !ok || !ok2 || out == nil || req.Params == nil || req.Params.Name == "news" {
				return res, err
			}
			if news := store.FormatNews(store.Open(dir).News(actor(req).Session)); news != "" {
				out.Content = append(out.Content, &mcp.TextContent{Text: news})
			}
			return res, err
		}
	})

	tool(s, "list", "The lists: each item's key@version, title, fields, what it waits on and who claimed it. Answer every question about the lists with it (find searches them; it shows 50 items at a time and says where the next ones start); never read or grep backlog.md, requests.md or questions.md.",
		func(req *mcp.CallToolRequest, in listIn) (string, error) {
			if in.Key != "" {
				return show.Render(st(req), []string{in.Key}, show.Opts{Width: 1000}), nil
			}
			status := in.Status
			if in.Find != "" && status == "" {
				status = "all"
			}
			l, err := st(req).List(in.Kind, status, in.Section)
			limit := in.Limit
			if limit <= 0 {
				limit = store.PageSize
			}
			return store.FormatList(st(req).Search(l, in.Find).Page(in.From, limit)), err
		})

	tool(s, "add", "Add a task, a request for the human, or a question for the human.",
		func(req *mcp.CallToolRequest, in addIn) (string, error) {
			k, err := st(req).KindNamed(in.Kind)
			if err != nil {
				return "", err
			}
			body := in.Body
			if len(in.Options) > 0 {
				if k != store.Questions {
					return "", fmt.Errorf("options are for questions; set kind to question")
				}
				if in.Recommended < 0 || in.Recommended > len(in.Options) {
					return "", fmt.Errorf("recommended is 1 to %d", len(in.Options))
				}
				var opts []string
				for i, o := range in.Options {
					opts = append(opts, board.OptionLine(o, i+1 == in.Recommended))
				}
				body = append(opts, body...)
			}
			x, a := st(req), actor(req)
			t, err := x.Add(k, board.New{Title: in.Title, Section: in.Section, Body: body, Needs: in.Needs, Parent: in.Parent}, a)
			if err == nil && len(in.Fields) > 0 {
				t, err = x.SetFields(t.Key, store.FieldList(in.Fields), a)
			}
			if err != nil {
				return "", err
			}
			return x.Line(t), nil
		})

	itemTool(s, w, "tick", "Mark a task or request done (done: false opens it again). Drops your claim on it.",
		func(x *store.Store, in tickIn, a store.Actor) (*board.Task, error) {
			return x.Tick(in.Key, in.Done == nil || *in.Done, a)
		})

	tool(s, "edit", "Change an item: title, section (its heading), fields (prio, area, due…), needs (keys it waits on; it's blocked until they're done), add_note (a plan or finding under it), notes (replace them), before or after (another key: its place in the list). delete removes it (only when the human asks, or a duplicate you made); restore brings a deleted one back.",
		func(req *mcp.CallToolRequest, in editIn) (string, error) {
			x, a := st(req), actor(req)
			switch {
			case in.Delete && in.Restore:
				return "", fmt.Errorf("give delete or restore, not both")
			case in.Restore:
				t, err := x.Restore(in.Key, a)
				if err != nil {
					return "", err
				}
				return "brought back " + x.Line(t), nil
			case in.Delete:
				t, freed, err := x.Delete(in.Key, a)
				if err != nil {
					return "", err
				}
				msg := "deleted " + t.Key + " " + t.Title + "; restore brings it back"
				if len(freed) > 0 {
					msg += "; no longer waiting on it: " + strings.Join(freed, ", ")
				}
				return msg, nil
			}
			t, err := edit(x, in, a)
			if err != nil {
				return "", err
			}
			return x.Line(t), nil
		})

	tool(s, "view", viewHelp,
		func(req *mcp.CallToolRequest, in viewIn) (string, error) {
			x := st(req)
			if in.Delete {
				return "removed " + in.Key, x.DeleteView(in.Key, actor(req))
			}
			spec := store.ViewSpec{Key: in.Key, Name: in.Name, List: in.List, Layout: in.Layout, Group: in.Group, Order: in.Order, Sort: in.Sort, Filter: in.Filter, Show: in.Show, Suggest: in.Suggest}
			if spec == (store.ViewSpec{}) {
				var views []string
				for _, v := range x.SavedViews() {
					views = append(views, x.Checked(v))
				}
				if views == nil {
					return "No saved views.", nil
				}
				return strings.Join(views, "\n"), nil
			}
			v, err := x.SaveView(spec, actor(req))
			if err != nil {
				return "", err
			}
			return x.Checked(v), nil
		})

	tool(s, "lists", "The lists here, or add, rename or remove one of the human's own lists (ideas, bugs) when they ask. Its items work like tasks; pass its name as kind.",
		func(req *mcp.CallToolRequest, in listsIn) (string, error) {
			x := st(req)
			if in.Rename != "" {
				k, err := x.RenameList(in.Rename, in.To, actor(req))
				if err != nil {
					return "", err
				}
				return "renamed to " + k.File + " (keys still " + k.Prefix + ")", nil
			}
			if in.Remove != "" {
				k, views, err := x.RemoveList(in.Remove, in.Force, actor(req))
				if err != nil {
					return "", err
				}
				msg := "removed " + k.File
				if len(views) > 0 {
					msg += " and its views (" + strings.Join(views, ", ") + ")"
				}
				return msg, nil
			}
			if in.Add != "" {
				k, made, err := x.AddList(in.Add, actor(req))
				if err != nil {
					return "", err
				}
				if !made {
					return k.File + " is already a list (keys " + k.Prefix + ")", nil
				}
				return "added " + k.File + " (keys " + k.Prefix + "); add to it with kind " + k.Name, nil
			}
			var lines []string
			for _, k := range x.Lists() {
				lines = append(lines, k.File+" keys "+k.Prefix)
			}
			return strings.Join(lines, "\n"), nil
		})

	tool(s, "section", "Rename, reorder or remove a heading in a list. Give one of to, before, after or remove.",
		func(req *mcp.CallToolRequest, in sectionIn) (string, error) {
			return st(req).ChangeSection(store.SectionChange(in), actor(req))
		})

	tool(s, "resolve", "After a merge where two branches changed an item differently, keep version 1 or 2 (list shows both). Ask the human if it matters.",
		func(req *mcp.CallToolRequest, in resolveIn) (string, error) {
			kept, err := st(req).Resolve(in.Key, in.Keep, actor(req))
			if err != nil {
				return "", err
			}
			if len(kept) == 0 {
				return fmt.Sprintf("removed %s: version %d is the one without it", in.Key, in.Keep), nil
			}
			var lines []string
			for _, t := range kept {
				lines = append(lines, "kept "+t.Key+"@"+t.Version+"  "+t.Title)
			}
			return strings.Join(lines, "\n"), nil
		})

	tool(s, "claim", "Say you're working on an item, so other agents pick something else. It lasts while your session runs, or 30 minutes after it goes quiet; tick drops it. Items whose for field names someone else are refused.",
		func(req *mcp.CallToolRequest, in claimIn) (string, error) {
			a, x := actor(req), st(req)
			if in.Release {
				key, _, _ := strings.Cut(in.Key, "@")
				return key + " released", x.Release(key, a.By, a.Session, false)
			}
			c, err := x.ClaimItem(in.Key, a.By, a.Session, in.Force)
			if err != nil {
				return "", err
			}
			return c.Key + " claimed by " + c.By, nil
		})

	itemTool(s, w, "assume", "Can't wait for the human? Pick an option and go on; the question stays open, marked assumed, until the human confirms or overturns it.",
		func(x *store.Store, in assumeIn, a store.Actor) (*board.Task, error) {
			return x.Assume(in.Key, in.Option, a)
		})

	tool(s, "answer", "Record the answer the human gave you in chat. Never pick one yourself (use assume) or answer to unblock work (use edit's needs).",
		func(req *mcp.CallToolRequest, in answerIn) (string, error) {
			a := store.Actor{By: "you", Session: actor(req).Session}
			res, _, err := store.Everywhere(st(req), in.Key, func(w *store.Store) (store.Answered, error) {
				return w.AnswerQuestion(in.Key, in.Answer, in.Why, a)
			})
			if err != nil {
				return "", err
			}
			out := fmt.Sprintf("%s@%s answered: %s", res.Key, res.Version, res.Answer)
			if len(res.Unblocked) > 0 {
				out += "\nno longer waiting on it: " + strings.Join(res.Unblocked, ", ")
			}
			if len(res.Reopened) > 0 {
				out += fmt.Sprintf("\nopen again, built on the assumed answer %q: %s", res.Assumed, strings.Join(res.Reopened, ", "))
			}
			return out, nil
		})

	tool(s, "news", "What changed since you last looked: answers, ticks, renames, new items. Each is told once.",
		func(req *mcp.CallToolRequest, in struct{}) (string, error) {
			if news := store.FormatNews(st(req).News(actor(req).Session)); news != "" {
				return news, nil
			}
			return "No news.", nil
		})
	return s
}

func Run(ctx context.Context, dir, version string) error {
	return New(dir, version).Run(ctx, Stdio(os.Stdin, os.Stdout))
}

func edit(x *store.Store, in editIn, a store.Actor) (t *board.Task, err error) {
	if in.Title != nil {
		if t, err = x.Rename(in.Key, *in.Title, a); err != nil {
			return nil, err
		}
	}
	if in.Section != nil {
		if t, err = x.Move(in.Key, *in.Section, a); err != nil {
			return nil, err
		}
	}
	if len(in.Fields) > 0 {
		if t, err = x.SetFields(in.Key, store.FieldList(in.Fields), a); err != nil {
			return nil, err
		}
	}
	if in.Needs != nil {
		if t, err = x.SetNeeds(in.Key, *in.Needs, a); err != nil {
			return nil, err
		}
	}
	if in.Notes != nil {
		if t, err = x.SetNotes(in.Key, *in.Notes, false, a); err != nil {
			return nil, err
		}
	}
	if in.AddNote != "" {
		if t, err = x.SetNotes(in.Key, in.AddNote, true, a); err != nil {
			return nil, err
		}
	}
	if in.Before != "" || in.After != "" {
		if in.Before != "" && in.After != "" {
			return nil, fmt.Errorf("give before or after, not both")
		}
		if t, err = x.Place(in.Key, in.Before+in.After, in.After != "", a); err != nil {
			return nil, err
		}
	}
	if t == nil {
		return nil, fmt.Errorf("nothing to change: give title, section, fields, needs, add_note, notes, before, after, delete or restore")
	}
	return t, nil
}

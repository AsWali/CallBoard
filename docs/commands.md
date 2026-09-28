# Commands and MCP tools

[← Back to the README](../README.md)

## Commands

| Command | Does |
|---|---|
| `callboard show [WORDS]` | The lists drawn for you, as `/callboard` does in Claude Code |
| `callboard watch [WORDS]` | The same, kept on screen and drawn again when the lists change; Tab or 1-9 switch views, q quits |
| `callboard panel [WORDS]` · `callboard panel off` | Open `watch` beside the agent, split into the same window when the terminal can; close it |
| `callboard statusline on [--below]` · `off` | A Callboard line in Claude Code's status line, only when you ask; `--below` keeps your own above it |
| `callboard list [--kind K] [--ready \| --done \| --all] [--section S] [--find WORDS] [--from N] [--limit N] [--json]` | The items, open ones by default; `--find` keeps those with all the words, done ones too. An agent gets 50 at a time, with where the next ones start (`--limit 0` for all) |
| `callboard add "title" [--kind task\|request\|question] [--section S] [--under KEY] [--needs KEYS] [--body L]… [--option O]… [--recommended N]` | Add an item; `--under` makes it a subtask |
| `callboard tick KEY [--undo]` | Mark a task or request done, or open it again |
| `callboard note KEY "text" [--replace \| --clear]` | Add a note under an item; `--replace` swaps all its notes, `--clear` removes them (a question keeps its options) |
| `callboard rename KEY "title"` | Change the title |
| `callboard delete KEY…` · `restore KEY` | Delete items; `restore` puts one back where it was, even if it was deleted by hand, in another clone or by someone else, as long as git has it |
| `callboard needs KEY KEYS… [--none]` | What it waits on |
| `callboard set KEY field=value…` | Set fields on an item (`field=` removes one) |
| `callboard move KEY "Section"` · `--before \| --after KEY2` | Put an item under another heading, or just before or after another item |
| `callboard lists [add NAME]` | The lists here, or make `NAME.md`, a task list with its own key letter |
| `callboard lists rename NAME NEW` / `remove NAME [--force]` | Rename or remove one of your lists |
| `callboard section NAME --to NEW \| --before S \| --after S \| --remove [--into S] [--kind K]` | Rename a heading, move it with everything under it, or remove it (`--into` says where its items go) |
| `callboard claim KEY [--force]` · `release KEY` | Say you're on it, so two agents don't do the same thing |
| `callboard assume QKEY N` | Go ahead with option N; the question stays open for you |
| `callboard answer QKEY N\|"words" [--why W]` | Answer a question |
| `callboard reopen QKEY` | Ask an answered question again |
| `callboard resolve KEY 1\|2` | After a merge conflict, keep the first or the second version |
| `callboard news` | What changed since this session last looked |
| `callboard agents [--json]` | The Claude Code and Codex sessions here: name, colour, working or not |
| `callboard branches [--json]` | Each branch against main, in the lists, and which items would conflict if it merged now |
| `callboard views [--json]` | The saved views |
| `callboard view [KEY] [--name N] [--list L] [--layout list\|board\|table] [--group F] [--order a,b] [--sort F,-G] [--filter F=V]… [--show a,b] [--suggest "why"] [--keep] [--delete]` | Make, change or delete a view; `--suggest` offers it to you instead of adding it |
| `callboard keys` | Give items typed by hand a key, and hand over a checklist Callboard was leaving alone |
| `callboard serve [--port N] [--open]` | The live page; each worktree has its own port, 4700–4999 |
| `callboard setup [--global \| claude \| codex]` · `disconnect [--global]` · `status [--global]` | Connect Claude Code, Codex and git, undo it, check it |
| `callboard install` · `uninstall` | Put callboard in `~/.local/bin`, or take it out |
| `callboard off` · `on` | Switch Callboard off in this repo, or back on |
| `callboard prime` · `hook …` · `mcp` · `merge …` | What Claude Code, Codex and git call |
| `callboard version` | The version |

Any `KEY` may carry the version you read, as in `B-k3f9@7c`. Flags can go before or after the other words, and `--` ends them, so a title can start with a dash.

## Words for /callboard

Words for `/callboard`, `callboard show`, `watch` and `panel` combine:

| Words | Show |
|---|---|
| (nothing) | The overview: a little of each view below |
| `you` | Waiting on you: your open requests and questions, the one that frees the most work first |
| `now` | Right now: who is on what, the ready tasks, and questions an agent answered for you that need your OK |
| `graph` | The map: what waits on what across all the lists |
| `quiet` | Gone quiet: open items nobody has touched in a week or more |
| `recent` | Recently done, day by day |
| `todo` · `tasks` · `requests` · `questions` | One list; open items, or add `done` or `all` |
| `ready` · `waiting` · `claimed` · `done` · `all` | Items in that state, across the lists |
| `board` · `table` · `list` | The layout; a board has a column per heading, or per value of `by:FIELD` |
| `B-k3f9` | One item: notes, options, answer, what it waits on and frees, its history |
| `views` · `view:NAME` | The saved views, or one of them |
| `prio=high` · `by!=codex` · `needs=R-dv8k` | Only items with that field |
| `by:prio` · `sort:-prio` · `show:prio,due` | Group, sort (`-` is high first), and the fields a table shows |
| any other word | Items with it in their title or notes |

## Environment variables

| Env | Means |
|---|---|
| `CALLBOARD_BY` | Who changes are by. Defaults to `codex` inside Codex, `claude` inside Claude Code, else `you`. |
| `CALLBOARD_SESSION` | Names the session for claims and news from the command line. Inside an agent it's the agent's own (`CODEX_THREAD_ID`, `CLAUDE_CODE_SESSION_ID`), so the command and the MCP tools share one session. |

## MCP tools

The tools agents use. Each answers in a few lines of text.

| Tool | Does |
|---|---|
| `list` | The open items in compact lines; `find` searches them; with `key`, one item with its notes, what it waits on and frees, and its history |
| `add` | Add a task, request or question, with notes, options and what it waits on; `parent` makes it a subtask |
| `edit` | In one call: title, heading, fields, what it waits on, `add_note` (text added under the item) or `notes` (replaces its notes; `""` removes them), or `before`/`after` another item; `delete` removes it and `restore` brings it back |
| `section` | Rename, reorder or remove a heading |
| `lists` | The lists here, or `add` one of your own |
| `tick` | Mark done, or open again |
| `claim` | Say you're on it, or `release: true` |
| `assume` | Go ahead with an option; the question stays open |
| `answer` | Answer a question |
| `news` | What changed since this session last looked |
| `resolve` | Keep one version after a merge conflict |
| `view` | Make, change, suggest or delete a view; with no arguments, the saved views |

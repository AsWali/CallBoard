# The page

[← Back to the README](../README.md)

```bash
callboard serve --open
```

The page shows the worktree you're in, live. It updates the moment a list, a claim or a session changes, whether an agent, you or git made the change. It follows your system's light or dark setting.

- **On the left:** the lists and their views, **For you**, the **Overview** and **Branches**.
- **On the right:** who is here now, who is on what, and recent changes.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-backlog-dark.png">
  <img alt="The backlog of a made-up habit tracker called Pebble: tasks under Now and Next, one being done by a Claude Code session called streaks, one by Codex on another branch, and others waiting on a request or a question. On the right: the session here now, who is on what, and the notes Claude just added." src="images/page-backlog-light.png">
</picture>

The pictures come from a made-up project, Pebble, that `scripts/demo.sh` builds for you.

## For you

**For you** holds what's waiting on you: questions to decide and requests to do, together. Each question shows its options, with the agent's recommendation marked. Pick one, or type your own answer and why. Under each one, it says what answering lets go ahead.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-foryou-dark.png">
  <img alt="For you: three questions with their options, the recommended one marked, a box to answer in your own words, and a line saying which task answering lets go ahead." src="images/page-foryou-light.png">
</picture>

## The Map

**The Map** shows what waits on what across all the lists. It starts with the requests and questions that nothing else is waiting for, then shows what each one frees, and what comes after that.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-map-dark.png">
  <img alt="The Map: three columns, Start here, Then and After that. The Apple Developer account request frees the APNs key request and Sign in with Apple, which free Push reminders." src="images/page-map-light.png">
</picture>

## The Overview

**The Overview** has four more views. They're the same ones `/callboard` draws in the terminal:

<table>
<tr>
<td width="50%"><b>Waiting on you</b>: your requests and questions. The one that frees the most work comes first, counting everything further down the chain.<br><br>
<picture><source media="(prefers-color-scheme: dark)" srcset="images/page-waiting-dark.png"><img alt="Waiting on you, with the number of items each one frees" src="images/page-waiting-light.png"></picture></td>
<td width="50%"><b>Right now</b>: who is on what, by agent and branch, when each was last active, and the tasks that are ready with nobody on them.<br><br>
<picture><source media="(prefers-color-scheme: dark)" srcset="images/page-now-dark.png"><img alt="Right now: the streaks session on main and Codex on feat/offline, then the ready tasks" src="images/page-now-light.png"></picture></td>
</tr>
<tr>
<td><b>Gone quiet</b>: open items nobody has touched in a week or more, the longest first.<br><br>
<picture><source media="(prefers-color-scheme: dark)" srcset="images/page-quiet-dark.png"><img alt="Gone quiet: five items untouched for 12 to 20 days" src="images/page-quiet-light.png"></picture></td>
<td><b>Recently done</b>: what was ticked or answered, day by day, with the answers and why.<br><br>
<picture><source media="(prefers-color-scheme: dark)" srcset="images/page-recent-dark.png"><img alt="Recently done: items ticked each day, and the chart library question answered with Swift Charts" src="images/page-recent-light.png"></picture></td>
</tr>
</table>

## Your own fields

Any item can carry any facts you like, such as `prio`, `area`, `due`, `owner` or `estimate`. They're kept in the item's hidden comment:

```markdown
- [ ] Offline sync with a local SQLite cache <!-- id:B-7tg8 … prio:high area:sync due:2026-10-15 owner:"mobile team" -->
```

- **On the page,** use **+ Add a property** on an item, or click a table cell.
- **Agents** use `callboard set B-7tg8 area=sync due=2026-10-15`, or the `edit` tool. When an agent starts, it's told which fields and values are already in use, so it writes `prio` rather than inventing `priority`.
- **In the terminal,** you can filter by them too: `/callboard area=sync`, `/callboard board by:owner`.

Values sort by what they are:
- priorities by importance;
- dates as dates (`2026-10-15`, `Oct 15 2026`, `15 October`);
- numbers as numbers;
- versions in order, so `v1.9` comes before `v1.10`.

An open item whose `due` (or `deadline`) is past or within three days says so: "Overdue by 3 days" or "Due tomorrow". That shows on the page, in the terminal, and in what agents read, including a line at the start of each session.

## Who it's for

The `for` field says who should do an item:
- `for:you` (or `me`) for you;
- `for:claude` or `for:codex`;
- a session's name;
- or several, like `for:claude or codex`.

An agent won't claim an item meant for someone else unless you ask it to (`claim --force`). It doesn't count that item as ready, and isn't offered it as the next task. Items meant for an agent are the first ones it's offered. Tasks for you show up under **For you**, with the questions and requests.

## Views

Views lay a list out as a list, a board or a table, grouped, sorted and filtered by any field, including your own. They're saved in `.callboard/views.md` and committed like the lists, so you, every agent and every branch share them.

- **On a board,** drag a card to another column to change its field, or up and down in its column to reorder the list.
- **In a table,** click a cell to change it.
- **In the list as it is in the file,** drag a row to reorder it or put it under another heading. The ⋯ beside a heading renames it, moves it up or down, or removes it.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-board-dark.png">
  <img alt="A board of the backlog by status: todo, doing and done columns, with each card's area and priority and what it waits on." src="images/page-board-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-table-dark.png">
  <img alt="A table of the backlog sorted most important first, with priority and area columns." src="images/page-table-light.png">
</picture>

**+ New view** offers ready-made views built from the list's own fields: a board by status, most important first, ready to start, waiting on something, who is on what, by area, and recently done. A bar above each view says in words what it does (Table · Sort: Prio, most important first · + Filter · Columns); click a part to change it.

Agents can make views too, and they hear back what the view shows. A view that uses a field no item has is refused, so a typo can't leave you with an empty view:

```bash
callboard view --name "By area" --layout board --group area
callboard view --name "Mobile, most urgent" --layout table --filter owner="mobile team" --sort -prio --show prio,due,area
```

An agent that thinks a list would read better another way can suggest a view. It shows as a strip above the list with **Preview**, **Keep** and **Dismiss**.

## Open an item

Click an item to open it, or use `j`/`k` and Enter. The panel shows everything about it, with nothing hidden behind hovering:
- its status and every field (click one to change it);
- its heading;
- what it waits on, and what finishing it frees;
- who is on it, and who added it and when;
- its notes, and the answer box for a question;
- its history across worktrees.

`/` searches, `x` ticks and Esc closes. The panel's URL (`#backlog/B-k3f9`) opens it again.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-item-dark.png">
  <img alt="The item panel for Sign in with Apple: status, area, priority, its heading, the two things it waits on, the task it frees, who added it, its key, a note from Claude and its history." src="images/page-item-light.png">
</picture>

To delete an item, use **Delete** in its panel. The note that follows has Undo, and a deletion in Recent changes has **Bring back**.

## Here now

**Here now** lists the Claude Code and Codex sessions in this worktree: the name you gave a session, its colour, whether it's working, and what it was last asked. Click a session's id to copy the command that resumes it.

Under it, **Who is on what** lists every task an agent has claimed, with the session's name and its branch.

## Branches

**Branches** compares each branch with main, in the lists only. For each branch it shows:
- where it forked;
- what main's lists got since then;
- what the branch changes;
- which items would conflict if it merged now.

It merges nothing to find out, and uncommitted lists in another worktree count.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="images/page-branches-dark.png">
  <img alt="Branches: feat/offline forked from main, nothing changed in main's lists since, the branch set a status and added a task, and it would merge with no conflicts." src="images/page-branches-light.png">
</picture>

## Who can reach it

The page listens only on this computer, so nothing else on your network can reach it. It also answers only to localhost addresses, so a website you visit can't reach it by pointing its own name at your computer.

# The files

[← Back to the README](../README.md)

Every repo gets three lists:

| File | Holds | Keys |
|---|---|---|
| `backlog.md` | The work agents do | `B-…` |
| `requests.md` | Things only you can do: keys, accounts, money, DNS, deploys | `R-…` |
| `questions.md` | Decisions that are yours, with options and a recommendation | `Q-…` |

They're plain Markdown checklists, so they read fine on GitHub and in any editor:

```markdown
## Now
- [ ] Streak counter on the home screen <!-- id:B-k3f9 by:claude at:2026-09-27T09:12 prio:high needs:Q-au7h -->
  Count days in a row per habit; a missed day resets it.
- [x] Dark mode <!-- id:B-dk2m by:codex at:2026-09-22T11:10 done:2026-09-27T09:30 done-by:claude -->
```

```markdown
- [ ] Which sign-in should Pebble have at launch? <!-- id:Q-au7h by:claude at:2026-09-24T10:07 -->
  - Apple, plus an email link for the web (recommended)
  - Apple and Google
```

Each item is a checkbox line with a hidden comment. The comment holds:
- the item's key;
- who added it and when, and who finished it and when;
- what it waits on (`needs:`);
- its fields;
- the option an agent went ahead with (`assumed:`).

Headings are the groups. Indented lines under an item are its notes (and a question's options). Every other line is left alone.

You can edit the files by hand:
- An item you type by hand gets its key on the next change, or when you run `callboard keys`.
- An item you tick by hand has no `done-by:`, so Callboard works out who finished it from git.

## Files Callboard leaves alone

A `backlog.md`, `requests.md` or `questions.md` with no Callboard items in it is left alone. That covers an FAQ called `questions.md`, or a checklist from another tool. For a file like that:
- agents can read it;
- Callboard doesn't list it or write to it;
- merges go line by line;
- `callboard status` says so.

To hand a checklist like that over to Callboard, run `callboard keys`.

## Subtasks

An indented checkbox under a task or request is an item of its own, with its own key:

```markdown
- [ ] Streak counter on the home screen <!-- id:B-k3f9 … -->
  Count days in a row per habit.
  - [x] Count the current streak <!-- id:B-qb7n … -->
  - [ ] Reset on a missed day <!-- id:B-azaq … -->
```

The parent shows "1 of 2 subtasks done". It waits until they're all done, so it can't be ticked before that. Moving or deleting the parent takes its subtasks along, and when two branches tick different subtasks, git merges them cleanly.

To add one, use `callboard add "title" --under B-k3f9`, the `add` tool's `parent`, or **+ Add a subtask** in the item's panel. A question's options stay options, even if they're written as checkboxes.

## Your own lists

Besides the three, a repo can have lists of its own, such as ideas or bugs. `callboard lists add ideas` makes `ideas.md`, a task list with its own key letter (`I-`), and records it in `.callboard/lists.md`. You can also use **+ New list** on the page, or ask an agent, which uses the `lists` tool.

Its items work like tasks: claims, ticks, subtasks, fields, needs, views and item-by-item merging. Agents hear about the list at the start of each session, and add to it with `--kind ideas` or the tools' `kind`.

Commit `ideas.md`, `.callboard/lists.md` and `.gitattributes` (which gets the list's merge line) like the other lists.

- `callboard lists rename ideas someday` renames one. Its keys stay the same.
- `callboard lists remove ideas` removes it once nothing in it is open.

If two branches each add a list and both get the same key letter, both keep working after the merge.

## Answers

An answer is kept on the question itself, which is then done:

```markdown
- [x] Which chart library? <!-- id:Q-ch3l … done:2026-09-20T21:00 answer:"Swift Charts" why:"built in, and it matches the system look" answered-by:you -->
```

It reaches every worktree that has the question. Items that need it stay linked (`needs:Q-ch3l`) but stop waiting, and agents see the answer next to them.

`callboard reopen` asks it again. The old answer stays as `was:`, and whatever needs it waits again.

## Fields

Any other fact about an item is a field. You set one on the page, and agents set one with `callboard set B-k3f9 prio=high area="page ui"`. An empty value removes it. Field names are lowercase words, such as `status`, `prio`, `area` or `due`. Agents are told which fields and values are in use, so they reuse the same words.

`status` keeps in step by itself, in the list's own words (`todo`, `doing`, `in progress`, `done` and the like):
- claiming a task moves it to doing, and releasing it moves it back;
- ticking it makes it done, and setting it to done ticks it.

Some things Callboard already knows, so they need no field: what an item waits on is `needs`, and who is on it is the claim.

## Versions

An item's version is a hash of its line and notes. A session's own changes don't make its versions stale. For example, an agent that claims an item (which may change its status) can still tick it with the version it read, unless someone else changed the item in between. Each session's chain of versions is kept in `.git/callboard/own/`.

## Views

Views live in `.callboard/views.md`. Each view is an item whose fields are its settings:

```markdown
- [ ] By status <!-- id:V-bs7x by:you at:2026-09-25T09:00 list:backlog layout:board group:status order:todo,doing,done -->
- [ ] Most important first <!-- id:V-mi2p by:you at:2026-09-25T09:01 list:backlog layout:table sort:-prio show:prio,area -->
```

| Setting | Means |
|---|---|
| `list` | `backlog` (the default), `requests` or `questions` |
| `layout` | `list` (rows under group headings), `board` (a column per value; drag a card to change it, or within its column to reorder) or `table` (a column per field; click a cell to change it) |
| `group` | the field to group by: any field, or `section`, `done`, `by`, `at` (added), `doneat`, `blocked`, `claimed`, `needs` |
| `order` | the groups' order; other values follow |
| `sort` | fields to sort by, with `-` in front for the other way; priority words sort by importance (`-prio` puts urgent and high first) |
| `filter` | `field=value`, `field!=value` or `field=a\|b`; an empty value means not set |
| `show` | the fields shown on items (all of an item's fields when left out) |

When an agent makes or changes a view, it hears back what the view shows ("Board of backlog.md: 8 of 10 tasks; columns by status: todo 6, doing 2, done 0"). It also gets warnings about values no item has, or a view that shows nothing. A field no item has, like `priority` where the items use `prio`, is refused with the fields in use.

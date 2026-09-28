# Git and branches

[← Back to the README](../README.md)

The lists are ordinary files in your repo, so they go wherever your code goes: into commits, branches, pull requests and other clones. What Callboard adds is merging them **item by item**, so two branches that both changed the lists merge cleanly.

This works in every git step that merges files: `merge`, `pull`, `pull --rebase`, `rebase`, `cherry-pick`, `revert`, `stash pop` and `merge --squash`.

## What merges cleanly

- A tick on one branch lands together with changes on the other: a rename, a new field, or a move to another heading.
- Items added on both sides are both kept.
- What an item waits on merges as a set.
- Status goes with the tick: an item claimed on one branch and ticked on the other ends up done.
- A move to another heading lands even when the other side moved the item it sat next to.

## Conflicts

A conflict happens only when the same thing changed two ways:
- the same title, notes or field changed differently on each side;
- or an item changed on one side and deleted on the other.

Only that item gets conflict markers. They're named after the branches (`<<<<<<< main` … `>>>>>>> feat`). In a rebase, they're named after the branch you rebase onto and yours. Both versions hold everything that merged cleanly, so keeping either one loses only the other side of the clash.

The page shows what differs, like "prio low from main" or "prio high from feat". Click one to keep it, or run `callboard resolve KEY 1|2`. Then `git add` and commit (or `git rebase --continue`) as usual.

## Git's changes show as news

Git's changes show on the page and in agents' news like any other change: `git (merge feat) finished Alpha`, `git (reset to HEAD~1) removed Bravo`. An edit to a file by hand shows as `someone (edited backlog.md)`. Callboard compares the lists with how it last saw them, in `.git/callboard/seen/`, and names the git step from HEAD's reflog.

## Worktrees and history

Each worktree has its own lists. The event log, claims and read positions are shared by all worktrees of a repo, in `.git/callboard/`.

The log keeps about the last 20,000 changes. Past 8 MB it drops the oldest half, but keeps what you'd need to bring back a deleted item. Older history is in git, and `callboard restore` finds deleted items there too.

## Fresh clones and missing Callboard

A fresh clone has `.gitattributes`, but not the git config that names the merge driver. The first `callboard` command there, or an agent's session start, sets it (and `pull.octopus`) and says so.

When the `callboard` on git's PATH is missing (a GUI client, or a teammate without Callboard) or is older, git merges the list line by line. The markers say "merged line by line, without callboard", so one side is never kept silently. The next `callboard` command, or the open page, redoes that merge item by item and stages the list when nothing conflicts.

## Merging several branches at once

Git's own octopus merge (`git merge a b c`) skips merge drivers. So Callboard sets `pull.octopus = callboard`, and git then merges several branches at once through `git-merge-callboard`. That script merges the lists item by item, and every other file the way git's octopus would. Like git's, it stops if any branch but the last has a clash; merge those one at a time.

Some details:
- Without `callboard` on the PATH, the script hands the merge to git's octopus.
- A `pull.octopus` of your own is left alone, and `git merge -s octopus` still picks git's.
- Git finds the script through the PATH, so Callboard sets this only when `~/.local/bin` is on yours.
- A git app that runs without `~/.local/bin` on its PATH (some GUI clients) says `Could not find merge strategy 'callboard'` for such a merge. Git has no setting for a strategy's full path, so Callboard can't fix this. Use `git merge -s octopus`, or merge one branch at a time.

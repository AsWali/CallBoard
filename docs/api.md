# Page API

[← Back to the README](../README.md)

The page is a small JSON API on localhost, which you can script too.

| Endpoint | Does |
|---|---|
| `GET /api/board` | This worktree: the lists by section (`own` marks your own) with the fields in use, the saved views, merges to settle, claims, agent sessions, the last change, the last thing that happened to each item, other branches |
| `GET /api/branch?name=B` | Another branch's lists, read-only (live from its worktree, or its last commit) |
| `GET /api/branches` | Each branch's fork, main's list changes since, what the branch changes, and the items that would conflict |
| `GET /api/events` | Server-sent events: `board` when a list, the views or the claims changed, `change` with a new log event |
| `GET /api/log?limit=20` | The latest events here; with `key=B-k3f9`, that item's history in every worktree |
| `GET /api/agents` | The Claude Code and Codex sessions here |
| `POST /api/items` | `{"kind", "title", "section", "parent", "body", "options", "recommended", "needs", "fields"}` → `201 {"key", "version"}` |
| `PATCH /api/items/{key}` | Any of `{"done"}`, `{"title"}`, `{"needs"}`, `{"fields": {"status": "doing"}}`, `{"section"}`, `{"before": "B-k3f9"}` or `{"after": "B-k3f9"}`, with `"version"` → `200`, or `409` if it changed |
| `POST /api/items/{key}/answer` | `{"answer": "2" or "words", "why", "version"}` → `200 {"answer", "unblocked", "branches"}` |
| `POST /api/items/{key}/resolve` | `{"keep": 1 or 2}` after a merge conflict |
| `POST /api/items/{key}/reopen` | Ask an answered question again |
| `DELETE /api/items/{key}` · `POST /api/items/{key}/restore` | Delete an item → `{"key", "title", "freed"}` (what no longer waits on it), or put it back where it was |
| `POST /api/views` · `PATCH /api/views/{key}` | `{"name", "list", "layout", "group", "order", "sort", "filter", "show", "keep"}`; settings left out stay as they are |
| `DELETE /api/views/{key}` | Delete a view |
| `POST /api/lists` | `{"name": "ideas"}`: make your own list |
| `PATCH /api/lists/NAME` | `{"name": "someday"}`: rename it |
| `DELETE /api/lists/NAME` | remove it (`?force=1` if it has open items) |
| `POST /api/sections` | `{"kind", "name"}` and one of `"to"`, `"before"`, `"after"` or `"remove"` (with `"into"`): rename, move or remove a heading |

It listens only on localhost, answers only to localhost addresses, and refuses writes that come from other sites.

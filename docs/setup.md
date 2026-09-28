# Install and set up

[← Back to the README](../README.md)

## Install

On macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/AsWali/CallBoard/main/install.sh | sh
```

The script downloads the latest release for your computer, checks it against the release's checksums, and puts `callboard` in `~/.local/bin`. To pick another folder, set `CALLBOARD_DIR=…`. To pick another release, set `CALLBOARD_VERSION=v0.4.0`.

On Windows, download `callboard-…-windows-amd64.exe` from [Releases](https://github.com/AsWali/CallBoard/releases).

If you have Go 1.25 or newer, either of these works too:

```bash
go install github.com/AsWali/CallBoard/cmd/callboard@latest
# or from a checkout:
go build -o bin/callboard ./cmd/callboard && bin/callboard install
```

## Connect your agents

```bash
callboard setup --global
```

You do this once. It connects Claude Code, Codex and git for every project. It only changes your own settings, never the files in your projects:

- **Claude Code:** the MCP server (user scope), five hooks and the `/callboard` command.
- **Codex:** the MCP server, the same five hooks, and a short section in `~/.codex/AGENTS.md`. Codex asks you once to approve the hooks: type `/hooks` in Codex and trust them.
- **Git:** the merge driver and the global attributes file. It also sets `pull.octopus = callboard` and writes a small `~/.local/bin/git-merge-callboard` script, so a merge of several branches at once also merges the lists item by item.

In a folder that isn't a git repo and has no lists, the hooks stay quiet and write nothing.

- `callboard status --global` shows what's connected.
- `callboard disconnect --global` undoes all of it. The lists stay in each project.

## Set up one repo for a team

If your teammates haven't set Callboard up themselves, connect one repo instead:

```bash
callboard setup
```

This writes files meant to be committed, so every worktree, branch and teammate gets them:

- `.mcp.json`: the `callboard` MCP server for Claude Code.
- `.claude/settings.json`: hooks for session start, prompts, tool calls, reads of the lists, and subagents.
- `.gitattributes`: says that the three lists, your own lists and `.callboard/views.md` merge with `merge=callboard`, and `.callboard/lists.md` with `merge=union`.
- For Codex, when it's installed (or with `callboard setup codex`): `AGENTS.md` (a short section between `<!-- callboard -->` markers), `.codex/config.toml` (the MCP server) and `.codex/hooks.json` (the same five hooks).

A few files stay yours and aren't committed:

- `.claude/settings.local.json`, which lets Claude Code start the server without asking;
- the repo's git config, which holds the merge driver and `pull.octopus`;
- `~/.local/bin/callboard` and `~/.local/bin/git-merge-callboard`.

Codex loads a repo's `.codex/` files only after you trust the folder and approve the hooks with `/hooks`. That trust is yours to give, so setup doesn't write it. Until then, Codex runs `callboard prime` from `AGENTS.md`.

Setup keeps the key order, the indentation and the one-line lists of the JSON files it edits. `callboard disconnect` removes all of it except the two `~/.local/bin` files, which other projects may use. The lists and the event log stay.

## Open the page

```bash
callboard serve --open
```

The page never starts by itself. It runs while `callboard serve` does, and each worktree gets its own port between 4700 and 4999. See [the page](page.md).

## Switch it off in one repo

`callboard off` switches Callboard off in one repo. The hooks say nothing there, and the lists become read-only: `/callboard`, `callboard show`, the page and the MCP `list` tool still show them, but nothing changes them and Callboard writes nothing in the repo, not even in `.git`. You might want this in a repo whose `backlog.md` is a document of its own. The setting is kept in the repo's own git config, so nothing is committed. `callboard on` switches it back on.

You often won't need to. A `backlog.md`, `requests.md` or `questions.md` with no Callboard items in it, such as an FAQ or another tool's checklist, is left alone anyway. See [the files](files.md#files-callboard-leaves-alone).

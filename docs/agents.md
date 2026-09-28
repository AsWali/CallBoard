# How agents use it

[← Back to the README](../README.md)

Once you've run `callboard setup --global`, Claude Code and Codex use Callboard by themselves. You don't have to tell them to. This is what they get.

- **The lists at the start.** When a session or subagent starts, it hears:
  - each list's open items with their fields, and each question's options;
  - the next task to do: ready, nobody on it, most important first;
  - the fields and views in use;
  - what changed while it was away;
  - how to keep the lists.
- **News as it happens.** After each prompt and tool call, it hears what you, another agent or git changed, like: `you answered Q-sy2c "Where should synced habits live?": Supabase`. Without hooks, the same news comes along in the MCP tools' replies.
- **Claims.** An agent claims a task before it starts, so two agents don't do the same work. The claim shows its agent and branch everywhere, and follows a renamed branch. It lasts while the agent's session runs. Once the session ends or goes quiet, the claim lets go after 30 minutes.
- **Notes.** A plan or a finding goes under the item it's about, not into a new item. Agents add one with the `edit` tool's `add_note` (or `notes` to replace them all), or with `callboard note B-k3f9 "text"`.
- **Versions.** Every item has a short version, shown as `B-k3f9@7c`. A change carries the version it read. If someone changed the item since, the change is refused and the agent sees the current item. So two agents, or an agent and you, never overwrite each other.
- **Assumed answers.** An agent that can't wait for you picks an option and carries on (`callboard assume Q-sy2c 1`). The question stays open for you:
  - If you answer the same way, nothing more happens.
  - If you answer differently, the finished work that depended on it opens again, and the agent is told: `opened B-7tg8 "Offline sync with a local SQLite cache" again: Q-sy2c was answered "Our own Postgres on Fly.io", not the assumed "Supabase". Redo it for the real answer.`
- **Short replies.** Every tool answers in a few lines of text, the same lines `callboard` prints, with no JSON copy. An agent reads a list in about a quarter of the tokens it would take to read the files.
- **Pages of 50.** When an agent lists items without a filter, it gets the open ones 50 at a time, with where the next page starts.
- **Reading goes through Callboard.** If an agent tries to read `backlog.md`, `requests.md` or `questions.md` directly, the hook hands it the same items through Callboard instead, so it sees keys, claims and what waits on what.

## Claude Code and Codex

Claude Code and Codex get all of this. The differences are small:
- Codex has no custom slash commands, so you type `!callboard …` where Claude Code uses `/callboard …`.
- Codex's sandbox lets commands write to the worktree but not to `.git`, so its changes go through the MCP tools, which run outside the sandbox.
- Codex asks you once to approve Callboard's hooks: type `/hooks` in Codex and trust them.

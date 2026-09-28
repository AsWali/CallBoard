# Questions people ask

[← Back to the README](../README.md)

### Do I need to know git?

No. Callboard keeps its lists in your project folder. If your project uses git, the lists are saved with your code, and Callboard makes merges work for them. If you never touch git yourself, you'll just see three extra files in your project: `backlog.md`, `requests.md` and `questions.md`.

### Do I have to tell my agent to use it?

No. After `callboard setup --global`, Claude Code and Codex hear the lists at the start of every session and keep them up to date as they work. You can still ask directly: "add that to the backlog", "what's waiting on me?", "pick up the next task".

### What's the difference between a task, a request and a question?

- A **task** is work an agent can do itself: "Add a settings screen".
- A **request** is something only you can do: create an account, paste an API key, pay for something, deploy. Agents write the exact steps, down to which name or environment variable to use.
- A **question** is a decision that's yours: "Which sign-in should the app have?" Agents give you the options and mark the one they recommend.

### What if I don't answer a question?

An agent that can't wait picks an option, usually the one it recommended, and carries on. The question stays open for you. If you later pick something else, the work that depended on it opens again, and the agent is told to redo it.

### Where do I answer?

Wherever you like:
- On the page (`callboard serve --open`), under **For you**, with one click.
- In the terminal: `/callboard you` in Claude Code shows what's waiting on you.
- Just tell the agent in the chat: "use Supabase for Q-sy2c".

### Does it cost tokens?

A little. Agents get a short summary at the start of each session, and a line of news when something changes. Callboard's replies are short text, about a quarter of the tokens it would take the agent to read the files. `/callboard` in Claude Code costs nothing: Callboard answers it before the model sees it.

### Does it send my code or my lists anywhere?

No. Callboard is one program on your computer, with no account and no server. The page runs only while `callboard serve` does, and only your own computer can open it. The lists travel only where you push your repo.

### Can I edit the lists myself?

Yes, they're plain checklists. Tick a box, change a title or add a line in any editor, and agents hear about the change. A line you add gets its key the next time Callboard changes that list.

### My project already has a `backlog.md` or a `questions.md`.

If it has no Callboard items in it, Callboard leaves it alone: it doesn't list it, write to it or add keys to it, and `callboard status` tells you. To let Callboard take over a checklist like that, run `callboard keys`. To keep Callboard out of a repo entirely, run `callboard off` there.

### Several agents at once: won't they trip over each other?

That's what claims are for. An agent claims a task before starting it, so another agent won't pick the same one. Every change also carries the version it read, so nobody overwrites anyone else's edit. This works across Claude Code and Codex, several sessions, and different branches.

### Which agents does it work with?

Claude Code and Codex. Other tools that support MCP servers can use the `callboard mcp` server, but they won't get the hooks that tell the agent about changes as they happen.

### How do I remove it?

`callboard disconnect --global` undoes the setup, and `callboard uninstall` removes the program from `~/.local/bin`. Your lists stay in your projects as plain Markdown.

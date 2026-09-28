# Working on Callboard

[← Back to the README](../README.md)

`cmd/callboard` is the CLI. The packages in `internal/` are:

- `board`: parse and write a list; notes, placing, versions, the per-field merge of one item, conflicts
- `store`: the lists (the three and your own) and the views, locked changes, fields, the event log, claims, news
- `show`: the lists drawn for a terminal (`/callboard`, `show`, `watch`)
- `overview`: what other branches changed
- `mcpserver`: the MCP tools
- `server`: the page, with its `web/` built in
- `merge`: the git merge driver
- `setup`: connecting Claude Code, Codex and git
- `gitx`: the git calls

Run the tests with `go vet ./... && go test ./...`. `scripts/release.sh` builds the release binaries for macOS, Linux and Windows (`VERSION=x.y.z` names them); pushing a `v*` tag that matches the version in `cmd/callboard/main.go` makes a GitHub release with them.

# claude_patcher

TUI for distributing Claude Code configuration, skills, and harness patches.

Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). One static binary per OS, no runtime needed.

## Screens

- **Skills**: multi-select install/uninstall of the skills bundled in `skills/` into `~/.claude/skills`.
- **Config**: toggles for `~/.claude/settings.json` keys and managed blocks in `~/.claude/CLAUDE.md`.
- **Patches**: harness changes, applied as hooks that call this binary.

Keys: `space` toggle, `a` all, `enter` apply, `esc` back. A `*` marks a pending change.

The first write to `settings.json` saves the original as `settings.json.claude_patcher.bak`. Set `CLAUDE_CONFIG_DIR` to target another config folder.

## Patches

Each patch registers `claude_patcher hook <name>` in `settings.json`. Hook paths are absolute, so re-enable patches if you move the binary.

| Patch | Hook | What it does |
|---|---|---|
| Time awareness | `timestamp` | Adds the time, time on the current request and session age to every prompt and tool result |
| Command guard | `guard` | Denies `rm -rf /` and similar, force push to main/master, `DROP TABLE`, `.env` reads; asks before `curl \| sh`, `git reset --hard`, other force pushes |
| Verify before done | `verify` | After Claude edits files, runs the project checks at the end of the turn and blocks finishing while they fail (3 tries) |
| Stuck detector | `stuck` | Warns Claude when it repeats an identical tool call 3+ times in its last 12 |
| Context budget | `context` | Tells Claude when context passes 50, 70, 85 and 95 percent. Set `CLAUDE_PATCHER_CONTEXT_WINDOW` to override the window size |
| Format and lint | `check` | Formats each edited file (gofmt, rustfmt, ruff, prettier) and feeds lint errors back (go vet, ruff, eslint). Missing tools are skipped |
| Git checkpoints | `checkpoint` | Snapshots the work tree at the end of each turn to `refs/claude-checkpoints/<session>` without touching your branch or index |
| Compaction memory | `compact` | Before compaction saves recent requests, edited files, the todo list and a transcript copy; gives the summary back after |
| Injection scan | `injection` | Warns Claude when web, shell or MCP output contains instructions aimed at it |
| Phone alerts | `alerts` | [ntfy](https://ntfy.sh) push when Claude finishes or waits; Allow/Deny buttons for permission prompts |

Verify checks are detected from `go.mod`, `Cargo.toml`, `package.json` scripts (`typecheck`, `lint`, `test`) and pytest. Override them per project in `.claude/claude_patcher.json`:

```json
{ "verify": ["make test"] }
```

Restore a checkpoint: `git for-each-ref refs/claude-checkpoints`, then `git checkout <ref> -- .`

Phone alerts settings live in `~/.claude/claude_patcher/alerts.json` (topic, server, `approve_wait_seconds`, `notify_on_stop`). A permission prompt waits that many seconds for a phone answer before showing in the terminal; set it to 0 to turn remote approval off. Prompt text goes to the ntfy server, so use your own server for sensitive work.

## Adding skills

Drop a folder with a `SKILL.md` into `skills/` and cut a release. Skills are compiled into the binary.

## Run

```sh
go run .
```

## Install

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/aveekpatra/claude_patcher/main/install.sh | sh
# or
wget -qO- https://raw.githubusercontent.com/aveekpatra/claude_patcher/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/aveekpatra/claude_patcher/main/install.ps1 | iex
```

## Release

Push a tag (`git tag v0.1.0 && git push --tags`). GitHub Actions builds binaries for macOS, Linux and Windows (amd64 + arm64) with GoReleaser.

## License

GPL-3.0. Free and open source: issues, pull requests and stars welcome.

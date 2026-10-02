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

### Time awareness

Registers `claude_patcher hook timestamp` on `UserPromptSubmit` and `PostToolUse`. Every prompt and tool result then carries a line such as:

```
[clock] Now 2026-10-02 18:39:54 CEST (Fri). Working on the current request for 11s; session running 11s.
```

The hook stores its paths as absolute paths to the binary, so re-enable the patch if you move it.

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

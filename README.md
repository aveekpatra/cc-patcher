# claude_patcher

TUI for distributing Claude Code configuration, skills, and harness patches.

Built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea).

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

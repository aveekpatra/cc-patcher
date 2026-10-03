# cc-patcher

A terminal app that sets up Claude Code the way you like it, in one place, on any machine.

Claude Code is configured through a pile of files: `settings.json`, `CLAUDE.md`, hook scripts, skills folders, environment variables. cc-patcher puts all of it behind one checklist. Tick what you want, press enter, and it writes the files for you. Untick it and the change is undone, including any value it replaced.

![cc-patcher home screen](docs/home.png)

## Install or update

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/aveekpatra/cc-patcher/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/aveekpatra/cc-patcher/main/install.ps1 | iex
```

Run the same command again to update. Then start it with `cc-patcher`.

## What it does

- **Patches**: change what no setting can. They run inside Claude Code as hooks, using the cc-patcher binary itself, so they need no Python or Node. Items marked with a red `!` go further and edit Claude Code's own program files.
- **Config**: change what Claude Code already lets you set: `settings.json` keys, environment variables, the status line, alone or as presets.
- **Skills**: instructions Claude reads. Install or remove any skill in `~/.claude/skills`, yours or bundled or downloaded from GitHub, and switch always-on `CLAUDE.md` rules on and off.

![cc-patcher patches screen](docs/patches.png)

![cc-patcher config screen](docs/config.png)

Every item shows what it changes before you turn it on. Use `/` to filter, `space` to tick, `enter` to apply.

## Save and share your setup

cc-patcher remembers what you turned on in `~/.claude/cc-patcher/profile.json`. Export it, then apply it anywhere in one go:

```sh
cc-patcher export my-setup.json      # or press e on the home screen
cc-patcher import my-setup.json      # a file or an https:// URL; or press i
```

Import makes each section in the file match it exactly: listed items are turned on, the rest of that section off.

The Skills screen lists every skill in `~/.claude/skills`, not just the ones cc-patcher ships, and any of them can be uninstalled. Uninstalled skills go to `~/.claude/cc-patcher/removed-skills`, so ticking them again brings them back; for a linked skill only the link moves. Exports carry the files of your own skills, so importing on a new machine installs them too.

## Good to know

- The first time cc-patcher edits `~/.claude/settings.json`, it saves the original next to it as `settings.json.cc-patcher.bak`.
- Patches point at the cc-patcher binary by its full path. If you move it, turn the patches off and on again.
- Set `CLAUDE_CONFIG_DIR` to try it against a scratch folder first: `CLAUDE_CONFIG_DIR=/tmp/cc-test cc-patcher`.

## Contributing

Issues and pull requests are welcome. To build from source you need Go:

```sh
go run .
go test ./...
```

A bundled skill is a folder with a `SKILL.md` under `skills/`. Releases are built by GitHub Actions when a `v*` tag is pushed.

## License

GPL-3.0. Free and open source.

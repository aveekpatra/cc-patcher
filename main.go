package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/aveekpatra/cc-patcher/internal/config"
	"github.com/aveekpatra/cc-patcher/internal/files"
	"github.com/aveekpatra/cc-patcher/internal/patches"
	"github.com/aveekpatra/cc-patcher/internal/skills"
	"github.com/aveekpatra/cc-patcher/internal/statusline"
	"github.com/aveekpatra/cc-patcher/internal/tui"
)

// version is set at release time by GoReleaser.
var version = "dev"

//go:embed all:skills
var bundled embed.FS

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "hook":
			if len(os.Args) > 2 {
				patches.RunHook(os.Args[2], os.Stdin, os.Stdout)
			}
			return
		case "files":
			files.Run(os.Stdin, os.Stdout)
			return
		case "statusline":
			if len(os.Args) > 2 {
				statusline.Run(os.Args[2], os.Stdin, os.Stdout)
			}
			return
		case "version", "--version", "-v":
			fmt.Println(version)
			return
		}
	}

	skillsFS, _ := fs.Sub(bundled, "skills")
	sections := []tui.Section{
		{Title: "Skills", Summary: "instructions Claude reads: skills and CLAUDE.md rules", Items: func() []tui.Toggle {
			var out []tui.Toggle
			for _, r := range config.Rules(skillsFS) {
				out = append(out, r)
			}
			all, _ := skills.List(skillsFS)
			for _, s := range all {
				out = append(out, s)
			}
			for _, r := range skills.RemoteList() {
				out = append(out, r)
			}
			return out
		}},
		{Title: "Config", Summary: "Claude Code settings, env vars and status line", Items: func() []tui.Toggle {
			var out []tui.Toggle
			for _, o := range config.Options() {
				out = append(out, o)
			}
			for _, p := range patches.StatusLine() {
				out = append(out, p)
			}
			return out
		}},
		{Title: "Patches", Summary: "change what no setting can: hooks and program edits", Items: func() []tui.Toggle {
			all := patches.All()
			out := make([]tui.Toggle, len(all), len(all)+1)
			for i, p := range all {
				out[i] = p
			}
			return append(out, patches.TweakccStudio{})
		}},
	}
	if err := tui.Run(version, sections); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

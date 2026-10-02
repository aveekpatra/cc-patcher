package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/aveekpatra/cc-patcher/internal/config"
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
		{Title: "Skills", Summary: "install or remove bundled skills", Items: func() []tui.Toggle {
			all, _ := skills.List(skillsFS)
			out := make([]tui.Toggle, len(all))
			for i, s := range all {
				out[i] = s
			}
			return out
		}},
		{Title: "Config", Summary: "Claude Code settings and CLAUDE.md", Items: func() []tui.Toggle {
			all := config.Options(skillsFS)
			out := make([]tui.Toggle, len(all))
			for i, o := range all {
				out[i] = o
			}
			return out
		}},
		{Title: "Patches", Summary: "change how the harness behaves", Items: func() []tui.Toggle {
			all := patches.All()
			out := make([]tui.Toggle, len(all))
			for i, p := range all {
				out[i] = p
			}
			return out
		}},
	}
	if err := tui.Run(version, sections); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

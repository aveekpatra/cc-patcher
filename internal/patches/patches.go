// Package patches changes how the Claude Code harness behaves, mostly by
// registering claude_patcher itself as a hook command.
package patches

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

// Patch is one harness change.
type Patch struct {
	name, desc string
	// events maps a hook event name to its matcher ("" for events without one).
	events map[string]string
	// hookArg is passed as `claude_patcher hook <hookArg>`.
	hookArg string
}

func (p *Patch) Name() string        { return p.name }
func (p *Patch) Description() string { return p.desc }

// marker identifies hook commands owned by this patch.
func (p *Patch) marker() string { return " hook " + p.hookArg }

func (p *Patch) command() string {
	exe, err := os.Executable()
	if err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
	} else {
		exe = "claude_patcher"
	}
	if runtime.GOOS == "windows" {
		exe = filepath.ToSlash(exe)
	}
	return `"` + exe + `"` + p.marker()
}

func (p *Patch) Enabled() bool {
	s, err := claude.LoadSettings()
	if err != nil {
		return false
	}
	hooks, _ := s["hooks"].(map[string]any)
	for event := range p.events {
		if !hasCommand(hooks[event], p.marker()) {
			return false
		}
	}
	return true
}

func (p *Patch) Enable() error {
	return claude.Update(func(s claude.Settings) {
		hooks := removeHooks(s, p.marker())
		for event, matcher := range p.events {
			group := map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": p.command(), "timeout": 5}},
			}
			if matcher != "" {
				group["matcher"] = matcher
			}
			list, _ := hooks[event].([]any)
			hooks[event] = append(list, group)
		}
		s["hooks"] = hooks
	})
}

func (p *Patch) Disable() error {
	return claude.Update(func(s claude.Settings) {
		hooks := removeHooks(s, p.marker())
		if len(hooks) == 0 {
			delete(s, "hooks")
		} else {
			s["hooks"] = hooks
		}
	})
}

func hasCommand(groups any, marker string) bool {
	list, _ := groups.([]any)
	for _, g := range list {
		gm, _ := g.(map[string]any)
		hs, _ := gm["hooks"].([]any)
		for _, h := range hs {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); isOurs(cmd, marker) {
				return true
			}
		}
	}
	return false
}

func isOurs(cmd, marker string) bool {
	return strings.HasSuffix(cmd, `"`+marker)
}

// removeHooks strips every hook command carrying marker, dropping groups and
// events left empty. It returns the (possibly new) hooks map.
func removeHooks(s claude.Settings, marker string) map[string]any {
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		return map[string]any{}
	}
	for event, groups := range hooks {
		list, _ := groups.([]any)
		var keptGroups []any
		for _, g := range list {
			gm, ok := g.(map[string]any)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			hs, _ := gm["hooks"].([]any)
			var kept []any
			for _, h := range hs {
				hm, _ := h.(map[string]any)
				if cmd, _ := hm["command"].(string); isOurs(cmd, marker) {
					continue
				}
				kept = append(kept, h)
			}
			if len(kept) > 0 {
				gm["hooks"] = kept
				keptGroups = append(keptGroups, gm)
			}
		}
		if len(keptGroups) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = keptGroups
		}
	}
	return hooks
}

// All returns every available patch.
func All() []*Patch {
	return []*Patch{
		{
			name:    "Time awareness",
			desc:    "Adds the current time and elapsed time to every prompt and tool call",
			events:  map[string]string{"UserPromptSubmit": "", "PostToolUse": "*"},
			hookArg: "timestamp",
		},
	}
}

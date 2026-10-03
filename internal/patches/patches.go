// Package patches changes how the Claude Code harness behaves by
// registering cc-patcher itself as a hook command.
package patches

import (
	"strings"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// reg is one hook registration in settings.json.
type reg struct {
	event   string
	matcher string // "" for events without one
	timeout int    // seconds
	async   bool
}

// Patch is one harness change.
type Patch struct {
	name, desc string
	// hookArg is passed as `cc-patcher hook <hookArg>`.
	hookArg  string
	regs     []reg
	warn     string        // optional caution shown with a red badge
	setup    func() error  // optional, runs before enabling
	teardown func() error  // optional, runs after disabling
	note     func() string // optional, shown after enabling
	// Optional overrides for patches that do not use hooks (status line).
	enabled func() bool
	enable  func() error
	disable func() error
}

func (p *Patch) Name() string        { return p.name }
func (p *Patch) Description() string { return p.desc }
func (p *Patch) Warning() string     { return p.warn }

// Details lists the hooks the patch registers.
func (p *Patch) Details() string {
	if len(p.regs) == 0 {
		return ""
	}
	var parts []string
	for _, r := range p.regs {
		e := r.event
		if r.matcher != "" && r.matcher != "*" {
			e += " (" + r.matcher + ")"
		}
		parts = append(parts, e)
	}
	return "Hooks: " + strings.Join(parts, ", ") + "\nRuns: cc-patcher hook " + p.hookArg
}

// Note is shown in the TUI after the patch is enabled.
func (p *Patch) Note() string {
	if p.note == nil {
		return ""
	}
	return p.note()
}

// marker identifies hook commands owned by this patch.
func (p *Patch) marker() string { return " hook " + p.hookArg }

func (p *Patch) command() string { return `"` + exePath() + `"` + p.marker() }

// exePath is where hooks run cc-patcher from.
func exePath() string { return claude.SelfPath() }

func (p *Patch) Enabled() bool {
	if p.enabled != nil {
		return p.enabled()
	}
	s, err := claude.LoadSettings()
	if err != nil {
		return false
	}
	hooks, _ := s["hooks"].(map[string]any)
	for _, r := range p.regs {
		if !hasCommand(hooks[r.event], p.marker()) {
			return false
		}
	}
	return true
}

func (p *Patch) Enable() error {
	if p.setup != nil {
		if err := p.setup(); err != nil {
			return err
		}
	}
	if p.enable != nil {
		return p.enable()
	}
	return claude.Update(func(s claude.Settings) {
		hooks := removeHooks(s, p.marker())
		for _, r := range p.regs {
			h := map[string]any{"type": "command", "command": p.command(), "timeout": r.timeout}
			if r.async {
				h["async"] = true
			}
			group := map[string]any{"hooks": []any{h}}
			if r.matcher != "" {
				group["matcher"] = r.matcher
			}
			list, _ := hooks[r.event].([]any)
			hooks[r.event] = append(list, group)
		}
		s["hooks"] = hooks
	})
}

func (p *Patch) Disable() error {
	if p.teardown != nil {
		if err := p.teardown(); err != nil {
			return err
		}
	}
	if p.disable != nil {
		return p.disable()
	}
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

const editTools = "Edit|Write|MultiEdit|NotebookEdit"

// extraPatches lets other files in this package register patches from
// init(), so each feature lives in its own file.
var extraPatches []*Patch

// All returns every available patch.
func All() []*Patch {
	return append(corePatches(), extraPatches...)
}

func corePatches() []*Patch {
	return []*Patch{
		{
			name: "Time awareness", hookArg: "timestamp",
			desc: "Adds the time and elapsed time to sessions, subagents, prompts and tool calls; explains it in CLAUDE.md",
			regs: []reg{
				{"SessionStart", "", 5, false}, {"SubagentStart", "", 5, false},
				{"UserPromptSubmit", "", 5, false}, {"PostToolUse", "*", 5, false},
			},
			setup:    func() error { return claude.SetBlock(claude.ClaudeMdPath(), "clock", clockNote) },
			teardown: func() error { return claude.RemoveBlock(claude.ClaudeMdPath(), "clock") },
		},
		{
			name: "Command guard", hookArg: "guard",
			desc: "Blocks rm -rf /, force push to main, DROP TABLE, .env reads; asks before curl | sh, reset --hard",
			regs: []reg{{"PreToolUse", "Bash|Read|Edit|Write|MultiEdit", 5, false}},
		},
		{
			name: "Verify before done", hookArg: "verify",
			desc: "After edits, Claude cannot finish until tests and lint pass (3 tries)",
			regs: []reg{{"PostToolUse", editTools, 5, false}, {"Stop", "", 600, false}},
		},
		{
			name: "Stuck detector", hookArg: "stuck",
			desc: "Warns Claude when it repeats the same tool call",
			regs: []reg{{"PreToolUse", "*", 5, false}},
		},
		{
			name: "Context budget", hookArg: "context",
			desc: "Tells Claude when its context passes 50, 70, 85 and 95 percent",
			regs: []reg{{"UserPromptSubmit", "", 5, false}, {"PostToolUse", "*", 5, false}},
		},
		{
			name: "Format and lint", hookArg: "check",
			desc: "Formats each edited file and feeds lint errors back (gofmt, prettier, ruff, eslint...)",
			regs: []reg{{"PostToolUse", "Edit|Write|MultiEdit", 60, false}},
		},
		{
			name: "Git checkpoints", hookArg: "checkpoint",
			desc: "Snapshots the work tree each turn into refs/claude-checkpoints/<session>",
			regs: []reg{{"Stop", "", 60, true}},
		},
		{
			name: "Compaction memory", hookArg: "compact",
			desc: "Saves requests, edited files and todos before compaction, restores them after",
			regs: []reg{{"PreCompact", "", 30, false}, {"SessionStart", "compact", 10, false}},
		},
		{
			name: "Injection scan", hookArg: "injection",
			desc: "Warns Claude when web, shell or MCP output contains instructions aimed at it",
			regs: []reg{{"PostToolUse", "WebFetch|WebSearch|Bash|mcp__.*", 5, false}},
		},
		{
			name: "Phone alerts", hookArg: "alerts",
			desc:  "ntfy push when Claude finishes or waits; approve or deny prompts from the phone",
			regs:  []reg{{"Stop", "", 15, true}, {"Notification", "", 15, true}, {"PermissionRequest", "", 120, false}},
			setup: setupAlerts, note: alertsNote,
		},
	}
}

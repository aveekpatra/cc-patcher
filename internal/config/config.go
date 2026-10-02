// Package config holds the Claude Code configuration options the TUI offers.
package config

import (
	"encoding/json"
	"io/fs"
	"reflect"

	"github.com/aveekpatra/claude_patcher/internal/claude"
	"github.com/aveekpatra/claude_patcher/internal/skills"
)

// Option is a config toggle backed by a pair of enable/disable functions.
type Option struct {
	name, desc string
	enabled    func() bool
	enable     func() error
	disable    func() error
}

func (o *Option) Name() string        { return o.name }
func (o *Option) Description() string { return o.desc }
func (o *Option) Enabled() bool       { return o.enabled() }
func (o *Option) Enable() error       { return o.enable() }
func (o *Option) Disable() error      { return o.disable() }

// setting toggles a single top-level key in settings.json.
func setting(name, desc, key string, value any) *Option {
	return &Option{
		name: name,
		desc: desc,
		enabled: func() bool {
			s, err := claude.LoadSettings()
			return err == nil && sameJSON(s[key], value)
		},
		enable:  func() error { return claude.Update(func(s claude.Settings) { s[key] = value }) },
		disable: func() error { return claude.Update(func(s claude.Settings) { delete(s, key) }) },
	}
}

// block toggles a managed section of ~/.claude/CLAUDE.md.
func block(name, desc, id, body string, before func() error) *Option {
	return &Option{
		name:    name,
		desc:    desc,
		enabled: func() bool { return claude.HasBlock(claude.ClaudeMdPath(), id) },
		enable: func() error {
			if before != nil {
				if err := before(); err != nil {
					return err
				}
			}
			return claude.SetBlock(claude.ClaudeMdPath(), id, body)
		},
		disable: func() error { return claude.RemoveBlock(claude.ClaudeMdPath(), id) },
	}
}

func sameJSON(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	var va, vb any
	_ = json.Unmarshal(ja, &va)
	_ = json.Unmarshal(jb, &vb)
	return reflect.DeepEqual(va, vb)
}

const conciseBody = `# Default response style

@~/.claude/skills/min/SKILL.md

Always apply the imported ` + "`min`" + ` skill to every response, even when the user does not invoke ` + "`/min`" + `. Treat an unqualified request as ` + "`/min`" + `. Apply ` + "`bullets`" + ` or ` + "`code`" + ` when the user supplies those arguments, and follow any explicit request for a different response style.`

const asciiBody = `# Character policy

Use ASCII characters only in all generated text unless the user explicitly requires other Unicode content. Never output or write Unicode code point U+2014 anywhere, including chat, commentary, code, documentation, filenames, commit messages, merge requests, tool inputs, and generated files. Use ASCII punctuation such as hyphen-minus (-), colon (:), comma (,), semicolon (;), or parentheses instead. This prohibition has no exceptions, even when asked to reproduce or quote that character.`

const waitingBody = `# Waiting on people or slow systems

When you are blocked on something outside your control (a person replying, a review, a merge, a deploy, CI), do not end your turn just to say you are still waiting, and do not re-check in a tight loop. Instead:

1. Say once what you are waiting for and who or what can unblock it.
2. Start a background wait, such as ` + "`sleep 900`" + ` run in the background, so a /goal or loop pauses its checks while you wait. When it finishes, check once more.
3. After about 3 checks with no change, stop, tell the user exactly what is blocked and what they can do, and if a /goal is active, suggest ` + "`/goal clear`" + ` or a narrower goal.

Do other unblocked work while you wait if there is any.`

// Options returns every config option. skillsFS is used by options that
// depend on a bundled skill.
func Options(skillsFS fs.FS) []*Option {
	installMin := func() error {
		if s := skills.Find(skillsFS, "min"); s != nil && !s.Enabled() {
			return s.Enable()
		}
		return nil
	}
	return []*Option{
		block("Concise replies", "CLAUDE.md: always answer in min style (installs the min skill)", "concise", conciseBody, installMin),
		block("Patient waiting", "CLAUDE.md: wait with background sleeps instead of looping when blocked on people or CI", "waiting", waitingBody, nil),
		block("ASCII-only output", "CLAUDE.md: ASCII only, never the em dash character", "ascii", asciiBody, nil),
		setting("Concise output style", `settings.json: outputStyle = "Concise"`, "outputStyle", "Concise"),
		setting("No co-author trailer", "settings.json: includeCoAuthoredBy = false", "includeCoAuthoredBy", false),
		setting("Fullscreen TUI", `settings.json: tui = "fullscreen"`, "tui", "fullscreen"),
	}
}

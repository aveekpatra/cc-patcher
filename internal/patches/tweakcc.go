package patches

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// tweakcc (https://github.com/Piebald-AI/tweakcc, MIT) patches Claude Code's
// own code: system prompts, toolsets, per-subagent models, themes and the
// spinner. It needs Node (npx) and its changes are lost on every Claude
// Code update, which the re-apply patch below handles.

func init() {
	extraPatches = append(extraPatches, &Patch{
		name: "Keep tweakcc tweaks", hookArg: "tweakcc",
		desc: "Re-applies your tweakcc customizations at session start after Claude Code updates (needs Node)",
		regs: []reg{{"SessionStart", "startup", 300, true}},
		setup: func() error {
			if _, err := exec.LookPath("npx"); err != nil {
				return errors.New("needs Node.js (npx) on PATH")
			}
			return nil
		},
	})
	handlers["tweakcc"] = tweakccHook
}

// TweakccStudio opens tweakcc's own editor for system prompts, toolsets,
// subagent models, themes and spinners.
type TweakccStudio struct{}

func (TweakccStudio) Name() string { return "tweakcc studio" }
func (TweakccStudio) Description() string {
	return "Opens tweakcc: edit system prompts, toolsets, subagent models, themes (FRAGILE: patches Claude Code itself)"
}
func (TweakccStudio) Enabled() bool {
	home, _ := os.UserHomeDir()
	return fileExists(filepath.Join(home, ".tweakcc", "config.json"))
}
func (TweakccStudio) Enable() error { return nil }
func (TweakccStudio) Disable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, err := run(exec.CommandContext(ctx, "npx", "-y", "tweakcc@latest", "--restore", "--yes"), "")
	return err
}

// Launch is run by the TUI in the foreground when the item is switched on.
func (TweakccStudio) Launch() *exec.Cmd { return exec.Command("npx", "-y", "tweakcc@latest") }

type tweakccState struct {
	Claude  string    `json:"claude"`
	ModTime time.Time `json:"mod_time"`
}

// tweakccHook re-applies tweakcc when the Claude Code install changed since
// the last apply.
func tweakccHook(_ *Input, _ time.Time) any {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil
	}
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}
	info, err := os.Stat(bin)
	if err != nil {
		return nil
	}
	p := filepath.Join(claude.StateDir(), "tweakcc.json")
	var st tweakccState
	if b, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Claude == bin && st.ModTime.Equal(info.ModTime()) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if _, err := run(exec.CommandContext(ctx, "npx", "-y", "tweakcc@latest", "--apply", "--yes"), ""); err != nil {
		return nil
	}
	if info, err := os.Stat(bin); err == nil {
		st = tweakccState{Claude: bin, ModTime: info.ModTime()}
		b, _ := json.Marshal(st)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, b, 0o644)
	}
	return nil
}

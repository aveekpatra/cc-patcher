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
// Code update, which the re-apply patch below handles. Claude Code updates
// itself in the background, so the check also runs at the end of every
// turn: the tweaks are back before the next launch, not one launch late.

func init() {
	extraPatches = append(extraPatches, &Patch{
		name: "Keep tweakcc tweaks", hookArg: "tweakcc",
		desc: "Re-applies your tweakcc customizations whenever Claude Code updates, before your next launch (needs Node)",
		regs: []reg{{"SessionStart", "startup", 300, true}, {"Stop", "", 300, true}},
		warn: tweakccWarning,
		setup: func() error {
			if _, err := exec.LookPath("npx"); err != nil {
				return errors.New("needs Node.js (npx) on PATH")
			}
			recordClaude() // what is installed now is already current
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
	return "Opens tweakcc: edit system prompts, toolsets, subagent models and themes"
}

func (TweakccStudio) Warning() string { return tweakccWarning }

const tweakccWarning = "Uses tweakcc, which rewrites Claude Code's own program files. Needs Node; Claude Code updates undo it unless Keep tweakcc tweaks is on; run npx tweakcc --restore to undo by hand."

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

// AfterLaunch records the binary tweakcc just patched, so the re-apply
// patch does not mistake that change for a Claude Code update.
func (TweakccStudio) AfterLaunch() { recordClaude() }

type tweakccState struct {
	Claude  string    `json:"claude"`
	ModTime time.Time `json:"mod_time"`
}

// claudeBinary returns the resolved path and modification time of the
// installed claude executable.
func claudeBinary() (string, time.Time, bool) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return "", time.Time{}, false
	}
	if r, err := filepath.EvalSymlinks(bin); err == nil {
		bin = r
	}
	info, err := os.Stat(bin)
	if err != nil {
		return "", time.Time{}, false
	}
	return bin, info.ModTime(), true
}

func tweakccStatePath() string { return filepath.Join(claude.StateDir(), "tweakcc.json") }

func recordClaude() {
	bin, mod, ok := claudeBinary()
	if !ok {
		return
	}
	b, _ := json.Marshal(tweakccState{Claude: bin, ModTime: mod})
	_ = os.MkdirAll(claude.StateDir(), 0o755)
	_ = os.WriteFile(tweakccStatePath(), b, 0o644)
}

func tweakccConfigExists() bool {
	if d := os.Getenv("TWEAKCC_CONFIG_DIR"); d != "" {
		return fileExists(filepath.Join(d, "config.json"))
	}
	home, _ := os.UserHomeDir()
	for _, d := range []string{filepath.Join(home, ".tweakcc"), filepath.Join(claude.Dir(), "tweakcc")} {
		if fileExists(filepath.Join(d, "config.json")) {
			return true
		}
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return fileExists(filepath.Join(x, "tweakcc", "config.json"))
	}
	return false
}

// tweakccApply runs `tweakcc --apply`; tests replace it.
var tweakccApply = func() error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	_, err := run(exec.CommandContext(ctx, "npx", "-y", "tweakcc@latest", "--apply", "--yes"), "")
	return err
}

// tweakccHook re-applies tweakcc when the Claude Code install changed since
// the last apply. A lock file keeps parallel sessions from patching the
// same binary at once; a lock older than 10 minutes is treated as stale.
func tweakccHook(_ *Input, now time.Time) any {
	if !tweakccConfigExists() {
		return nil // nothing saved in tweakcc, nothing to re-apply
	}
	bin, mod, ok := claudeBinary()
	if !ok {
		return nil
	}
	var st tweakccState
	if b, err := os.ReadFile(tweakccStatePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	if st.Claude == bin && st.ModTime.Equal(mod) {
		return nil
	}

	lock := filepath.Join(claude.StateDir(), "tweakcc.lock")
	_ = os.MkdirAll(claude.StateDir(), 0o755)
	if info, err := os.Stat(lock); err == nil && now.Sub(info.ModTime()) > 10*time.Minute {
		_ = os.Remove(lock)
	}
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil // another session is applying
	}
	f.Close()
	defer os.Remove(lock)

	if tweakccApply() == nil {
		recordClaude()
	}
	return nil
}

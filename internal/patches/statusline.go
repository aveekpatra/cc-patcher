package patches

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aveekpatra/cc-patcher/internal/claude"
	"github.com/aveekpatra/cc-patcher/internal/statusline"
)

// The status line items share Claude Code's single statusLine setting:
// each one adds or removes its segment from `cc-patcher statusline <segs>`.
// The status line is a built-in setting, so the TUI lists these under
// Config, not Patches.

// StatusLine returns the status line segment toggles.
func StatusLine() []*Patch {
	var out []*Patch
	for _, p := range []struct{ name, seg, desc string }{
		{"Usage meter", "usage", "Status line: context, 5h and 7d limits with reset countdowns, and session cost"},
		{"Mood face", "mood", "Status line: an ASCII face showing what Claude is doing (reading, editing, failing...)"},
		{"Session pet", "pet", "Status line: an ASCII creature picked per session that hatches and grows as you work"},
	} {
		seg := p.seg
		out = append(out, &Patch{
			name: p.name, desc: p.desc,
			enabled: func() bool { return slices.Contains(ourSegments(), seg) },
			enable:  func() error { return setSegment(seg, true) },
			disable: func() error { return setSegment(seg, false) },
		})
	}
	return out
}

const statusMarker = `" statusline `

func statusPrevPath() string { return filepath.Join(claude.StateDir(), "statusline-prev.json") }

// segmentsOf returns the segments in cmd and whether cmd is ours.
func segmentsOf(cmd string) ([]string, bool) {
	i := strings.LastIndex(cmd, statusMarker)
	if i < 0 || !strings.HasPrefix(cmd, `"`) {
		return nil, false
	}
	var segs []string
	for _, s := range strings.Split(cmd[i+len(statusMarker):], ",") {
		if s = strings.TrimSpace(s); s != "" {
			segs = append(segs, s)
		}
	}
	return segs, true
}

func statusCommand(s claude.Settings) string {
	sl, _ := s["statusLine"].(map[string]any)
	cmd, _ := sl["command"].(string)
	return cmd
}

func ourSegments() []string {
	s, err := claude.LoadSettings()
	if err != nil {
		return nil
	}
	segs, _ := segmentsOf(statusCommand(s))
	return segs
}

// setSegment turns one segment on or off. The first segment saves the
// user's own statusLine; removing the last one puts it back.
func setSegment(seg string, on bool) error {
	var ferr error
	err := claude.Update(func(s claude.Settings) {
		cur, ours := segmentsOf(statusCommand(s))
		if !ours {
			cur = nil
			ferr = savePrevStatus(s["statusLine"])
			if ferr != nil {
				return
			}
		}
		var segs []string
		for _, name := range statusline.Segments { // keep the fixed order
			if (name == seg && on) || (name != seg && slices.Contains(cur, name)) {
				segs = append(segs, name)
			}
		}
		if len(segs) == 0 {
			ferr = restorePrevStatus(s)
			return
		}
		sl := map[string]any{
			"type":    "command",
			"command": `"` + exePath() + statusMarker + strings.Join(segs, ","),
		}
		if slices.Contains(segs, "usage") {
			sl["refreshInterval"] = 30 // keep reset countdowns moving
		}
		s["statusLine"] = sl
	})
	if err != nil {
		return err
	}
	return ferr
}

// savePrevStatus records the statusLine we are about to replace, or clears
// a stale record when there is none.
func savePrevStatus(prev any) error {
	if prev == nil {
		if err := os.Remove(statusPrevPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(prev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(statusPrevPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(statusPrevPath(), b, 0o644)
}

func restorePrevStatus(s claude.Settings) error {
	delete(s, "statusLine")
	b, err := os.ReadFile(statusPrevPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var prev any
	if json.Unmarshal(b, &prev) == nil && prev != nil {
		s["statusLine"] = prev
	}
	return os.Remove(statusPrevPath())
}

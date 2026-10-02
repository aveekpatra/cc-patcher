package patches

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func init() {
	extraPatches = append(extraPatches,
		&Patch{
			name: "Sound alerts", hookArg: "sound",
			desc: "Plays a system sound when Claude finishes, needs you, or a tool fails",
			regs: []reg{{"Stop", "", 10, true}, {"Notification", "", 10, true}, {"PostToolUseFailure", "*", 10, true}},
		},
		&Patch{
			name: "tmux status", hookArg: "tmux",
			desc: "Marks the tmux window [*] while Claude works, [?] when it waits, [ok] when done",
			regs: []reg{
				{"UserPromptSubmit", "", 5, true}, {"PreToolUse", "*", 5, true},
				{"Notification", "", 5, true}, {"Stop", "", 5, true}, {"SessionEnd", "", 5, true},
			},
		},
	)
	handlers["sound"] = soundHook
	handlers["tmux"] = tmuxHook
}

// soundHook plays a built-in OS sound for the event.
func soundHook(in *Input, _ time.Time) any {
	kind := "done"
	switch in.HookEventName {
	case "Notification":
		kind = "attention"
	case "PostToolUseFailure":
		kind = "error"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		name := map[string]string{"done": "Glass", "attention": "Ping", "error": "Basso"}[kind]
		_ = exec.CommandContext(ctx, "afplay", "/System/Library/Sounds/"+name+".aiff").Run()
	case "windows":
		name := map[string]string{"done": "Asterisk", "attention": "Exclamation", "error": "Hand"}[kind]
		_ = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command",
			"[System.Media.SystemSounds]::"+name+".Play(); Start-Sleep -Milliseconds 800").Run()
	default:
		name := map[string]string{"done": "complete", "attention": "bell", "error": "dialog-warning"}[kind]
		for _, dir := range []string{"/usr/share/sounds/freedesktop/stereo"} {
			for _, ext := range []string{".oga", ".ogg", ".wav"} {
				f := filepath.Join(dir, name+ext)
				if !fileExists(f) {
					continue
				}
				for _, player := range []string{"paplay", "pw-play", "aplay"} {
					if _, err := exec.LookPath(player); err == nil {
						_ = exec.CommandContext(ctx, player, f).Run()
						return nil
					}
				}
			}
		}
		_, _ = os.Stderr.WriteString("\a")
	}
	return nil
}

var tmuxMarks = []string{"[*] ", "[?] ", "[ok] "}

// tmuxHook prefixes the current tmux window name with Claude's state.
func tmuxHook(in *Input, _ time.Time) any {
	pane := os.Getenv("TMUX_PANE")
	if os.Getenv("TMUX") == "" || pane == "" {
		return nil
	}
	mark := "[*] "
	switch in.HookEventName {
	case "Notification":
		mark = "[?] "
	case "Stop":
		mark = "[ok] "
	case "SessionEnd":
		mark = ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", pane, "#W").Output()
	if err != nil {
		return nil
	}
	name := strings.TrimSpace(string(out))
	for _, m := range tmuxMarks {
		name = strings.TrimPrefix(name, m)
	}
	if mark+name != strings.TrimSpace(string(out)) {
		_ = exec.CommandContext(ctx, "tmux", "rename-window", "-t", pane, mark+name).Run()
	}
	return nil
}

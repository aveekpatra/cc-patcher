package patches

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

type hookInput struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
}

type sessionTimes struct {
	Start     time.Time `json:"start"`
	TurnStart time.Time `json:"turn_start"`
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// RunTimestampHook is the body of `claude_patcher hook timestamp`. It never
// fails loudly: a broken hook must not block the agent.
func RunTimestampHook(stdin io.Reader, stdout io.Writer, now time.Time) {
	var in hookInput
	_ = json.NewDecoder(stdin).Decode(&in)

	dir := filepath.Join(claude.StateDir(), "sessions")
	_ = os.MkdirAll(dir, 0o755)
	id := unsafeID.ReplaceAllString(in.SessionID, "")
	if id == "" {
		id = "unknown"
	}
	path := filepath.Join(dir, id+".json")

	var st sessionTimes
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st)
	} else {
		pruneSessions(dir, now)
	}
	if st.Start.IsZero() {
		st.Start = now
	}
	if in.HookEventName == "UserPromptSubmit" || st.TurnStart.IsZero() {
		st.TurnStart = now
	}
	if b, err := json.Marshal(st); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}

	var msg string
	stamp := now.Format("2006-01-02 15:04:05 MST (Mon)")
	if in.HookEventName == "UserPromptSubmit" {
		msg = fmt.Sprintf("[clock] User message received at %s. Session running %s.",
			stamp, human(now.Sub(st.Start)))
	} else {
		msg = fmt.Sprintf("[clock] Now %s. Working on the current request for %s; session running %s.",
			stamp, human(now.Sub(st.TurnStart)), human(now.Sub(st.Start)))
	}

	event := in.HookEventName
	if event == "" {
		event = "PostToolUse"
	}
	_ = json.NewEncoder(stdout).Encode(map[string]any{
		"hookSpecificOutput": map[string]any{
			"hookEventName":     event,
			"additionalContext": msg,
		},
	})
}

// human formats a duration like "1h05m", "12m30s" or "8s".
func human(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// pruneSessions deletes session files untouched for a week.
func pruneSessions(dir string, now time.Time) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 7*24*time.Hour {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

package patches

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

type sessionTimes struct {
	Start     time.Time `json:"start"`
	TurnStart time.Time `json:"turn_start"`
}

// timestampHook tells Claude the time and how long the session and the
// current request have been running.
func timestampHook(in *Input, now time.Time) any {
	var st sessionTimes
	if !loadState("sessions", in.SessionID, &st) {
		pruneOld(filepath.Join(claude.StateDir(), "sessions"), now)
	}
	if st.Start.IsZero() {
		st.Start = now
	}
	if in.HookEventName == "UserPromptSubmit" || st.TurnStart.IsZero() {
		st.TurnStart = now
	}
	saveState("sessions", in.SessionID, st)

	stamp := now.Format("2006-01-02 15:04:05 MST (Mon)")
	if in.HookEventName == "UserPromptSubmit" {
		return addContext("UserPromptSubmit", fmt.Sprintf(
			"[clock] User message received at %s. Session running %s.",
			stamp, human(now.Sub(st.Start))))
	}
	event := in.HookEventName
	if event == "" {
		event = "PostToolUse"
	}
	return addContext(event, fmt.Sprintf(
		"[clock] Now %s. Working on the current request for %s; session running %s.",
		stamp, human(now.Sub(st.TurnStart)), human(now.Sub(st.Start))))
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

package patches

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

type sessionTimes struct {
	Start     time.Time            `json:"start"`
	TurnStart time.Time            `json:"turn_start"`
	Agents    map[string]time.Time `json:"agents,omitempty"` // subagent start times
}

// clockNote is added to CLAUDE.md so every agent, subagents included, knows
// what the [clock] lines are for.
const clockNote = `# Time awareness

Lines starting with [clock] come from a hook and give the real current time, how long the current request has run, and how long the session or subagent has run. Use them:

- Treat durations the user gives ("two hours", "by 5pm", "20 minutes max") as real wall-clock budgets and check them against [clock].
- When a step has taken far longer than expected, stop and reassess instead of pushing on.
- When you start subagents, pass them any time budget and tell them to report back when they exceed it.`

// timestampHook tells Claude the time and how long the session, the current
// request and (inside a subagent) the subagent have been running.
func timestampHook(in *Input, now time.Time) any {
	var st sessionTimes
	if !loadState("sessions", in.SessionID, &st) {
		pruneOld(filepath.Join(claude.StateDir(), "sessions"), now)
	}
	if st.Start.IsZero() {
		st.Start = now
	}
	if st.Agents == nil {
		st.Agents = map[string]time.Time{}
	}
	stamp := now.Format("2006-01-02 15:04:05 MST (Mon)")
	var msg string
	event := in.HookEventName

	switch {
	case event == "SessionStart":
		st.TurnStart = now
		msg = fmt.Sprintf("[clock] Session started (%s) at %s. Session running %s.",
			in.Source, stamp, human(now.Sub(st.Start)))
	case event == "SubagentStart":
		st.Agents[in.AgentID] = now
		msg = fmt.Sprintf("[clock] Subagent started at %s. The main session has been running %s.",
			stamp, human(now.Sub(st.Start)))
	case event == "UserPromptSubmit":
		st.TurnStart = now
		msg = fmt.Sprintf("[clock] User message received at %s. Session running %s.",
			stamp, human(now.Sub(st.Start)))
	case in.AgentID != "":
		start, ok := st.Agents[in.AgentID]
		if !ok {
			start = now
			st.Agents[in.AgentID] = now
		}
		msg = fmt.Sprintf("[clock] Now %s. This subagent has been running %s; main session running %s.",
			stamp, human(now.Sub(start)), human(now.Sub(st.Start)))
	default:
		if st.TurnStart.IsZero() {
			st.TurnStart = now
		}
		msg = fmt.Sprintf("[clock] Now %s. Working on the current request for %s; session running %s.",
			stamp, human(now.Sub(st.TurnStart)), human(now.Sub(st.Start)))
	}
	saveState("sessions", in.SessionID, st)
	if event == "" {
		event = "PostToolUse"
	}
	return addContext(event, msg)
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

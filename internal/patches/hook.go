package patches

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// Input is the JSON Claude Code sends a hook on stdin. Only the fields our
// patches read are listed; see https://code.claude.com/docs/en/hooks.
type Input struct {
	SessionID            string          `json:"session_id"`
	TranscriptPath       string          `json:"transcript_path"`
	Cwd                  string          `json:"cwd"`
	HookEventName        string          `json:"hook_event_name"`
	AgentID              string          `json:"agent_id"`
	AgentType            string          `json:"agent_type"`
	ToolName             string          `json:"tool_name"`
	ToolInput            map[string]any  `json:"tool_input"`
	ToolResponse         json.RawMessage `json:"tool_response"`
	Source               string          `json:"source"`
	Trigger              string          `json:"trigger"`
	Message              string          `json:"message"`
	NotificationType     string          `json:"notification_type"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	StopHookActive       bool            `json:"stop_hook_active"`
}

func (in *Input) str(key string) string {
	s, _ := in.ToolInput[key].(string)
	return s
}

// handler returns the JSON to print, or nil for no output.
type handler func(in *Input, now time.Time) any

var handlers = map[string]handler{
	"timestamp":  timestampHook,
	"guard":      guardHook,
	"verify":     verifyHook,
	"stuck":      stuckHook,
	"context":    contextHook,
	"check":      checkHook,
	"checkpoint": checkpointHook,
	"compact":    compactHook,
	"injection":  injectionHook,
	"alerts":     alertsHook,
}

// RunHook is the body of `cc-patcher hook <name>`. It never fails
// loudly: a broken hook must not block the agent.
func RunHook(name string, stdin io.Reader, stdout io.Writer) {
	h, ok := handlers[name]
	if !ok {
		return
	}
	defer func() { _ = recover() }()
	var in Input
	_ = json.NewDecoder(stdin).Decode(&in)
	if in.Cwd == "" {
		in.Cwd, _ = os.Getwd()
	}
	if out := h(&in, time.Now()); out != nil {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(out)
	}
}

// addContext gives Claude extra text alongside the event.
func addContext(event, msg string) any {
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     event,
		"additionalContext": msg,
	}}
}

// toolDecision answers a PreToolUse event with allow, deny or ask.
func toolDecision(decision, reason string) any {
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":            "PreToolUse",
		"permissionDecision":       decision,
		"permissionDecisionReason": reason,
	}}
}

// block stops a Stop event from ending the turn and tells Claude why.
func block(reason string) any {
	return map[string]any{"decision": "block", "reason": reason}
}

// tellUser shows a message to the user without involving Claude.
func tellUser(msg string) any { return map[string]any{"systemMessage": msg} }

// Per-session state lives in StateDir/<patch>/<session>.json.

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func statePath(patch, session string) string {
	id := unsafeID.ReplaceAllString(session, "")
	if id == "" {
		id = "unknown"
	}
	return filepath.Join(claude.StateDir(), patch, id+".json")
}

// loadState reads state into v and reports whether it existed.
func loadState(patch, session string, v any) bool {
	b, err := os.ReadFile(statePath(patch, session))
	if err != nil {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func saveState(patch, session string, v any) {
	p := statePath(patch, session)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.Marshal(v); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// pruneOld deletes files in dir untouched for a week.
func pruneOld(dir string, now time.Time) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > 7*24*time.Hour {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

// shell runs a command line through the platform shell in dir.
func shell(ctx context.Context, dir, line string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", line)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", line)
	}
	return run(cmd, dir)
}

func run(cmd *exec.Cmd, dir string) (string, error) {
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CI=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// tail keeps the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return "...\n" + strings.Join(lines[len(lines)-n:], "\n")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

// projectConfig is the optional <project>/.claude/cc-patcher.json.
type projectConfig struct {
	Verify []string `json:"verify"`
}

func loadProjectConfig(cwd string) projectConfig {
	var c projectConfig
	if b, err := os.ReadFile(filepath.Join(cwd, ".claude", "cc-patcher.json")); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

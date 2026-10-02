package patches

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

// alertsConfig lives in StateDir/alerts.json and is created on enable.
type alertsConfig struct {
	Server string `json:"server"`
	Topic  string `json:"topic"`
	// ApproveWaitSeconds is how long a permission prompt waits for a phone
	// answer before falling back to the terminal. 0 disables remote approval.
	ApproveWaitSeconds int  `json:"approve_wait_seconds"`
	NotifyOnStop       bool `json:"notify_on_stop"`
}

func alertsPath() string { return filepath.Join(claude.StateDir(), "alerts.json") }

func loadAlerts() (alertsConfig, bool) {
	var c alertsConfig
	b, err := os.ReadFile(alertsPath())
	if err != nil || json.Unmarshal(b, &c) != nil || c.Topic == "" {
		return c, false
	}
	if c.Server == "" {
		c.Server = "https://ntfy.sh"
	}
	c.Server = strings.TrimRight(c.Server, "/")
	return c, true
}

// setupAlerts creates alerts.json with a random topic if it is missing.
func setupAlerts() error {
	if _, ok := loadAlerts(); ok {
		return nil
	}
	c := alertsConfig{Server: "https://ntfy.sh", Topic: "claude-" + randHex(12), ApproveWaitSeconds: 45, NotifyOnStop: true}
	if err := os.MkdirAll(claude.StateDir(), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(alertsPath(), b, 0o600)
}

func alertsNote() string {
	c, ok := loadAlerts()
	if !ok {
		return ""
	}
	return fmt.Sprintf("Install the ntfy app and subscribe to topic %q (server %s). Settings: %s", c.Topic, c.Server, alertsPath())
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// alertsHook pushes "done" and "needs input" alerts to the phone, and lets
// the user approve or deny permission prompts from it.
func alertsHook(in *Input, now time.Time) any {
	c, ok := loadAlerts()
	if !ok {
		return nil
	}
	project := filepath.Base(in.Cwd)
	switch in.HookEventName {
	case "Stop":
		if c.NotifyOnStop {
			msg := clip(strings.TrimSpace(in.LastAssistantMessage), 300)
			if msg == "" {
				msg = "Finished."
			}
			_ = push(c, c.Topic, "Claude done: "+project, msg, "white_check_mark", "")
		}
	case "Notification":
		if in.NotificationType != "permission_prompt" { // PermissionRequest covers these
			_ = push(c, c.Topic, "Claude: "+project, in.Message, "bell", "")
		}
	case "PermissionRequest":
		return remoteApprove(c, in, project, now)
	}
	return nil
}

func remoteApprove(c alertsConfig, in *Input, project string, now time.Time) any {
	if c.ApproveWaitSeconds <= 0 {
		return nil
	}
	nonce := randHex(6)
	reply := c.Topic + "-reply"
	what := in.str("command")
	if what == "" {
		what = in.str("file_path")
	}
	if what == "" {
		b, _ := json.Marshal(in.ToolInput)
		what = string(b)
	}
	actions := fmt.Sprintf("http, Allow, %[1]s/%[2]s, body=allow %[3]s, clear=true; http, Deny, %[1]s/%[2]s, body=deny %[3]s, clear=true",
		c.Server, reply, nonce)
	if push(c, c.Topic, "Claude wants "+in.ToolName+": "+project, clip(what, 300), "lock", actions) != nil {
		return nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	deadline := now.Add(time.Duration(c.ApproveWaitSeconds) * time.Second)
	since := now.Unix() - 1
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		resp, err := client.Get(fmt.Sprintf("%s/%s/json?poll=1&since=%d", c.Server, reply, since))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			var m struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			switch strings.TrimSpace(m.Message) {
			case "allow " + nonce:
				resp.Body.Close()
				return permissionDecision("allow", "Approved from phone")
			case "deny " + nonce:
				resp.Body.Close()
				return permissionDecision("deny", "The user denied this from their phone")
			}
		}
		resp.Body.Close()
	}
	return nil // no answer: fall back to the terminal prompt
}

func permissionDecision(behavior, msg string) any {
	return map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "PermissionRequest",
		"decision":      map[string]any{"behavior": behavior, "message": msg},
	}}
}

func push(c alertsConfig, topic, title, msg, tags, actions string) error {
	req, err := http.NewRequest("POST", c.Server+"/"+topic, strings.NewReader(msg))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Tags", tags)
	if actions != "" {
		req.Header.Set("Actions", actions)
		req.Header.Set("Priority", "high")
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: %s", resp.Status)
	}
	return nil
}

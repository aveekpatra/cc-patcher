package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// preset sets several settings.json keys and env vars at once. Values it
// replaces are saved and put back when the preset is turned off.
func preset(id, name, desc string, settings map[string]any, env map[string]string) *Option {
	var lines []string
	for _, k := range sortedKeys(settings) {
		v, _ := json.Marshal(settings[k])
		lines = append(lines, "settings.json "+k+" = "+clipJSON(string(v)))
	}
	envKeys := make([]string, 0, len(env))
	for k := range env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		lines = append(lines, "env "+k+"="+env[k])
	}
	return &Option{
		name:    name,
		desc:    desc,
		details: strings.Join(lines, "\n"),
		enabled: func() bool {
			s, err := claude.LoadSettings()
			if err != nil {
				return false
			}
			for k, v := range settings {
				if !sameJSON(s[k], v) {
					return false
				}
			}
			e, _ := s["env"].(map[string]any)
			for k, v := range env {
				if e[k] != v {
					return false
				}
			}
			return true
		},
		enable: func() error {
			return claude.Update(func(s claude.Settings) {
				prev := savedValues{Settings: map[string]any{}, Env: map[string]any{}}
				for k, v := range settings {
					if old, ok := s[k]; ok && !sameJSON(old, v) {
						prev.Settings[k] = old
					}
					s[k] = v
				}
				e, _ := s["env"].(map[string]any)
				if e == nil {
					e = map[string]any{}
				}
				for k, v := range env {
					if old, ok := e[k]; ok && old != v {
						prev.Env[k] = old
					}
					e[k] = v
				}
				if len(e) > 0 {
					s["env"] = e
				}
				savePrev(id, prev)
			})
		},
		disable: func() error {
			prev := loadPrev(id)
			return claude.Update(func(s claude.Settings) {
				for k := range settings {
					if old, ok := prev.Settings[k]; ok {
						s[k] = old
					} else {
						delete(s, k)
					}
				}
				e, _ := s["env"].(map[string]any)
				for k := range env {
					if old, ok := prev.Env[k]; ok {
						e[k] = old
					} else {
						delete(e, k)
					}
				}
				if len(e) == 0 {
					delete(s, "env")
				}
				savePrev(id, savedValues{})
			})
		},
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func clipJSON(s string) string {
	if len(s) > 160 {
		return s[:157] + "..."
	}
	return s
}

type savedValues struct {
	Settings map[string]any `json:"settings,omitempty"`
	Env      map[string]any `json:"env,omitempty"`
}

func prevPath(id string) string {
	return filepath.Join(claude.StateDir(), "config-prev", id+".json")
}

func savePrev(id string, v savedValues) {
	p := prevPath(id)
	if len(v.Settings) == 0 && len(v.Env) == 0 {
		_ = os.Remove(p)
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(p, b, 0o644)
}

func loadPrev(id string) savedValues {
	var v savedValues
	if b, err := os.ReadFile(prevPath(id)); err == nil {
		_ = json.Unmarshal(b, &v)
	}
	return v
}

// requestFile holds what the request injector adds to every API call.
func requestFile() string { return filepath.Join(claude.StateDir(), "request.json") }

type requestSpec struct {
	ExtraBody map[string]any    `json:"extra_body"`
	Betas     []string          `json:"betas"`
	Headers   map[string]string `json:"headers"`
}

// requestInjector maps request.json onto CLAUDE_CODE_EXTRA_BODY,
// ANTHROPIC_BETAS and ANTHROPIC_CUSTOM_HEADERS.
func requestInjector() *Option {
	keys := []string{"CLAUDE_CODE_EXTRA_BODY", "ANTHROPIC_BETAS", "ANTHROPIC_CUSTOM_HEADERS"}
	return &Option{
		name: "Request injector",
		desc: "Adds JSON fields, beta flags and headers from ~/.claude/cc-patcher/request.json to every API request",
		enabled: func() bool {
			s, _ := claude.LoadSettings()
			e, _ := s["env"].(map[string]any)
			for _, k := range keys {
				if _, ok := e[k]; ok {
					return true
				}
			}
			return false
		},
		enable: func() error {
			var spec requestSpec
			b, err := os.ReadFile(requestFile())
			if errors.Is(err, os.ErrNotExist) {
				_ = os.MkdirAll(claude.StateDir(), 0o755)
				tmpl, _ := json.MarshalIndent(requestSpec{ExtraBody: map[string]any{}, Betas: []string{}, Headers: map[string]string{}}, "", "  ")
				_ = os.WriteFile(requestFile(), tmpl, 0o644)
				return fmt.Errorf("created %s; fill it in, then enable again", requestFile())
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, &spec); err != nil {
				return fmt.Errorf("%s: %v", requestFile(), err)
			}
			env := map[string]string{}
			if len(spec.ExtraBody) > 0 {
				j, _ := json.Marshal(spec.ExtraBody)
				env["CLAUDE_CODE_EXTRA_BODY"] = string(j)
			}
			if len(spec.Betas) > 0 {
				env["ANTHROPIC_BETAS"] = strings.Join(spec.Betas, ",")
			}
			if len(spec.Headers) > 0 {
				var lines []string
				for k, v := range spec.Headers {
					lines = append(lines, k+": "+v)
				}
				sort.Strings(lines)
				env["ANTHROPIC_CUSTOM_HEADERS"] = strings.Join(lines, "\n")
			}
			if len(env) == 0 {
				return fmt.Errorf("%s is empty; add extra_body, betas or headers first", requestFile())
			}
			return claude.Update(func(s claude.Settings) {
				e, _ := s["env"].(map[string]any)
				if e == nil {
					e = map[string]any{}
				}
				for _, k := range keys {
					delete(e, k)
				}
				for k, v := range env {
					e[k] = v
				}
				s["env"] = e
			})
		},
		disable: func() error {
			return claude.Update(func(s claude.Settings) {
				e, _ := s["env"].(map[string]any)
				for _, k := range keys {
					delete(e, k)
				}
				if len(e) == 0 {
					delete(s, "env")
				}
			})
		},
	}
}

// skipPermissions starts sessions in bypassPermissions mode. It edits only
// permissions.defaultMode, keeping the rest of the permissions object, and
// puts back whatever mode was set before.
func skipPermissions() *Option {
	prevMode := filepath.Join(claude.StateDir(), "config-prev", "skip-permissions.json")
	mode := func(s claude.Settings) any {
		p, _ := s["permissions"].(map[string]any)
		return p["defaultMode"]
	}
	return &Option{
		name:    "Skip permission prompts",
		desc:    "Starts every local session in bypassPermissions (--dangerously-skip-permissions) without the confirmation. Deny rules and the command guard still apply; cloud sessions ignore it",
		details: "settings.json permissions.defaultMode = \"bypassPermissions\"\nsettings.json skipDangerousModePermissionPrompt = true",
		enabled: func() bool {
			s, err := claude.LoadSettings()
			return err == nil && mode(s) == "bypassPermissions" && s["skipDangerousModePermissionPrompt"] == true
		},
		enable: func() error {
			return claude.Update(func(s claude.Settings) {
				if old := mode(s); old != nil && old != "bypassPermissions" {
					b, _ := json.Marshal(old)
					_ = os.MkdirAll(filepath.Dir(prevMode), 0o755)
					_ = os.WriteFile(prevMode, b, 0o644)
				}
				p, _ := s["permissions"].(map[string]any)
				if p == nil {
					p = map[string]any{}
				}
				p["defaultMode"] = "bypassPermissions"
				s["permissions"] = p
				s["skipDangerousModePermissionPrompt"] = true
			})
		},
		disable: func() error {
			return claude.Update(func(s claude.Settings) {
				p, _ := s["permissions"].(map[string]any)
				var old any
				if b, err := os.ReadFile(prevMode); err == nil && json.Unmarshal(b, &old) == nil && p != nil {
					p["defaultMode"] = old
				} else if p != nil {
					delete(p, "defaultMode")
					if len(p) == 0 {
						delete(s, "permissions")
					}
				}
				delete(s, "skipDangerousModePermissionPrompt")
				_ = os.Remove(prevMode)
			})
		},
	}
}

func presets() []*Option {
	return []*Option{
		skipPermissions(),
		preset("unattended", "Unattended mode",
			"Retries rate limits forever, continues after usage resets, auto-answers questions after 10m",
			map[string]any{"autoContinueAtUsageLimit": true, "askUserQuestionTimeout": "10m"},
			map[string]string{"CLAUDE_CODE_RETRY_WATCHDOG": "1", "CLAUDE_CODE_RESUME_INTERRUPTED_TURN": "1"}),
		preset("lean", "Lean context",
			"Drops built-in git instructions, the Chrome prompt and the attribution block to save tokens",
			map[string]any{"includeGitInstructions": false},
			map[string]string{"CLAUDE_CODE_DISABLE_CFC_PROMPT": "1", "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"}),
		preset("compact-early", "Compact earlier",
			"Auto-compacts at 70% of the window instead of the default",
			nil, map[string]string{"CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "70"}),
		preset("cache-1h", "1-hour prompt cache",
			"Keeps the main conversation cached for an hour (cache writes cost more)",
			map[string]any{"promptCacheTtl": "1h"}, nil),
		preset("big-output", "Bigger tool output",
			"100k chars of shell output, 10m default / 60m max command timeout, 50k-token file reads",
			map[string]any{"bashOutputMaxChars": 100000},
			map[string]string{"BASH_DEFAULT_TIMEOUT_MS": "600000", "BASH_MAX_TIMEOUT_MS": "3600000", "CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS": "50000"}),
		preset("fun-ui", "Fun UI pack",
			"Extra spinner verbs, cc-patcher tips, a startup message and turn durations",
			map[string]any{
				"spinnerVerbs": map[string]any{"mode": "append", "verbs": []any{"Patching", "Tinkering", "Rewiring", "Soldering", "Splicing", "Tuning"}},
				"spinnerTipsOverride": map[string]any{"tips": []any{
					"Run cc-patcher to turn patches, config presets and skills on or off",
					"Lines starting with [clock] tell Claude the real time and how long it has been working",
				}},
				"companyAnnouncements": []any{"cc-patcher is active. Run cc-patcher to change patches."},
				"showTurnDuration":     true,
			}, nil),
		preset("no-attribution", "No AI attribution",
			"No co-author trailer on commits and no Claude line or session link in PRs",
			map[string]any{"attribution": map[string]any{"commit": "", "pr": "", "sessionUrl": false}}, nil),
		preset("privacy", "Privacy mode",
			"No telemetry, error reports or auto-updates; MCP servers get a clean env. Also turns off Remote Control",
			nil, map[string]string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "CLAUDE_CODE_MCP_ALLOWLIST_ENV": "1"}),
		preset("sandbox", "Sandbox with safe defaults",
			"Sandboxes shell commands, hides ~/.ssh, cloud and registry credentials, pre-allows code hosts and package registries",
			map[string]any{"sandbox": map[string]any{
				"enabled": true,
				"filesystem": map[string]any{"denyRead": []any{
					"~/.ssh", "~/.aws/credentials", "~/.config/gcloud", "~/.azure", "~/.gnupg",
					"~/.netrc", "~/.docker/config.json", "~/.npmrc", "~/.pypirc", "~/.kube/config",
				}},
				"network": map[string]any{"allowedDomains": []any{
					"github.com", "*.github.com", "*.githubusercontent.com", "gitlab.com",
					"registry.npmjs.org", "*.npmjs.org", "proxy.golang.org", "sum.golang.org",
					"pypi.org", "files.pythonhosted.org", "crates.io", "static.crates.io",
				}},
			}}, nil),
		preset("fuzzy-files", "Fuzzy @-file picker",
			"Fuzzy-matches @ file suggestions over git files, so @btn finds src/components/Button.tsx",
			map[string]any{"fileSuggestion": map[string]any{"type": "command", "command": claude.SelfCommand("files")}}, nil),
		requestInjector(),
		preset("agent-teams", "Agent teams (experimental)",
			"Turns on agent teams: several Claude sessions that work together",
			nil, map[string]string{"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"}),
		preset("fork-everywhere", "Forked subagents in -p",
			"Lets Claude fork subagents in headless and SDK sessions too",
			nil, map[string]string{"CLAUDE_CODE_FORK_SUBAGENT": "1"}),
		preset("flat-subagents", "No nested subagents",
			"Subagents cannot start their own subagents",
			nil, map[string]string{"CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH": "1"}),
	}
}

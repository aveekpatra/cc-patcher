// Package claude locates and edits the user's Claude Code configuration.
package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Dir returns the Claude Code config directory (~/.claude or $CLAUDE_CONFIG_DIR).
func Dir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// StateDir holds files owned by claude_patcher.
func StateDir() string { return filepath.Join(Dir(), "claude_patcher") }

func SettingsPath() string { return filepath.Join(Dir(), "settings.json") }

func ClaudeMdPath() string { return filepath.Join(Dir(), "CLAUDE.md") }

// Settings is settings.json as a generic map so unknown keys survive a round trip.
type Settings map[string]any

func LoadSettings() (Settings, error) {
	b, err := os.ReadFile(SettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return nil, err
	}
	s := Settings{}
	if len(bytes.TrimSpace(b)) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// SaveSettings writes settings.json, keeping a one-time backup of the original.
func SaveSettings(s Settings) error {
	path := SettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	backup := path + ".claude_patcher.bak"
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if orig, err := os.ReadFile(path); err == nil {
			_ = os.WriteFile(backup, orig, 0o644)
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Update loads settings, applies fn, and saves the result.
func Update(fn func(Settings)) error {
	s, err := LoadSettings()
	if err != nil {
		return err
	}
	fn(s)
	return SaveSettings(s)
}

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

func TestPresetRestoresPreviousValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"promptCacheTtl":"5m","env":{"KEEP":"1","BASH_MAX_TIMEOUT_MS":"5"}}`), 0o644)
	for _, o := range []*Option{
		preset("a", "a", "", map[string]any{"promptCacheTtl": "1h"}, nil),
		preset("b", "b", "", map[string]any{"bashOutputMaxChars": 100000}, map[string]string{"BASH_MAX_TIMEOUT_MS": "3600000"}),
	} {
		if err := o.Enable(); err != nil {
			t.Fatal(err)
		}
		if !o.Enabled() {
			t.Fatalf("%s not enabled", o.name)
		}
		if err := o.Disable(); err != nil {
			t.Fatal(err)
		}
	}
	s, _ := claude.LoadSettings()
	e := s["env"].(map[string]any)
	if s["promptCacheTtl"] != "5m" || e["BASH_MAX_TIMEOUT_MS"] != "5" || e["KEEP"] != "1" || s["bashOutputMaxChars"] != nil {
		t.Fatalf("not restored: %v", s)
	}
}

func TestSkipPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"permissions":{"allow":["x"],"defaultMode":"acceptEdits"}}`), 0o644)
	o := skipPermissions()
	if err := o.Enable(); err != nil || !o.Enabled() {
		t.Fatal("enable failed", err)
	}
	if err := o.Disable(); err != nil {
		t.Fatal(err)
	}
	s, _ := claude.LoadSettings()
	p := s["permissions"].(map[string]any)
	if p["defaultMode"] != "acceptEdits" || len(p["allow"].([]any)) != 1 || s["skipDangerousModePermissionPrompt"] != nil {
		t.Fatalf("not restored: %v", s)
	}
}

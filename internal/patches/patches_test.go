package patches

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

func TestEnableDisableKeepsOtherHooks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	orig := `{"theme":"dark","hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	p := All()[0]
	if p.Enabled() {
		t.Fatal("enabled before install")
	}
	if err := p.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := p.Enable(); err != nil { // idempotent
		t.Fatal(err)
	}
	if !p.Enabled() {
		t.Fatal("not enabled after install")
	}
	b, _ := os.ReadFile(claude.SettingsPath())
	if n := strings.Count(string(b), " hook timestamp"); n != 4 {
		t.Fatalf("want 4 hook entries, got %d:\n%s", n, b)
	}
	if !claude.HasBlock(claude.ClaudeMdPath(), "clock") {
		t.Fatal("CLAUDE.md clock note missing")
	}
	if err := p.Disable(); err != nil {
		t.Fatal(err)
	}
	s, _ := claude.LoadSettings()
	hooks := s["hooks"].(map[string]any)
	if len(hooks) != 1 || s["theme"] != "dark" || !strings.Contains(mustRead(t), "echo hi") {
		t.Fatalf("other settings lost: %v", s)
	}
	if claude.HasBlock(claude.ClaudeMdPath(), "clock") {
		t.Fatal("CLAUDE.md clock note left behind")
	}
}

func mustRead(t *testing.T) string {
	b, err := os.ReadFile(claude.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

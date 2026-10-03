package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactAndRestoreKeepsSecrets(t *testing.T) {
	src := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", src)
	os.WriteFile(filepath.Join(src, "settings.json"), []byte(`{"env":{"GITHUB_TOKEN":"ghp_real","EDITOR":"vim"},"theme":"dark"}`), 0o644)
	os.WriteFile(filepath.Join(src, ".claude.json"), []byte(`{"mcpServers":{"x":{"headers":{"Authorization":"Bearer abc"},"url":"https://x"}},"userID":"secret-user"}`), 0o600)
	os.MkdirAll(filepath.Join(src, "agents"), 0o755)
	os.WriteFile(filepath.Join(src, "agents", "rev.md"), []byte("agent"), 0o644)
	os.MkdirAll(filepath.Join(src, "projects", "p"), 0o755)
	os.WriteFile(filepath.Join(src, "projects", "p", "t.jsonl"), []byte("history"), 0o644)

	b, err := Take(false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b)
	if strings.Contains(string(raw), "ghp_real") || strings.Contains(string(raw), "Bearer abc") || strings.Contains(string(raw), "secret-user") {
		t.Fatal("secret leaked into backup")
	}
	if _, ok := b.Files["projects/p/t.jsonl"]; ok {
		t.Fatal("session history included")
	}
	if string(b.Files["agents/rev.md"]) != "agent" {
		t.Fatal("agents missing")
	}

	// Restore onto a machine that already has its own token: it is kept.
	dst := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dst)
	os.WriteFile(filepath.Join(dst, "settings.json"), []byte(`{"env":{"GITHUB_TOKEN":"ghp_mine"}}`), 0o644)
	if _, err := Restore(b); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, _ := os.ReadFile(filepath.Join(dst, "settings.json"))
	json.Unmarshal(data, &got)
	env := got["env"].(map[string]any)
	if env["GITHUB_TOKEN"] != "ghp_mine" || env["EDITOR"] != "vim" || got["theme"] != "dark" {
		t.Fatalf("restore wrong: %s", data)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "agents", "rev.md")); string(b) != "agent" {
		t.Fatal("agent not restored")
	}
}

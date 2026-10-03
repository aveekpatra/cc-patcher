package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSkills(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "claude"))
	sk := filepath.Join(dir, "claude", "skills")
	os.MkdirAll(filepath.Join(sk, "mine"), 0o755)
	os.WriteFile(filepath.Join(sk, "mine", "SKILL.md"), []byte("---\nname: mine\ndescription: My skill\n---\nbody"), 0o644)
	os.MkdirAll(filepath.Join(sk, "managed"), 0o755)
	os.WriteFile(filepath.Join(sk, "managed", "SKILL.md"), []byte("x"), 0o644)
	ext := filepath.Join(dir, "elsewhere", "linked")
	os.MkdirAll(ext, 0o755)
	os.WriteFile(filepath.Join(ext, "SKILL.md"), []byte("---\ndescription: Linked\n---"), 0o644)
	os.Symlink(ext, filepath.Join(sk, "linked"))

	list := LocalList(map[string]bool{"managed": true})
	if len(list) != 2 || list[0].Name() != "linked" || list[1].Description() != "My skill (your skill)" {
		t.Fatalf("list: %v", list)
	}
	linked := list[0]
	files, _ := linked.Files()
	if string(files["SKILL.md"]) != "---\ndescription: Linked\n---" {
		t.Fatalf("files: %v", files)
	}

	// Uninstall a linked skill: the link goes to the trash, its target stays.
	if err := linked.Disable(); err != nil || linked.Enabled() {
		t.Fatal("disable", err)
	}
	if _, err := os.Stat(filepath.Join(ext, "SKILL.md")); err != nil {
		t.Fatal("link target was touched")
	}
	if l := LocalList(nil); len(l) != 3 {
		t.Fatalf("trashed skill not listed: %d", len(l))
	}
	if err := linked.Enable(); err != nil || !linked.Enabled() {
		t.Fatal("restore", err)
	}

	// Install from an export, refusing unsafe paths and existing skills.
	if err := InstallFiles("copy", map[string][]byte{"SKILL.md": []byte("x"), "ref/a.md": []byte("a")}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(sk, "copy", "ref", "a.md")); string(b) != "a" {
		t.Fatal("nested file missing")
	}
	if InstallFiles("copy", map[string][]byte{"SKILL.md": nil}) == nil {
		t.Fatal("overwrote existing skill")
	}
	for _, bad := range []string{"../evil", "/abs", `a\b`} {
		if InstallFiles("bad", map[string][]byte{"SKILL.md": nil, bad: nil}) == nil {
			t.Fatalf("accepted unsafe path %q", bad)
		}
	}
}

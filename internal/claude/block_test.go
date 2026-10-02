package claude

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBlockRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CLAUDE.md")
	orig := "# Mine\n\nkeep me\n"
	os.WriteFile(p, []byte(orig), 0o644)
	if err := SetBlock(p, "x", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := SetBlock(p, "x", "hello again"); err != nil {
		t.Fatal(err)
	}
	if !HasBlock(p, "x") {
		t.Fatal("block missing")
	}
	if err := RemoveBlock(p, "x"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != orig {
		t.Fatalf("got %q want %q", b, orig)
	}
}

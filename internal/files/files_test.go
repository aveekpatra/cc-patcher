package files

import "testing"

func TestMatch(t *testing.T) {
	paths := []string{"src/components/Button.tsx", "src/lib/bitten.ts", "docs/button-guide.md", "README.md"}
	got := Match(paths, "btn", 3)
	if len(got) < 2 || (got[0] != "src/components/Button.tsx" && got[1] != "src/components/Button.tsx") {
		t.Fatalf("btn: %v", got)
	}
	if got := Match(paths, "readme", 5); len(got) != 1 || got[0] != "README.md" {
		t.Fatalf("readme: %v", got)
	}
	if got := Match(paths, "zzz", 5); len(got) != 0 {
		t.Fatalf("zzz: %v", got)
	}
}

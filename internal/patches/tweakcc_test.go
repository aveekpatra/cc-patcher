package patches

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTweakccReapply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(dir, "claude"))
	t.Setenv("TWEAKCC_CONFIG_DIR", filepath.Join(dir, "tweakcc"))
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	t.Setenv("PATH", bin)
	claudeBin := filepath.Join(bin, "claude")
	os.WriteFile(claudeBin, []byte("v1"), 0o755)

	var applies atomic.Int32
	old := tweakccApply
	tweakccApply = func() error { applies.Add(1); time.Sleep(50 * time.Millisecond); return nil }
	defer func() { tweakccApply = old }()

	recordClaude()
	tweakccHook(&Input{}, time.Now())
	if applies.Load() != 0 {
		t.Fatal("applied without a tweakcc config")
	}
	os.MkdirAll(filepath.Join(dir, "tweakcc"), 0o755)
	os.WriteFile(filepath.Join(dir, "tweakcc", "config.json"), []byte("{}"), 0o644)

	tweakccHook(&Input{}, time.Now())
	if applies.Load() != 0 {
		t.Fatal("applied with no update")
	}

	// Simulate a Claude Code update, then several sessions ending at once.
	os.WriteFile(claudeBin, []byte("v2"), 0o755)
	os.Chtimes(claudeBin, time.Now().Add(time.Minute), time.Now().Add(time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); tweakccHook(&Input{}, time.Now()) }()
	}
	wg.Wait()
	if n := applies.Load(); n != 1 {
		t.Fatalf("want exactly 1 apply after update, got %d", n)
	}
	tweakccHook(&Input{}, time.Now())
	if applies.Load() != 1 {
		t.Fatal("re-applied after recording")
	}
}

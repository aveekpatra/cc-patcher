package patches

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

func statusPatch(t *testing.T, name string) *Patch {
	t.Helper()
	for _, p := range StatusLine() {
		if p.Name() == name {
			return p
		}
	}
	t.Fatalf("no patch %q", name)
	return nil
}

func TestStatusLineSegments(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	orig := `{"theme":"dark","statusLine":{"type":"command","command":"~/bin/my-status.sh","padding":1}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	usage, mood, pet := statusPatch(t, "Usage meter"), statusPatch(t, "Mood face"), statusPatch(t, "Session pet")
	if usage.Enabled() || mood.Enabled() || pet.Enabled() {
		t.Fatal("enabled before install")
	}

	cmdAndRefresh := func() (string, any) {
		s, _ := claude.LoadSettings()
		sl, _ := s["statusLine"].(map[string]any)
		cmd, _ := sl["command"].(string)
		return cmd, sl["refreshInterval"]
	}

	steps := []struct {
		p       *Patch
		on      bool
		suffix  string
		refresh any
	}{
		{pet, true, `" statusline pet`, nil},
		{usage, true, `" statusline usage,pet`, float64(30)},
		{usage, true, `" statusline usage,pet`, float64(30)}, // idempotent
		{mood, true, `" statusline usage,mood,pet`, float64(30)},
		{usage, false, `" statusline mood,pet`, nil},
		{pet, false, `" statusline mood`, nil},
	}
	for i, st := range steps {
		var err error
		if st.on {
			err = st.p.Enable()
		} else {
			err = st.p.Disable()
		}
		if err != nil {
			t.Fatal(err)
		}
		cmd, refresh := cmdAndRefresh()
		if !strings.HasPrefix(cmd, `"`) || !strings.HasSuffix(cmd, st.suffix) || refresh != st.refresh {
			t.Fatalf("step %d: command %q refresh %v", i, cmd, refresh)
		}
		if st.p.Enabled() != st.on {
			t.Fatalf("step %d: %s Enabled() = %v", i, st.p.Name(), !st.on)
		}
	}
	if !fileExists(statusPrevPath()) {
		t.Fatal("previous statusLine not saved")
	}

	if err := mood.Disable(); err != nil {
		t.Fatal(err)
	}
	s, _ := claude.LoadSettings()
	sl, _ := s["statusLine"].(map[string]any)
	if sl["command"] != "~/bin/my-status.sh" || sl["padding"] != float64(1) || s["theme"] != "dark" {
		t.Fatalf("user statusLine not restored: %v", s)
	}
	if fileExists(statusPrevPath()) {
		t.Fatal("statusline-prev.json left behind")
	}
	if mood.Enabled() {
		t.Fatal("mood enabled with user statusLine")
	}

	// Without a previous statusLine, removing the last segment drops the key.
	if err := claude.Update(func(s claude.Settings) { delete(s, "statusLine") }); err != nil {
		t.Fatal(err)
	}
	if err := pet.Enable(); err != nil {
		t.Fatal(err)
	}
	if err := pet.Disable(); err != nil {
		t.Fatal(err)
	}
	s, _ = claude.LoadSettings()
	if _, ok := s["statusLine"]; ok {
		t.Fatalf("statusLine left behind: %v", s)
	}
}

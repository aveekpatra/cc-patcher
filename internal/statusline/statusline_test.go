package statusline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var now = time.Unix(1_800_000_000, 0)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func parse(t *testing.T, js string) *Input {
	t.Helper()
	var in Input
	if err := json.Unmarshal([]byte(js), &in); err != nil {
		t.Fatal(err)
	}
	return &in
}

func TestUsage(t *testing.T) {
	cases := []struct{ name, js, want string }{
		{"empty", `{}`, ""},
		{"nulls", `{"context_window":{"used_percentage":null},"rate_limits":null,"cost":{"total_cost_usd":null}}`, ""},
		{"context only", `{"context_window":{"used_percentage":42.4}}`, "ctx 42%"},
		{"full", `{"model":{"display_name":"Opus"},"cost":{"total_cost_usd":1.234},
			"context_window":{"used_percentage":42},
			"rate_limits":{"five_hour":{"used_percentage":23,"resets_at":1800007800},
			"seven_day":{"used_percentage":61,"resets_at":1800274000}}}`,
			"ctx 42% | 5h 23% (2h10m) | 7d 61% (3d04h) | $1.23"},
		{"reset passed", `{"rate_limits":{"five_hour":{"used_percentage":90,"resets_at":1700000000}}}`, "5h 90%"},
		{"no resets_at", `{"rate_limits":{"seven_day":{"used_percentage":5}}}`, "7d 5%"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ansi.ReplaceAllString(usage(parse(t, c.js), now), ""); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestUsageColors(t *testing.T) {
	for pct, want := range map[float64]string{10: green, 50: yellow, 79: yellow, 80: red} {
		if got := color(pct); got != want {
			t.Errorf("color(%v) = %q, want %q", pct, got, want)
		}
	}
}

// transcript writes JSONL lines to a temp file and returns its path.
func transcript(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func toolUse(name string) string {
	return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"x","name":"` + name + `","input":{}}]}}`
}

const (
	okResult  = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","content":"fine"}]}}`
	errResult = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"x","is_error":true,"content":"boom"}]}}`
	reply     = `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done."}]}}`
	prompt    = `{"type":"user","message":{"role":"user","content":"fix the bug"}}`
	sysLine   = `{"type":"system","content":"hook ran"}`
)

func TestMood(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		ctx   string
		want  string
	}{
		{"no transcript", nil, "", "idle"},
		{"read", []string{prompt, toolUse("Grep")}, "", "curious"},
		{"edit after result", []string{toolUse("Edit"), okResult, sysLine}, "", "focused"},
		{"bash", []string{toolUse("Bash")}, "", "busy"},
		{"agent", []string{toolUse("Agent")}, "", "delegating"},
		{"web", []string{toolUse("WebSearch")}, "", "browsing"},
		{"error", []string{toolUse("Bash"), errResult}, "", "distressed"},
		{"old error ignored", []string{toolUse("Bash"), errResult, toolUse("Read"), okResult}, "", "curious"},
		{"reply", []string{toolUse("Read"), okResult, reply}, "", "happy"},
		{"prompt", []string{reply, prompt}, "", "thinking"},
		{"tired", []string{reply}, `,"context_window":{"used_percentage":91}`, "tired"},
		{"error beats tired", []string{toolUse("Bash"), errResult}, `,"context_window":{"used_percentage":91}`, "distressed"},
		{"garbage", []string{"not json", "{"}, "", "idle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := ""
			if c.lines != nil {
				path = transcript(t, c.lines...)
			}
			js, _ := json.Marshal(path)
			m := moodOf(parse(t, `{"transcript_path":`+string(js)+c.ctx+`}`))
			if m.state != c.want || m.face != moods[c.want].face || m.face == "" {
				t.Fatalf("got %+v, want %s", m, c.want)
			}
		})
	}
}

func TestMoodReadsOnlyTail(t *testing.T) {
	// A huge old line followed by a recent tool call.
	big := `{"type":"assistant","message":{"content":[{"type":"text","text":"` + strings.Repeat("a", tailBytes) + `"}]}}`
	if got := transcriptMood(transcript(t, big, toolUse("Write"))); got != "focused" {
		t.Fatalf("got %s", got)
	}
}

func TestPet(t *testing.T) {
	cases := []struct {
		name, js string
		want     *regexp.Regexp
	}{
		{"egg no duration", `{"session_id":"abc"}`, regexp.MustCompile(`^\(\.\) \w+ hatches in 5m$`)},
		{"egg", `{"session_id":"abc","cost":{"total_duration_ms":150000}}`, regexp.MustCompile(`^\(\.\) \w+ hatches in 3m$`)},
		{"baby", `{"session_id":"abc","cost":{"total_duration_ms":600000}}`, regexp.MustCompile(`^\S+ \w+ is curious$`)},
		{"adult", `{"session_id":"abc","cost":{"total_duration_ms":3600000}}`, regexp.MustCompile(`^\S+ \w+ is curious$`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pet(parse(t, c.js), moods["curious"])
			if !c.want.MatchString(got) {
				t.Fatalf("got %q", got)
			}
		})
	}
	// Same session, same pet; baby and adult art differ.
	baby := pet(parse(t, cases[2].js), moods["idle"])
	if baby != pet(parse(t, cases[2].js), moods["idle"]) {
		t.Fatal("pet not deterministic")
	}
	if adult := pet(parse(t, cases[3].js), moods["idle"]); adult == baby {
		t.Fatalf("adult looks like baby: %q", adult)
	}
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		seen[strings.Fields(pet(parse(t, `{"session_id":"`+id+`","cost":{"total_duration_ms":3600000}}`), mood{}))[0]] = true
	}
	if len(seen) < 3 {
		t.Fatalf("few species across sessions: %v", seen)
	}
}

func TestRun(t *testing.T) {
	var out bytes.Buffer
	Run("pet,usage,bogus", strings.NewReader(`{"session_id":"s","cost":{"total_cost_usd":0.5}}`), &out)
	got := out.String()
	if !strings.HasPrefix(got, "$0.50 | (.) ") || !strings.HasSuffix(got, "\n") {
		t.Fatalf("got %q", got)
	}
	for _, bad := range []string{"", "not json", "[1,2]", `{"cost":{"total_cost_usd":"x"}}`} {
		out.Reset()
		Run("usage,mood,pet", strings.NewReader(bad), &out)
		if out.Len() != 0 {
			t.Errorf("input %q printed %q", bad, out.String())
		}
	}
}

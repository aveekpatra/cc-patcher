// Package statusline renders `cc-patcher statusline <segments>`, the
// command Claude Code runs to draw its status line.
package statusline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"
	"time"
)

// Segments lists every segment in display order.
var Segments = []string{"usage", "mood", "pet"}

// Input is the JSON Claude Code sends the status line command on stdin.
// Pointers mark fields that may be absent or null.
type Input struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cost           struct {
		TotalCostUSD    *float64 `json:"total_cost_usd"`
		TotalDurationMS *float64 `json:"total_duration_ms"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	RateLimits struct {
		FiveHour *limit `json:"five_hour"`
		SevenDay *limit `json:"seven_day"`
	} `json:"rate_limits"`
}

type limit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"` // unix seconds
}

// Run is the body of `cc-patcher statusline <seg1,seg2>`. Like hooks, it
// never fails loudly: bad input prints nothing.
func Run(arg string, stdin io.Reader, stdout io.Writer) {
	defer func() { _ = recover() }()
	var in Input
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return
	}
	if line := Render(&in, strings.Split(arg, ","), time.Now()); line != "" {
		fmt.Fprintln(stdout, line)
	}
}

// Render joins the requested segments in the fixed Segments order.
func Render(in *Input, want []string, now time.Time) string {
	on := map[string]bool{}
	for _, w := range want {
		on[strings.TrimSpace(w)] = true
	}
	var m mood
	if on["mood"] || on["pet"] {
		m = moodOf(in)
	}
	var parts []string
	for _, seg := range Segments {
		if !on[seg] {
			continue
		}
		var s string
		switch seg {
		case "usage":
			s = usage(in, now)
		case "mood":
			s = m.face
		case "pet":
			s = pet(in, m)
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " | ")
}

// Usage meter.

const (
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	reset  = "\x1b[0m"
)

func color(pct float64) string {
	switch {
	case pct >= 80:
		return red
	case pct >= 50:
		return yellow
	default:
		return green
	}
}

// usage shows context, 5h and 7d rate limits with reset countdowns, and cost.
func usage(in *Input, now time.Time) string {
	var parts []string
	if p := in.ContextWindow.UsedPercentage; p != nil {
		parts = append(parts, pctLabel("ctx", *p, ""))
	}
	for _, l := range []struct {
		label string
		lim   *limit
	}{{"5h", in.RateLimits.FiveHour}, {"7d", in.RateLimits.SevenDay}} {
		if l.lim == nil || l.lim.UsedPercentage == nil {
			continue
		}
		left := ""
		if r := l.lim.ResetsAt; r != nil {
			if d := time.Unix(int64(*r), 0).Sub(now); d > 0 {
				left = countdown(d)
			}
		}
		parts = append(parts, pctLabel(l.label, *l.lim.UsedPercentage, left))
	}
	if c := in.Cost.TotalCostUSD; c != nil {
		parts = append(parts, fmt.Sprintf("$%.2f", *c))
	}
	return strings.Join(parts, " | ")
}

func pctLabel(label string, pct float64, left string) string {
	s := fmt.Sprintf("%s %s%.0f%%%s", label, color(pct), pct, reset)
	if left != "" {
		s += " (" + left + ")"
	}
	return s
}

// countdown formats a duration like "3d04h", "2h10m" or "12m".
func countdown(d time.Duration) string {
	m := int(d.Minutes())
	switch {
	case m >= 24*60:
		return fmt.Sprintf("%dd%02dh", m/(24*60), m/60%24)
	case m >= 60:
		return fmt.Sprintf("%dh%02dm", m/60, m%60)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// Mood face.

type mood struct{ state, face string }

var moods = map[string]mood{}

func init() {
	for _, m := range []mood{
		{"idle", "(-_-)"}, {"happy", "(^_^)"}, {"thinking", "(._.)"},
		{"curious", "(o_o)"}, {"focused", "(>_>)"}, {"busy", "(*_*)"},
		{"delegating", "(^o^)/"}, {"browsing", "(@_@)"},
		{"distressed", "(>_<)"}, {"tired", "(x_x)"},
	} {
		moods[m.state] = m
	}
}

// toolMood maps a tool name to what Claude looks like while using it.
func toolMood(name string) string {
	switch name {
	case "Read", "Grep", "Glob", "LS":
		return "curious"
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return "focused"
	case "Bash":
		return "busy"
	case "Agent", "Task":
		return "delegating"
	case "WebFetch", "WebSearch":
		return "browsing"
	}
	return "busy"
}

// moodOf reads the transcript tail; a failed tool beats a full context,
// which beats whatever Claude is doing.
func moodOf(in *Input) mood {
	state := transcriptMood(in.TranscriptPath)
	if p := in.ContextWindow.UsedPercentage; p != nil && *p >= 80 && state != "distressed" {
		state = "tired"
	}
	return moods[state]
}

// tailBytes bounds how much of the transcript is read per refresh.
const tailBytes = 256 << 10

type entry struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

func transcriptMood(path string) string {
	lines := tailLines(path)
	sawResult := false
	for i := len(lines) - 1; i >= 0; i-- {
		var e entry
		if json.Unmarshal(lines[i], &e) != nil {
			continue
		}
		var blocks []block
		_ = json.Unmarshal(e.Message.Content, &blocks) // a plain string leaves it empty
		switch e.Type {
		case "user":
			isResult := false
			for _, b := range blocks {
				if b.Type == "tool_result" {
					isResult = true
					if b.IsError && !sawResult {
						return "distressed"
					}
				}
			}
			if !isResult {
				if sawResult {
					continue
				}
				return "thinking" // a fresh prompt
			}
			sawResult = true // the tool_use that caused it comes earlier
		case "assistant":
			if len(blocks) == 0 {
				continue
			}
			switch last := blocks[len(blocks)-1]; last.Type {
			case "tool_use":
				return toolMood(last.Name)
			case "text":
				return "happy"
			case "thinking", "redacted_thinking":
				return "thinking"
			}
		}
	}
	return "idle"
}

// tailLines returns the complete lines in the last tailBytes of path.
func tailLines(path string) [][]byte {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	off := info.Size() - tailBytes
	if off < 0 {
		off = 0
	}
	buf := make([]byte, info.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil
	}
	if off > 0 { // drop the partial first line
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	var lines [][]byte
	sc := bufio.NewScanner(bytes.NewReader(buf))
	sc.Buffer(make([]byte, 0, 64<<10), tailBytes)
	for sc.Scan() {
		if l := bytes.TrimSpace(sc.Bytes()); len(l) > 0 {
			lines = append(lines, append([]byte(nil), l...))
		}
	}
	return lines
}

// Session pet.

type species struct {
	kind        string
	names       []string
	baby, adult string
}

var zoo = []species{
	{"cat", []string{"Mochi", "Miso", "Tofu"}, "=^.^=", "~(=^.^=)"},
	{"dog", []string{"Biscuit", "Pixel", "Rex"}, "U.U", "U^.^U_/"},
	{"bunny", []string{"Clover", "Pip", "Hops"}, `(\_/)`, `(\(o.o)/)`},
	{"owl", []string{"Hoot", "Sage", "Echo"}, "{o,o}", "{(O,O)}"},
	{"fish", []string{"Bubbles", "Finn", "Nemo"}, "<><", "><(((o>"},
	{"crab", []string{"Pinch", "Sandy", "Clawd"}, "v(.)v", "V(o_o)V"},
	{"snake", []string{"Noodle", "Slinky", "Hiss"}, "~o", "~~~~:>"},
}

const (
	hatchAt = 5 * time.Minute
	grownAt = 30 * time.Minute
)

// pet picks a creature from the session id and grows it with session age.
func pet(in *Input, m mood) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(in.SessionID))
	n := h.Sum32()
	sp := zoo[n%uint32(len(zoo))]
	name := sp.names[(n/uint32(len(zoo)))%uint32(len(sp.names))]
	var age time.Duration
	if d := in.Cost.TotalDurationMS; d != nil && *d > 0 {
		age = time.Duration(*d) * time.Millisecond
	}
	if age < hatchAt {
		return fmt.Sprintf("(.) %s hatches in %s", name, countdown(hatchAt-age+time.Minute-1))
	}
	art := sp.baby
	if age >= grownAt {
		art = sp.adult
	}
	s := art + " " + name
	if m.state != "" && m.state != "idle" {
		s += " is " + m.state
	}
	return s
}

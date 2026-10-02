package patches

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

var budgetThresholds = []int{50, 70, 85, 95}

type budgetState struct {
	Announced int `json:"announced"` // highest threshold already reported
}

// contextHook tells Claude how full its context window is, once per
// threshold crossed.
func contextHook(in *Input, _ time.Time) any {
	used := contextTokens(in.TranscriptPath)
	if used == 0 {
		return nil
	}
	window := contextWindow(used)
	pct := used * 100 / window

	var st budgetState
	loadState("context", in.SessionID, &st)
	level := 0
	for _, t := range budgetThresholds {
		if pct >= t {
			level = t
		}
	}
	if level < st.Announced { // compaction freed space
		st.Announced = level
		saveState("context", in.SessionID, st)
	}
	if level == 0 || level <= st.Announced {
		return nil
	}
	st.Announced = level
	saveState("context", in.SessionID, st)

	advice := "Keep going, but avoid reading large files you do not need."
	switch {
	case level >= 85:
		advice = "Compaction is close. Finish the current step, write down state the next context will need, and avoid big reads."
	case level >= 70:
		advice = "Plan the rest of the work so it fits, and prefer targeted reads."
	}
	return addContext(in.HookEventName, fmt.Sprintf("[context] About %dk of %dk tokens used (%d%%). %s",
		used/1000, window/1000, pct, advice))
}

// contextWindow returns $CLAUDE_PATCHER_CONTEXT_WINDOW, or 1M once usage
// has passed 200k (only 1M models get there), else 200k.
func contextWindow(used int) int {
	if n, err := strconv.Atoi(os.Getenv("CLAUDE_PATCHER_CONTEXT_WINDOW")); err == nil && n > 0 {
		return n
	}
	if used > 200_000 {
		return 1_000_000
	}
	return 200_000
}

// contextTokens reads the last main-thread assistant usage from the
// transcript: input plus cache tokens is what the model saw.
func contextTokens(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	const chunk = 1 << 20
	if info, err := f.Stat(); err == nil && info.Size() > chunk {
		_, _ = f.Seek(info.Size()-chunk, io.SeekStart)
	}
	b, _ := io.ReadAll(f)
	lines := bytes.Split(b, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"usage"`)) {
			continue
		}
		var e struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Usage struct {
					Input       int `json:"input_tokens"`
					CacheCreate int `json:"cache_creation_input_tokens"`
					CacheRead   int `json:"cache_read_input_tokens"`
					Output      int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &e) != nil || e.Type != "assistant" || e.IsSidechain {
			continue
		}
		u := e.Message.Usage
		if n := u.Input + u.CacheCreate + u.CacheRead + u.Output; n > 0 {
			return n
		}
	}
	return 0
}

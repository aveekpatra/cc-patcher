package patches

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const stuckWindow = 12

type stuckState struct {
	Recent []string `json:"recent"`
}

// stuckHook warns Claude when it repeats an identical tool call. It never
// denies: polling the same command can be legitimate.
func stuckHook(in *Input, _ time.Time) any {
	b, _ := json.Marshal(in.ToolInput) // map keys marshal sorted
	sum := sha256.Sum256(append([]byte(in.ToolName+"\x00"), b...))
	h := hex.EncodeToString(sum[:8])

	var st stuckState
	loadState("stuck", in.SessionID, &st)
	st.Recent = append(st.Recent, h)
	if len(st.Recent) > stuckWindow {
		st.Recent = st.Recent[len(st.Recent)-stuckWindow:]
	}
	saveState("stuck", in.SessionID, st)

	n := 0
	for _, r := range st.Recent {
		if r == h {
			n++
		}
	}
	switch {
	case n >= 5:
		return addContext("PreToolUse", fmt.Sprintf(
			"[stuck] This is the %dth identical %s call in your last %d tool calls. You are likely in a loop. Stop, state what you expected versus what happened, and change approach or ask the user.",
			n, in.ToolName, len(st.Recent)))
	case n >= 3:
		return addContext("PreToolUse", fmt.Sprintf(
			"[stuck] You have made this exact %s call %d times in your last %d tool calls. If the result will not change, try something different.",
			in.ToolName, n, len(st.Recent)))
	}
	return nil
}

package patches

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const verifyMaxAttempts = 3

type verifyState struct {
	Dirty    bool `json:"dirty"`
	Attempts int  `json:"attempts"`
}

// verifyHook marks the session dirty when Claude edits files, then on Stop
// runs the project's checks and refuses to let Claude finish while they
// fail, up to verifyMaxAttempts times in a row.
func verifyHook(in *Input, _ time.Time) any {
	var st verifyState
	loadState("verify", in.SessionID, &st)
	if in.HookEventName == "PostToolUse" {
		if !st.Dirty {
			st.Dirty = true
			saveState("verify", in.SessionID, st)
		}
		return nil
	}
	if in.HookEventName != "Stop" || !st.Dirty {
		return nil
	}

	cmds := verifyCommands(in.Cwd)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	for _, c := range cmds {
		out, err := shell(ctx, in.Cwd, c)
		if err == nil {
			continue
		}
		st.Attempts++
		if st.Attempts > verifyMaxAttempts {
			saveState("verify", in.SessionID, verifyState{})
			return tellUser(fmt.Sprintf("claude_patcher verify: `%s` still fails after %d attempts; letting Claude stop.", c, verifyMaxAttempts))
		}
		saveState("verify", in.SessionID, st)
		return block(fmt.Sprintf(
			"claude_patcher verify (attempt %d/%d): `%s` failed.\n\n%s\n\nFix the failure before finishing. If it cannot be fixed or is unrelated to your change, say so plainly to the user.",
			st.Attempts, verifyMaxAttempts, c, tail(out, 60)))
	}
	saveState("verify", in.SessionID, verifyState{})
	return nil
}

// verifyCommands returns the project's configured checks, or detects them
// from the files in cwd.
func verifyCommands(cwd string) []string {
	if c := loadProjectConfig(cwd); len(c.Verify) > 0 {
		return c.Verify
	}
	has := func(name string) bool { return fileExists(filepath.Join(cwd, name)) }
	onPath := func(name string) bool { _, err := exec.LookPath(name); return err == nil }
	var cmds []string
	if has("go.mod") && onPath("go") {
		cmds = append(cmds, "go vet ./...", "go test ./...")
	}
	if has("Cargo.toml") && onPath("cargo") {
		cmds = append(cmds, "cargo test")
	}
	if has("package.json") {
		cmds = append(cmds, npmScripts(cwd)...)
	}
	if (has("pyproject.toml") || has("setup.py")) && onPath("pytest") && (has("tests") || has("test")) {
		cmds = append(cmds, "pytest -q")
	}
	return cmds
}

func npmScripts(cwd string) []string {
	b, err := os.ReadFile(filepath.Join(cwd, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	runner := "npm run"
	switch {
	case fileExists(filepath.Join(cwd, "pnpm-lock.yaml")):
		runner = "pnpm run"
	case fileExists(filepath.Join(cwd, "yarn.lock")):
		runner = "yarn run"
	case fileExists(filepath.Join(cwd, "bun.lock")), fileExists(filepath.Join(cwd, "bun.lockb")):
		runner = "bun run"
	}
	var cmds []string
	for _, s := range []string{"typecheck", "lint", "test"} {
		script, ok := pkg.Scripts[s]
		if !ok || strings.Contains(script, "no test specified") || strings.Contains(script, "--watch") {
			continue
		}
		cmds = append(cmds, runner+" "+s)
	}
	return cmds
}

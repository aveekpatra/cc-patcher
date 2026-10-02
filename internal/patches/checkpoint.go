package patches

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// checkpointHook snapshots the working tree at the end of each turn into
// refs/claude-checkpoints/<session>. It uses a scratch index, so the
// user's branch, index and files are never touched.
func checkpointHook(in *Input, now time.Time) any {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g := func(env []string, args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = in.Cwd
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil
	}
	top, err := g(nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	index, err := g(nil, "rev-parse", "--path-format=absolute", "--git-path", "index")
	if err != nil {
		return nil
	}

	tmp, err := os.CreateTemp("", "cc-patcher-index-*")
	if err != nil {
		return nil
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if b, err := os.ReadFile(index); err == nil {
		_ = os.WriteFile(tmp.Name(), b, 0o644)
	} else {
		os.Remove(tmp.Name()) // git creates a fresh index
	}
	env := []string{"GIT_INDEX_FILE=" + tmp.Name()}
	if _, err := g(env, "-C", top, "add", "-A"); err != nil {
		return nil
	}
	tree, err := g(env, "write-tree")
	if err != nil {
		return nil
	}

	id := unsafeID.ReplaceAllString(in.SessionID, "")
	if id == "" {
		id = "unknown"
	}
	ref := "refs/claude-checkpoints/" + id
	head, _ := g(nil, "rev-parse", "--verify", "-q", "HEAD")
	prev, _ := g(nil, "rev-parse", "--verify", "-q", ref)
	last := prev
	if last == "" {
		last = head
	}
	if last != "" {
		if t, _ := g(nil, "rev-parse", last+"^{tree}"); t == tree {
			return nil // nothing changed since the last snapshot
		}
	}

	args := []string{"commit-tree", tree, "-m", fmt.Sprintf("claude checkpoint %s (%s)", now.Format(time.RFC3339), filepath.Base(top))}
	if head != "" {
		args = append(args, "-p", head)
	}
	if prev != "" && prev != head {
		args = append(args, "-p", prev)
	}
	commit, err := g([]string{
		"GIT_AUTHOR_NAME=cc-patcher", "GIT_AUTHOR_EMAIL=cc-patcher@localhost",
		"GIT_COMMITTER_NAME=cc-patcher", "GIT_COMMITTER_EMAIL=cc-patcher@localhost",
	}, args...)
	if err != nil {
		return nil
	}
	_, _ = g(nil, "update-ref", "-m", "claude checkpoint", "--create-reflog", ref, commit)
	return nil
}

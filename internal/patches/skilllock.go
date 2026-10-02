package patches

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

func init() {
	extraPatches = append(extraPatches, &Patch{
		name: "Skill drift lock", hookArg: "skilllock",
		desc: "Warns at session start when an installed skill was added, changed or removed",
		regs: []reg{{"SessionStart", "", 10, false}},
		setup: func() error {
			saveLock(hashSkills())
			return nil
		},
	})
	handlers["skilllock"] = skilllockHook
}

func lockPath() string { return filepath.Join(claude.StateDir(), "skills.lock.json") }

func skilllockHook(in *Input, _ time.Time) any {
	if in.Source == "compact" {
		return nil
	}
	now := hashSkills()
	var before map[string]string
	b, err := os.ReadFile(lockPath())
	if err != nil || json.Unmarshal(b, &before) != nil {
		saveLock(now)
		return nil
	}
	var changes []string
	for name, h := range now {
		switch old, ok := before[name]; {
		case !ok:
			changes = append(changes, name+" (new)")
		case old != h:
			changes = append(changes, name+" (changed)")
		}
	}
	for name := range before {
		if _, ok := now[name]; !ok {
			changes = append(changes, name+" (removed)")
		}
	}
	if len(changes) == 0 {
		return nil
	}
	sort.Strings(changes)
	saveLock(now)
	return tellUser("cc-patcher: skills changed since last session: " + strings.Join(changes, ", ") +
		". Review them if you did not change them yourself.")
}

func saveLock(m map[string]string) {
	_ = os.MkdirAll(claude.StateDir(), 0o755)
	b, _ := json.MarshalIndent(m, "", "  ")
	_ = os.WriteFile(lockPath(), b, 0o644)
}

// hashSkills hashes every file of every skill in ~/.claude/skills,
// following symlinked skill folders.
func hashSkills() map[string]string {
	root := filepath.Join(claude.Dir(), "skills")
	out := map[string]string{}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
		}
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			continue
		}
		h := sha256.New()
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, p)
			io.WriteString(h, filepath.ToSlash(rel)+"\x00")
			if f, err := os.Open(p); err == nil {
				_, _ = io.Copy(h, f)
				f.Close()
			}
			return nil
		})
		out[e.Name()] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}

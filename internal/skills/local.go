package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// Local is a skill already in ~/.claude/skills that cc-patcher did not
// ship: one you wrote, or one another tool installed. Uninstalling moves
// it to a trash folder, so installing it again puts it back.
type Local struct {
	name string
	desc string
	link string // symlink target when the skill folder is a link
}

const (
	maxSkillFile  = 1 << 20
	maxSkillTotal = 10 << 20
)

func skillsDir() string { return filepath.Join(claude.Dir(), "skills") }

func trashDir() string { return filepath.Join(claude.StateDir(), "removed-skills") }

func (l *Local) Name() string { return l.name }

func (l *Local) Description() string {
	d := l.desc
	if d == "" {
		d = "no description"
	}
	if l.link != "" {
		return d + " (your skill, linked from " + l.link + ")"
	}
	return d + " (your skill)"
}

func (l *Local) target() string  { return filepath.Join(skillsDir(), l.name) }
func (l *Local) trashed() string { return filepath.Join(trashDir(), l.name) }

func (l *Local) Enabled() bool {
	_, err := os.Stat(filepath.Join(l.target(), "SKILL.md"))
	return err == nil
}

// Disable moves the skill folder (or just the link, for a linked skill)
// into the trash. The link's target is never touched.
func (l *Local) Disable() error {
	if err := os.MkdirAll(trashDir(), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(l.trashed())
	return os.Rename(l.target(), l.trashed())
}

// Enable restores the skill from the trash.
func (l *Local) Enable() error {
	if _, err := os.Lstat(l.trashed()); err != nil {
		return fmt.Errorf("%s is not in the trash; nothing to restore", l.name)
	}
	if _, err := os.Lstat(l.target()); err == nil {
		return fmt.Errorf("%s already exists", l.target())
	}
	return os.Rename(l.trashed(), l.target())
}

// Files returns the skill's files for export, relative to its folder.
// Large files and version control folders are skipped.
func (l *Local) Files() (map[string][]byte, error) {
	root := l.target()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	out := map[string][]byte{}
	total := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSkillFile || total+int(info.Size()) > maxSkillTotal {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = b
		total += len(b)
		return nil
	})
	return out, err
}

// LocalList returns every skill in ~/.claude/skills (and in the trash)
// whose name is not in skip, which holds the names cc-patcher manages.
func LocalList(skip map[string]bool) []*Local {
	seen := map[string]bool{}
	var out []*Local
	for _, dir := range []string{skillsDir(), trashDir()} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if skip[name] || seen[name] || strings.HasPrefix(name, ".") {
				continue
			}
			p := filepath.Join(dir, name)
			b, err := os.ReadFile(filepath.Join(p, "SKILL.md"))
			if err != nil {
				continue
			}
			l := &Local{name: name, desc: frontmatter(string(b), "description")}
			if t, err := os.Readlink(p); err == nil {
				l.link = t
			}
			seen[name] = true
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// InstallFiles writes an exported skill into ~/.claude/skills/<name>. It
// refuses to overwrite a skill that is already there.
func InstallFiles(name string, files map[string][]byte) error {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("bad skill name %q", name)
	}
	if _, ok := files["SKILL.md"]; !ok {
		return errors.New(name + ": no SKILL.md in the export")
	}
	dst := filepath.Join(skillsDir(), name)
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%s already exists", dst)
	}
	stage := dst + ".cc-patcher-tmp"
	_ = os.RemoveAll(stage)
	for rel, b := range files {
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." || strings.Contains(rel, `\`) {
			_ = os.RemoveAll(stage)
			return fmt.Errorf("%s: unsafe path %q", name, rel)
		}
		p := filepath.Join(stage, clean)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			_ = os.RemoveAll(stage)
			return err
		}
	}
	return os.Rename(stage, dst)
}

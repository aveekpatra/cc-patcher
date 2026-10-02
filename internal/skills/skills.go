// Package skills installs bundled skills into ~/.claude/skills.
package skills

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aveekpatra/claude_patcher/internal/claude"
)

// Skill is one bundled skill directory.
type Skill struct {
	src  fs.FS
	name string
	desc string
}

func (s *Skill) Name() string        { return s.name }
func (s *Skill) Description() string { return s.desc }

func (s *Skill) target() string { return filepath.Join(claude.Dir(), "skills", s.name) }

func (s *Skill) Enabled() bool {
	_, err := os.Stat(filepath.Join(s.target(), "SKILL.md"))
	return err == nil
}

func (s *Skill) Enable() error {
	dst := s.target()
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return fs.WalkDir(s.src, s.name, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dst, filepath.FromSlash(strings.TrimPrefix(p, s.name)))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := fs.ReadFile(s.src, p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, b, 0o644)
	})
}

func (s *Skill) Disable() error { return os.RemoveAll(s.target()) }

// List returns every skill in src (one directory per skill, each with SKILL.md).
func List(src fs.FS) ([]*Skill, error) {
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return nil, err
	}
	var out []*Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := fs.ReadFile(src, path.Join(e.Name(), "SKILL.md"))
		if err != nil {
			continue
		}
		out = append(out, &Skill{src: src, name: e.Name(), desc: frontmatter(string(b), "description")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// Find returns the named skill, or nil.
func Find(src fs.FS, name string) *Skill {
	all, _ := List(src)
	for _, s := range all {
		if s.name == name {
			return s
		}
	}
	return nil
}

func frontmatter(doc, key string) string {
	if !strings.HasPrefix(doc, "---") {
		return ""
	}
	for _, line := range strings.Split(doc, "\n")[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// BaseURL is where repo tarballs come from. Tests point it at a local server.
var BaseURL = "https://codeload.github.com"

// Marker is written into every remote skill dir we install, so Disable only
// ever removes dirs that cc-patcher put there.
const Marker = ".cc-patcher-remote.json"

const (
	maxArchive = 64 << 20
	maxFile    = 16 << 20
)

// Source is one skill inside a repo: Path is the folder holding SKILL.md
// ("" for the repo root) and Name is the dir it gets under ~/.claude/skills.
type Source struct {
	Path string
	Name string
}

// Remote is a third-party skill set downloaded from GitHub at install time.
// Nothing from these repos is bundled, so their licenses stay their own.
type Remote struct {
	Title   string
	Desc    string
	Repo    string // owner/repo
	Ref     string
	License string
	Skills  []Source
}

func (r *Remote) Name() string { return r.Title }

func (r *Remote) Description() string {
	return r.Desc + " (downloads from GitHub, " + r.License + ")"
}

func remoteTarget(name string) string { return filepath.Join(claude.Dir(), "skills", name) }

func (r *Remote) Enabled() bool {
	for _, s := range r.Skills {
		if _, err := os.Stat(filepath.Join(remoteTarget(s.Name), "SKILL.md")); err != nil {
			return false
		}
	}
	return true
}

func (r *Remote) Disable() error {
	for _, s := range r.Skills {
		dir := remoteTarget(s.Name)
		if _, err := os.Stat(filepath.Join(dir, Marker)); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

func (r *Remote) Enable() error {
	root := filepath.Join(claude.Dir(), "skills")
	for _, s := range r.Skills {
		dir := filepath.Join(root, s.Name)
		if _, err := os.Stat(dir); err == nil {
			if _, err := os.Stat(filepath.Join(dir, Marker)); err != nil {
				return fmt.Errorf("%s exists and was not installed by cc-patcher", dir)
			}
		}
	}
	data, err := download(r.Repo, r.Ref)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, ".cc-patcher-tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extract(data, stage, r.Skills); err != nil {
		return fmt.Errorf("%s: %w", r.Repo, err)
	}
	mark, _ := json.MarshalIndent(map[string]string{
		"source":    "https://github.com/" + r.Repo,
		"ref":       r.Ref,
		"installed": time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	for _, s := range r.Skills {
		tmp := filepath.Join(stage, s.Name)
		if _, err := os.Stat(filepath.Join(tmp, "SKILL.md")); err != nil {
			return fmt.Errorf("%s: no SKILL.md under %q", r.Repo, s.Path)
		}
		if err := os.WriteFile(filepath.Join(tmp, Marker), append(mark, '\n'), 0o644); err != nil {
			return err
		}
	}
	for _, s := range r.Skills {
		dst := filepath.Join(root, s.Name)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stage, s.Name), dst); err != nil {
			return err
		}
	}
	return nil
}

func download(repo, ref string) ([]byte, error) {
	url := strings.TrimSuffix(BaseURL, "/") + "/" + repo + "/tar.gz/" + ref
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxArchive+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxArchive {
		return nil, fmt.Errorf("GET %s: archive larger than %d bytes", url, maxArchive)
	}
	return b, nil
}

// extract writes each source folder of a GitHub tarball into stage/<name>.
// Only regular files and dirs are written; symlinks and other entry types
// are skipped, and any path that would leave its skill dir is an error.
func extract(data []byte, stage string, srcs []Source) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			continue
		}
		if strings.Contains(h.Name, `\`) || path.IsAbs(h.Name) {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		// GitHub tarballs nest everything under "<repo>-<ref>/".
		_, p, _ := strings.Cut(h.Name, "/")
		for _, part := range strings.Split(p, "/") {
			if part == ".." {
				return fmt.Errorf("unsafe path %q", h.Name)
			}
		}
		p = strings.TrimSuffix(p, "/")
		for _, s := range srcs {
			rel, ok := within(p, s.Path)
			if !ok {
				continue
			}
			dir := filepath.Join(stage, s.Name)
			out := filepath.Join(dir, filepath.FromSlash(rel))
			if r, err := filepath.Rel(dir, out); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe path %q", h.Name)
			}
			if h.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(out, 0o755); err != nil {
					return err
				}
				continue
			}
			if h.Size > maxFile {
				return fmt.Errorf("%s: file too large", h.Name)
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			b, err := io.ReadAll(io.LimitReader(tr, maxFile))
			if err != nil {
				return err
			}
			if err := os.WriteFile(out, b, 0o644); err != nil {
				return err
			}
		}
	}
}

// within reports p's path relative to dir, if p is dir or below it.
func within(p, dir string) (string, bool) {
	if dir == "" {
		return p, true
	}
	if p == dir {
		return "", true
	}
	if rel, ok := strings.CutPrefix(p, dir+"/"); ok {
		return rel, true
	}
	return "", false
}

func thinking(names ...string) []Source {
	out := make([]Source, len(names))
	for i, n := range names {
		out[i] = Source{Path: "skills/thinking-" + n, Name: "thinking-" + n}
	}
	return out
}

// RemoteList is the catalog of third-party skills, verified against each
// repo's default branch.
func RemoteList() []*Remote {
	return []*Remote{
		{
			Title: "karpathy-rules", Desc: "Karpathy's guidelines against common LLM coding mistakes",
			Repo: "multica-ai/andrej-karpathy-skills", Ref: "main", License: "MIT",
			Skills: []Source{{Path: "skills/karpathy-guidelines", Name: "karpathy-guidelines"}},
		},
		{
			Title: "thinking-frameworks", Desc: "28 mental models: first principles, pre-mortem, OODA, red team...",
			Repo: "tjboudreaux/cc-thinking-skills", Ref: "main", License: "MIT",
			Skills: thinking(
				"bounded-rationality", "circle-of-competence", "cynefin", "effectuation",
				"first-principles", "five-whys-plus", "jobs-to-be-done", "kepner-tregoe",
				"lindy-effect", "map-territory", "margin-of-safety", "model-combination",
				"model-router", "ooda", "opportunity-cost", "pre-mortem", "probabilistic",
				"red-team", "reversibility", "scientific-method", "second-order", "socratic",
				"steel-manning", "systems", "theory-of-constraints", "thought-experiment",
				"triz", "via-negativa",
			),
		},
		{
			Title: "avoid-ai-writing", Desc: "Audit and rewrite text to remove AI writing patterns",
			Repo: "conorbronsdon/avoid-ai-writing", Ref: "main", License: "MIT",
			Skills: []Source{{Path: "skills/avoid-ai-writing", Name: "avoid-ai-writing"}},
		},
		{
			Title: "visual-explainer", Desc: "Self-contained HTML diagrams, reviews and slide decks",
			Repo: "nicobailon/visual-explainer", Ref: "main", License: "MIT",
			Skills: []Source{{Path: "plugins/visual-explainer", Name: "visual-explainer"}},
		},
		{
			Title: "codebase-to-course", Desc: "Turn a codebase into an interactive HTML course",
			Repo: "zarazhangrui/codebase-to-course", Ref: "main", License: "no license stated",
			Skills: []Source{{Path: "", Name: "codebase-to-course"}},
		},
		{
			Title: "second-opinion", Desc: "Run Codex CLI for a second-opinion code review",
			Repo: "skills-directory/skill-codex", Ref: "main", License: "MIT",
			Skills: []Source{{Path: "plugins/skill-codex/skills/codex", Name: "codex"}},
		},
	}
}

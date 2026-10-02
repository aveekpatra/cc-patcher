package patches

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// tool is a formatter or linter that runs on a single file.
type tool struct {
	name string
	args func(file string) []string
}

var formatters = map[string][]tool{
	".go": {{"gofmt", func(f string) []string { return []string{"-w", f} }}},
	".rs": {{"rustfmt", func(f string) []string { return []string{f} }}},
	".py": {{"ruff", func(f string) []string { return []string{"format", f} }}},
	".js": {prettier}, ".jsx": {prettier}, ".ts": {prettier}, ".tsx": {prettier},
	".mjs": {prettier}, ".cjs": {prettier}, ".css": {prettier}, ".scss": {prettier},
	".json": {prettier}, ".html": {prettier}, ".vue": {prettier}, ".svelte": {prettier},
	".yaml": {prettier}, ".yml": {prettier},
}

var prettier = tool{"prettier", func(f string) []string { return []string{"--write", "--log-level", "warn", f} }}

var linters = map[string]tool{
	".py": {"ruff", func(f string) []string { return []string{"check", "--quiet", f} }},
	".js": eslint, ".jsx": eslint, ".ts": eslint, ".tsx": eslint, ".mjs": eslint,
	".vue": eslint, ".svelte": eslint,
}

var eslint = tool{"eslint", func(f string) []string { return []string{f} }}

// checkHook formats a file Claude just edited and reports lint problems.
// Missing tools are skipped silently.
func checkHook(in *Input, _ time.Time) any {
	file := in.str("file_path")
	if file == "" || !fileExists(file) {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(file))
	dir := filepath.Dir(file)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	var notes []string
	before, _ := os.ReadFile(file)
	for _, t := range formatters[ext] {
		if bin := findTool(t.name, dir); bin != "" {
			_, _ = run(exec.CommandContext(ctx, bin, t.args(file)...), dir)
		}
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(before, after) {
		notes = append(notes, "Reformatted "+filepath.Base(file)+"; re-read it before your next edit to it.")
	}

	var out string
	var err error
	if ext == ".go" {
		if bin := findTool("go", dir); bin != "" {
			out, err = run(exec.CommandContext(ctx, bin, "vet", "."), dir)
		}
	} else if t, ok := linters[ext]; ok {
		if bin := findTool(t.name, dir); bin != "" {
			out, err = run(exec.CommandContext(ctx, bin, t.args(file)...), dir)
		}
	}
	if err != nil && out != "" {
		notes = append(notes, "Lint problems:\n"+tail(out, 40)+"\nFix these before moving on.")
	}
	if len(notes) == 0 {
		return nil
	}
	return addContext("PostToolUse", "[check] "+strings.Join(notes, "\n"))
}

// findTool prefers a project-local node_modules/.bin binary, then PATH.
func findTool(name, dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		for _, n := range []string{name, name + ".cmd"} {
			if p := filepath.Join(d, "node_modules", ".bin", n); fileExists(p) {
				return p
			}
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}

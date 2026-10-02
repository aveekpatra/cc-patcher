// Package files answers Claude Code's @-file suggestion requests with a
// fuzzy match over the project's files.
package files

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxResults = 15

// Run reads {"query": "..."} from stdin and prints the best matches.
func Run(stdin io.Reader, stdout io.Writer) {
	var in struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(stdin).Decode(&in)
	root := os.Getenv("CLAUDE_PROJECT_DIR")
	if root == "" {
		root, _ = os.Getwd()
	}
	w := bufio.NewWriter(stdout)
	defer w.Flush()
	for _, p := range Match(list(root), in.Query, maxResults) {
		w.WriteString(p + "\n")
	}
}

// list returns project files: git's view when available, else a bounded walk.
func list(root string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		return strings.Split(strings.TrimSpace(string(bytes.ReplaceAll(out, []byte("\r"), nil))), "\n")
	}
	var paths []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || len(paths) > 50000 {
			return filepath.SkipDir
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".venv", "target", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(root, p); err == nil {
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return paths
}

// Match ranks paths by how well query matches them as a subsequence,
// favoring matches in the file name, consecutive runs and short paths.
func Match(paths []string, query string, n int) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	type hit struct {
		path  string
		score int
	}
	var hits []hit
	for _, p := range paths {
		if p == "" {
			continue
		}
		if s, ok := score(strings.ToLower(p), q); ok {
			hits = append(hits, hit{p, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return len(hits[i].path) < len(hits[j].path)
	})
	var out []string
	for i := 0; i < len(hits) && i < n; i++ {
		out = append(out, hits[i].path)
	}
	return out
}

func score(path, q string) (int, bool) {
	if q == "" {
		return -len(path), true
	}
	base := strings.LastIndex(path, "/") + 1
	s, run, qi := 0, 0, 0
	prev := -2
	for i := 0; i < len(path) && qi < len(q); i++ {
		if path[i] != q[qi] {
			continue
		}
		pts := 1
		if i == prev+1 {
			run++
			pts += 2 * run
		} else {
			run = 0
		}
		if i >= base {
			pts += 3
		}
		if i == 0 || strings.ContainsRune("/._-", rune(path[i-1])) {
			pts += 4
		}
		s += pts
		prev = i
		qi++
	}
	if qi < len(q) {
		return 0, false
	}
	if strings.Contains(path[base:], q) {
		s += 20
	}
	return s - len(path)/8, true
}

// Package backup captures a user's whole Claude Code setup: the user-level
// files in ~/.claude that define how Claude Code behaves, plus the MCP
// servers from ~/.claude.json. Session data, history, caches and login
// credentials are never included.
package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// Backup is the setup part of an export.
type Backup struct {
	Home string `json:"home"` // home folder of the machine it came from
	// Files maps paths relative to ~/.claude to their contents.
	Files map[string][]byte `json:"files"`
	// MCPServers is the user-scope "mcpServers" object from ~/.claude.json.
	MCPServers map[string]any `json:"mcp_servers,omitempty"`
	Redacted   bool           `json:"redacted"`
}

// What gets backed up, relative to ~/.claude.
var (
	files = []string{"settings.json", "CLAUDE.md", "keybindings.json"}
	dirs  = []string{"skills", "agents", "commands", "output-styles", "hooks", "themes", "rules"}
)

const (
	maxFile  = 1 << 20
	maxTotal = 64 << 20
	redacted = "<redacted by cc-patcher>"
)

var secretKey = regexp.MustCompile(`(?i)(token|secret|password|passwd|api[_-]?key|auth|credential|private[_-]?key|cookie)`)

func claudeJSONPath() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, ".claude.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude.json")
}

// Take captures the current setup. With secrets false, values of env vars,
// headers and MCP settings whose names look like secrets are replaced.
func Take(secrets bool) (*Backup, error) {
	home, _ := os.UserHomeDir()
	b := &Backup{Home: home, Files: map[string][]byte{}, Redacted: !secrets}
	root := claude.Dir()
	total := 0
	add := func(rel string, data []byte) {
		if len(data) > maxFile || total+len(data) > maxTotal {
			return
		}
		b.Files[filepath.ToSlash(rel)] = data
		total += len(data)
	}
	for _, f := range files {
		if data, err := os.ReadFile(filepath.Join(root, f)); err == nil {
			if f == "settings.json" && !secrets {
				data = redactJSON(data)
			}
			add(f, data)
		}
	}
	for _, d := range dirs {
		walk(filepath.Join(root, d), d, add)
	}
	if data, err := os.ReadFile(claudeJSONPath()); err == nil {
		var cj map[string]any
		if json.Unmarshal(data, &cj) == nil {
			if m, ok := cj["mcpServers"].(map[string]any); ok && len(m) > 0 {
				if !secrets {
					redact(m)
				}
				b.MCPServers = m
			}
		}
	}
	return b, nil
}

// walk adds every regular file under dir, following symlinked folders one
// level deep (skills are often links), skipping VCS and dependency folders.
func walk(dir, rel string, add func(string, []byte)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".git" || name == "node_modules" || name == ".DS_Store" {
			continue
		}
		p := filepath.Join(dir, name)
		info, err := os.Stat(p) // follows links
		if err != nil {
			continue
		}
		if info.IsDir() {
			walk(p, filepath.Join(rel, name), add)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			add(filepath.Join(rel, name), data)
		}
	}
}

func redactJSON(data []byte) []byte {
	var v map[string]any
	if json.Unmarshal(data, &v) != nil {
		return data
	}
	redact(v)
	if out, err := marshal(v); err == nil {
		return out
	}
	return data
}

// marshal writes indented JSON without escaping <, > and &, so the
// redaction placeholder stays readable and matchable.
func marshal(v any) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// redact replaces string values whose key name looks like a secret, at any
// depth (env vars, headers, MCP settings).
func redact(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if s, ok := val.(string); ok && s != "" && secretKey.MatchString(k) {
				t[k] = redacted
				continue
			}
			redact(val)
		}
	case []any:
		for _, x := range t {
			redact(x)
		}
	}
}

// Restore writes the backup into this machine's Claude Code setup. Files it
// replaces are copied first to ~/.claude/cc-patcher/restore-<time>/. Home
// paths are rewritten to this machine's home, and redacted secrets keep
// whatever value this machine already has. It returns a short report.
func Restore(b *Backup) ([]string, error) {
	root := claude.Dir()
	home, _ := os.UserHomeDir()
	saveDir := filepath.Join(claude.StateDir(), "restore-"+time.Now().Format("20060102-150405"))
	var report []string
	written, kept, replaced := 0, 0, 0

	rels := make([]string, 0, len(b.Files))
	for rel := range b.Files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			report = append(report, "skipped unsafe path "+rel)
			continue
		}
		dst := filepath.Join(root, clean)
		data := b.Files[rel]
		if b.Home != "" && b.Home != home && isText(data) {
			data = []byte(strings.ReplaceAll(string(data), b.Home, home))
		}
		old, err := os.ReadFile(dst)
		if err == nil {
			if clean == "settings.json" {
				data = keepSecrets(data, old)
			}
			if string(old) == string(data) {
				kept++
				continue
			}
			replaced++
			bak := filepath.Join(saveDir, clean)
			_ = os.MkdirAll(filepath.Dir(bak), 0o755)
			_ = os.WriteFile(bak, old, 0o644)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return report, err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(clean, ".sh") || strings.HasPrefix(clean, "hooks"+string(filepath.Separator)) {
			mode = 0o755
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			return report, err
		}
		written++
	}

	if len(b.MCPServers) > 0 {
		n, err := mergeMCP(b, home)
		if err != nil {
			report = append(report, "MCP servers not restored: "+err.Error())
		} else if n > 0 {
			report = append(report, fmt.Sprintf("added %d MCP server(s) to ~/.claude.json", n))
		}
	}
	report = append([]string{fmt.Sprintf("restored %d file(s), %d already matched", written, kept)}, report...)
	if replaced > 0 {
		report = append(report, fmt.Sprintf("%d replaced file(s) were saved in %s", replaced, saveDir))
	}
	if b.Redacted {
		report = append(report, "secrets were redacted in this backup; set them again where a value shows "+redacted)
	}
	return report, nil
}

// keepSecrets puts this machine's values back wherever the backup has the
// redaction placeholder.
func keepSecrets(data, old []byte) []byte {
	if !strings.Contains(string(data), redacted) {
		return data
	}
	var nv, ov map[string]any
	if json.Unmarshal(data, &nv) != nil || json.Unmarshal(old, &ov) != nil {
		return data
	}
	fill(nv, ov)
	if out, err := marshal(nv); err == nil {
		return out
	}
	return data
}

func fill(nv, ov map[string]any) {
	for k, v := range nv {
		switch t := v.(type) {
		case string:
			if t == redacted {
				if o, ok := ov[k].(string); ok {
					nv[k] = o
				}
			}
		case map[string]any:
			if o, ok := ov[k].(map[string]any); ok {
				fill(t, o)
			}
		}
	}
}

// mergeMCP adds servers this machine does not have yet; existing servers
// are left alone.
func mergeMCP(b *Backup, home string) (int, error) {
	p := claudeJSONPath()
	cj := map[string]any{}
	if data, err := os.ReadFile(p); err == nil {
		if err := json.Unmarshal(data, &cj); err != nil {
			return 0, err
		}
	}
	m, _ := cj["mcpServers"].(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	added := 0
	for name, srv := range b.MCPServers {
		if _, ok := m[name]; ok {
			continue
		}
		if b.Home != "" && b.Home != home {
			raw, _ := json.Marshal(srv)
			_ = json.Unmarshal([]byte(strings.ReplaceAll(string(raw), b.Home, home)), &srv)
		}
		m[name] = srv
		added++
	}
	if added == 0 {
		return 0, nil
	}
	cj["mcpServers"] = m
	out, err := marshal(cj)
	if err != nil {
		return 0, err
	}
	return added, os.WriteFile(p, out, 0o600)
}

func isText(b []byte) bool {
	n := min(len(b), 8000)
	for _, c := range b[:n] {
		if c == 0 {
			return false
		}
	}
	return true
}

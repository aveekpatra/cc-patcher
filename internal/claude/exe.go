package claude

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SelfPath returns the path hooks should run cc-patcher from. A binary
// built by `go run` or `go test` lives in a temporary folder that Go
// deletes later, which would break every hook, so in that case the
// installed copy is used instead.
func SelfPath() string {
	exe, err := os.Executable()
	if err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
	}
	if err != nil || IsTempBuild(exe) {
		exe = installedPath()
	}
	if runtime.GOOS == "windows" {
		exe = filepath.ToSlash(exe)
	}
	return exe
}

// IsTempBuild reports whether path is a throwaway Go build.
func IsTempBuild(path string) bool {
	p := filepath.ToSlash(path)
	return strings.Contains(p, "/go-build") || strings.HasSuffix(p, ".test") ||
		strings.HasPrefix(p, filepath.ToSlash(os.TempDir()))
}

// installedPath finds an installed cc-patcher: on PATH, or where the
// install scripts put it.
func installedPath() string {
	if p, err := exec.LookPath("cc-patcher"); err == nil && !IsTempBuild(p) {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".local", "bin", "cc-patcher"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "cc-patcher", "cc-patcher.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "cc-patcher"
}

// SelfCommand returns a shell command line that runs cc-patcher with args,
// for use in settings.json hooks and commands.
func SelfCommand(args string) string {
	return `"` + SelfPath() + `" ` + args
}

// RepairPaths points every cc-patcher command in settings.json at the
// current binary when the path it uses is gone or is a temporary build.
// It returns how many commands it fixed.
func RepairPaths() int {
	self := SelfPath()
	if IsTempBuild(self) {
		return 0
	}
	if _, err := os.Stat(self); err != nil {
		return 0
	}
	s, err := LoadSettings()
	if err != nil {
		return 0
	}
	fixed := 0
	fix := func(cmd string) string {
		if !strings.HasPrefix(cmd, `"`) {
			return cmd
		}
		end := strings.Index(cmd[1:], `"`)
		if end < 0 {
			return cmd
		}
		path := cmd[1 : end+1]
		base := strings.TrimSuffix(filepath.Base(filepath.FromSlash(path)), ".exe")
		if base != "cc-patcher" || path == self {
			return cmd
		}
		if _, err := os.Stat(filepath.FromSlash(path)); err == nil && !IsTempBuild(path) {
			return cmd
		}
		fixed++
		return `"` + self + `"` + cmd[end+2:]
	}
	if hooks, ok := s["hooks"].(map[string]any); ok {
		for _, groups := range hooks {
			list, _ := groups.([]any)
			for _, g := range list {
				gm, _ := g.(map[string]any)
				hs, _ := gm["hooks"].([]any)
				for _, h := range hs {
					if hm, ok := h.(map[string]any); ok {
						if c, ok := hm["command"].(string); ok {
							hm["command"] = fix(c)
						}
					}
				}
			}
		}
	}
	for _, key := range []string{"statusLine", "fileSuggestion"} {
		if m, ok := s[key].(map[string]any); ok {
			if c, ok := m["command"].(string); ok {
				m["command"] = fix(c)
			}
		}
	}
	if fixed > 0 {
		_ = SaveSettings(s)
	}
	return fixed
}

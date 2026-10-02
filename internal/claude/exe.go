package claude

import (
	"os"
	"path/filepath"
	"runtime"
)

// SelfCommand returns a shell command line that runs this binary with args,
// for use in settings.json hooks and commands.
func SelfCommand(args string) string {
	exe, err := os.Executable()
	if err == nil {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
	} else {
		exe = "cc-patcher"
	}
	if runtime.GOOS == "windows" {
		exe = filepath.ToSlash(exe)
	}
	return `"` + exe + `" ` + args
}

package patches

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// guardHook denies catastrophic commands outright and asks the user before
// risky ones. It also keeps Claude away from .env secrets.
func guardHook(in *Input, _ time.Time) any {
	switch in.ToolName {
	case "Bash":
		decision, reason := checkCommand(in.str("command"))
		if decision != "" {
			return toolDecision(decision, "cc-patcher guard: "+reason)
		}
	case "Read", "Edit", "Write", "MultiEdit":
		if isEnvFile(in.str("file_path")) {
			return toolDecision("deny", "cc-patcher guard: .env files hold secrets; ask the user for the value you need")
		}
	}
	return nil
}

type rule struct {
	re     *regexp.Regexp
	reason string
}

var denyRules = []rule{
	{regexp.MustCompile(`(?i)\b(drop\s+(table|database|schema)|truncate\s+table)\b`), "destructive SQL"},
	{regexp.MustCompile(`\bmkfs(\.\w+)?\b`), "formats a disk"},
	{regexp.MustCompile(`\bdd\b.*\bof=/dev/`), "writes to a raw device"},
	{regexp.MustCompile(`>\s*/dev/(sd|nvme|disk)`), "writes to a raw device"},
	{regexp.MustCompile(`:\(\)\s*\{\s*:\s*\|\s*:\s*&\s*\}`), "fork bomb"},
	{regexp.MustCompile(`\bchmod\s+(-\w*R\w*\s+)?777\s+/(\s|$)`), "opens up the whole filesystem"},
}

var askRules = []rule{
	{regexp.MustCompile(`\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba|z|da)?sh\b`), "pipes a download straight into a shell"},
	{regexp.MustCompile(`\bgit\s+reset\s+--hard\b`), "discards uncommitted work"},
	{regexp.MustCompile(`\bgit\s+clean\s+-\w*f`), "deletes untracked files"},
	{regexp.MustCompile(`\bgit\s+checkout\s+(--\s+)?\.(\s|$)`), "discards uncommitted work"},
	{regexp.MustCompile(`\bgit\s+push\b.*(\s--force(\s|$)|\s-f(\s|$))`), "force-pushes"},
}

var (
	segmentSplit = regexp.MustCompile(`&&|\|\||;|\n`)
	forcePush    = regexp.MustCompile(`\bgit\s+push\b.*(\s--force(\s|$)|\s-f(\s|$)|\s\+\S+)`)
	mainBranch   = regexp.MustCompile(`(\s|:|\+)(main|master)(\s|$)`)
	readsFile    = regexp.MustCompile(`\b(cat|less|more|head|tail|bat|strings|xxd|base64|grep|rg|awk|sed|source|cp|scp)\b`)
)

// checkCommand returns "deny", "ask" or "" plus a reason.
func checkCommand(cmd string) (string, string) {
	for _, seg := range segmentSplit.Split(cmd, -1) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if dangerousRm(seg) {
			return "deny", "recursive force delete of a top-level path"
		}
		if forcePush.MatchString(seg) && mainBranch.MatchString(seg) {
			return "deny", "force push to main/master"
		}
		for _, r := range denyRules {
			if r.re.MatchString(seg) {
				return "deny", r.reason
			}
		}
		if readsFile.MatchString(seg) {
			for _, f := range strings.Fields(seg) {
				if isEnvFile(strings.Trim(f, `"'`)) {
					return "deny", ".env files hold secrets; ask the user for the value you need"
				}
			}
		}
	}
	for _, seg := range segmentSplit.Split(cmd, -1) {
		for _, r := range askRules {
			if r.re.MatchString(seg) {
				return "ask", r.reason
			}
		}
	}
	return "", ""
}

// dangerousRm reports an `rm -rf` aimed at /, ~, $HOME, a top-level
// directory, or a bare wildcard.
func dangerousRm(seg string) bool {
	f := strings.Fields(seg)
	i := 0
	for i < len(f) && (f[i] == "sudo" || strings.Contains(f[i], "=")) {
		i++
	}
	if i >= len(f) || f[i] != "rm" {
		return false
	}
	recursive, force := false, false
	var targets []string
	for _, a := range f[i+1:] {
		switch {
		case a == "--recursive":
			recursive = true
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--"):
			recursive = recursive || strings.ContainsAny(a, "rR")
			force = force || strings.Contains(a, "f")
		default:
			targets = append(targets, strings.Trim(a, `"'`))
		}
	}
	if !recursive || !force {
		return false
	}
	for _, t := range targets {
		switch t {
		case "/", "/*", "~", "~/", "~/*", "$HOME", "$HOME/", "${HOME}", "*", ".", "..", "./*", "../*":
			return true
		}
		if strings.HasPrefix(t, "/") && strings.Count(strings.TrimSuffix(t, "/"), "/") == 1 {
			return true // /usr, /etc, /Users ...
		}
	}
	return false
}

func isEnvFile(p string) bool {
	base := filepath.Base(filepath.ToSlash(p))
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return false
	}
	switch strings.TrimPrefix(base, ".env.") {
	case "example", "sample", "template", "dist", "defaults":
		return false
	}
	return true
}

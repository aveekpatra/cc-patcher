package patches

import (
	"regexp"
	"strings"
	"time"
)

func init() {
	extraPatches = append(extraPatches, &Patch{
		name: "Safe-command auto-approve", hookArg: "autoapprove",
		desc: "Approves read-only shell commands (ls, cat, grep, git status/log/diff...) without a prompt",
		regs: []reg{{"PreToolUse", "Bash", 5, false}},
	})
	handlers["autoapprove"] = autoapproveHook
}

// readOnly lists commands that only read. Each maps to subcommands that are
// allowed, or nil when any arguments are fine.
var readOnly = map[string][]string{
	"ls": nil, "pwd": nil, "cat": nil, "head": nil, "tail": nil, "wc": nil, "echo": nil,
	"grep": nil, "rg": nil, "ag": nil, "tree": nil, "file": nil, "stat": nil, "du": nil, "df": nil,
	"which": nil, "whoami": nil, "date": nil, "uname": nil, "env": nil, "printenv": nil,
	"basename": nil, "dirname": nil, "realpath": nil, "diff": nil, "cmp": nil, "sort": nil,
	"uniq": nil, "cut": nil, "tr": nil, "jq": nil, "less": nil, "true": nil, "id": nil,
	"git":    {"status", "log", "diff", "show", "branch", "remote", "rev-parse", "describe", "blame", "ls-files", "shortlog", "tag"},
	"go":     {"version", "env", "list", "doc"},
	"npm":    {"ls", "view", "outdated"},
	"cargo":  {"tree", "metadata"},
	"gh":     {"status"},
	"docker": {"ps", "images"},
}

var (
	unsafeShell = regexp.MustCompile("[`]|\\$\\(|>|<\\(|\\bsudo\\b")
	splitOps    = regexp.MustCompile(`\|\||&&|;|\||\n`)
)

// autoapproveHook allows a Bash call only if every command in it is on the
// read-only list. Anything else falls through to the normal prompt.
func autoapproveHook(in *Input, _ time.Time) any {
	cmd := in.str("command")
	if cmd == "" || unsafeShell.MatchString(cmd) || strings.Contains(cmd, "&") && !strings.Contains(cmd, "&&") {
		return nil
	}
	if d, _ := checkCommand(cmd); d != "" {
		return nil // the guard's call
	}
	for _, seg := range splitOps.Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) == 0 {
			continue
		}
		subs, ok := readOnly[f[0]]
		if !ok {
			return nil
		}
		if subs != nil && (len(f) < 2 || !contains(subs, f[1])) {
			return nil
		}
		if f[0] == "git" && f[1] == "branch" && len(f) > 2 && !strings.HasPrefix(f[2], "-") {
			return nil // creates a branch
		}
		if f[0] == "git" && (f[1] == "remote" || f[1] == "tag") && len(f) > 2 && !strings.HasPrefix(f[2], "-") {
			return nil // remote add, tag create
		}
		for _, a := range f[1:] {
			if isEnvFile(strings.Trim(a, `"'`)) {
				return nil
			}
		}
	}
	return toolDecision("allow", "cc-patcher: read-only command")
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

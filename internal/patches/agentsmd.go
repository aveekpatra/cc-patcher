package patches

import (
	"os"
	"path/filepath"
	"time"
)

func init() {
	extraPatches = append(extraPatches,
		&Patch{
			name: "AGENTS.md fallback", hookArg: "agentsmd",
			desc: "Loads a project's AGENTS.md when it has no CLAUDE.md (Codex, Cursor and others use it)",
			regs: []reg{{"SessionStart", "", 5, false}},
		},
		&Patch{
			name: "Auto-accept plans", hookArg: "plan",
			desc: "Approves plan mode's 'ready to code?' prompt automatically",
			regs: []reg{{"PermissionRequest", "ExitPlanMode", 5, false}},
		},
	)
	handlers["agentsmd"] = agentsmdHook
	handlers["plan"] = planHook
}

// agentsmdHook hands Claude the project's AGENTS.md when no CLAUDE.md
// would be loaded for it.
func agentsmdHook(in *Input, _ time.Time) any {
	for _, name := range []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md"), "CLAUDE.local.md"} {
		if fileExists(filepath.Join(in.Cwd, name)) {
			return nil
		}
	}
	b, err := os.ReadFile(filepath.Join(in.Cwd, "AGENTS.md"))
	if err != nil || len(b) == 0 {
		return nil
	}
	return addContext("SessionStart", "Project instructions from AGENTS.md (this project has no CLAUDE.md):\n\n"+clip(string(b), 40000))
}

func planHook(in *Input, _ time.Time) any {
	return permissionDecision("allow", "cc-patcher: plans are auto-accepted")
}

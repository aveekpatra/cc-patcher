package patches

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

// compactHook saves a summary and a copy of the transcript before
// compaction, then hands the summary back to Claude once the compacted
// session starts.
func compactHook(in *Input, now time.Time) any {
	dir := filepath.Join(claude.StateDir(), "compact")
	id := unsafeID.ReplaceAllString(in.SessionID, "")
	if id == "" {
		id = "unknown"
	}
	summaryPath := filepath.Join(dir, id+".md")

	switch in.HookEventName {
	case "PreCompact":
		_ = os.MkdirAll(dir, 0o755)
		pruneOld(dir, now)
		backup := filepath.Join(dir, fmt.Sprintf("%s-%s.jsonl", id, now.Format("20060102-150405")))
		_ = copyFile(in.TranscriptPath, backup)
		s := summarizeTranscript(in.TranscriptPath)
		s += "\nFull pre-compaction transcript: " + backup + "\n"
		_ = os.WriteFile(summaryPath, []byte(s), 0o644)
	case "SessionStart":
		if in.Source != "compact" {
			return nil
		}
		b, err := os.ReadFile(summaryPath)
		if err != nil {
			return nil
		}
		return addContext("SessionStart", "[compaction memory] Saved just before compaction:\n"+string(b))
	}
	return nil
}

type transcriptLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// summarizeTranscript pulls out recent user requests, touched files and the
// latest todo list.
func summarizeTranscript(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var prompts, files []string
	seen := map[string]bool{}
	var todos []any
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		var l transcriptLine
		if json.Unmarshal(line, &l) == nil && !l.IsSidechain {
			var text string
			var blocks []contentBlock
			if json.Unmarshal(l.Message.Content, &text) != nil {
				_ = json.Unmarshal(l.Message.Content, &blocks)
			}
			switch l.Type {
			case "user":
				for _, b := range blocks {
					if b.Type == "text" {
						text += b.Text
					}
				}
				if t := strings.TrimSpace(text); t != "" && !strings.HasPrefix(t, "<") {
					prompts = append(prompts, clip(t, 400))
				}
			case "assistant":
				for _, b := range blocks {
					if b.Type != "tool_use" {
						continue
					}
					switch b.Name {
					case "Edit", "Write", "MultiEdit", "NotebookEdit":
						if p, _ := b.Input["file_path"].(string); p != "" && !seen[p] {
							seen[p] = true
							files = append(files, p)
						}
					case "TodoWrite":
						if t, ok := b.Input["todos"].([]any); ok {
							todos = t
						}
					}
				}
			}
		}
		if err == io.EOF || err != nil {
			break
		}
	}

	var s strings.Builder
	if len(prompts) > 5 {
		prompts = prompts[len(prompts)-5:]
	}
	if len(prompts) > 0 {
		s.WriteString("Recent user requests (oldest first):\n")
		for _, p := range prompts {
			s.WriteString("- " + strings.ReplaceAll(p, "\n", " ") + "\n")
		}
	}
	if len(files) > 0 {
		s.WriteString("Files edited this session:\n")
		for _, p := range files {
			s.WriteString("- " + p + "\n")
		}
	}
	if len(todos) > 0 {
		s.WriteString("Last todo list:\n")
		for _, t := range todos {
			m, _ := t.(map[string]any)
			s.WriteString(fmt.Sprintf("- [%v] %v\n", m["status"], m["content"]))
		}
	}
	return s.String()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

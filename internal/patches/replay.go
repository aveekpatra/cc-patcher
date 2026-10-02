package patches

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aveekpatra/cc-patcher/internal/claude"
)

func init() {
	extraPatches = append(extraPatches, &Patch{
		name: "Session replay", hookArg: "replay",
		desc: "Saves every finished session as an HTML replay in ~/.claude/cc-patcher/replays",
		regs: []reg{{"SessionEnd", "", 30, false}},
	})
	handlers["replay"] = replayHook
}

func replayHook(in *Input, now time.Time) any {
	dir := filepath.Join(claude.StateDir(), "replays")
	_ = os.MkdirAll(dir, 0o755)
	id := unsafeID.ReplaceAllString(in.SessionID, "")
	if id == "" {
		id = "unknown"
	}
	name := fmt.Sprintf("%s-%s-%s.html", now.Format("20060102-1504"), filepath.Base(in.Cwd), id[:min(8, len(id))])
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	defer f.Close()
	_ = WriteReplay(in.TranscriptPath, f)
	return nil
}

// WriteReplay renders a transcript JSONL file as a standalone HTML page.
func WriteReplay(transcript string, w io.Writer) error {
	src, err := os.Open(transcript)
	if err != nil {
		return err
	}
	defer src.Close()
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	bw.WriteString(replayHead)

	r := bufio.NewReaderSize(src, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		var l struct {
			Type        string `json:"type"`
			Timestamp   string `json:"timestamp"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &l) == nil && (l.Type == "user" || l.Type == "assistant") {
			writeEntry(bw, l.Type, l.Timestamp, l.IsSidechain, l.Message.Content)
		}
		if err != nil {
			break
		}
	}
	bw.WriteString("</main></body></html>\n")
	return nil
}

func writeEntry(w *bufio.Writer, role, ts string, side bool, content json.RawMessage) {
	var text string
	var blocks []struct {
		Type    string          `json:"type"`
		Text    string          `json:"text"`
		Name    string          `json:"name"`
		Input   json.RawMessage `json:"input"`
		Content json.RawMessage `json:"content"`
		IsError bool            `json:"is_error"`
	}
	if json.Unmarshal(content, &text) != nil {
		_ = json.Unmarshal(content, &blocks)
	}
	var body strings.Builder
	if text != "" {
		body.WriteString("<pre>" + html.EscapeString(text) + "</pre>")
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				body.WriteString("<pre>" + html.EscapeString(b.Text) + "</pre>")
			}
		case "tool_use":
			body.WriteString("<details><summary>tool: " + html.EscapeString(b.Name) + "</summary><pre>" +
				html.EscapeString(clip(string(b.Input), 8000)) + "</pre></details>")
		case "tool_result":
			var s string
			if json.Unmarshal(b.Content, &s) != nil {
				s = string(b.Content)
			}
			cls := "result"
			if b.IsError {
				cls = "result err"
			}
			body.WriteString(`<details class="` + cls + `"><summary>result</summary><pre>` +
				html.EscapeString(clip(s, 8000)) + "</pre></details>")
		}
	}
	if body.Len() == 0 {
		return
	}
	cls := role
	if side {
		cls += " side"
	}
	fmt.Fprintf(w, "<section class=%q><header>%s <time>%s</time></header>%s</section>\n",
		cls, role, html.EscapeString(ts), body.String())
}

const replayHead = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Claude Code session replay</title>
<style>
:root{--bg:#fbfaf7;--fg:#1a1918;--muted:#6b6b6b;--user:#efe9df;--asst:#ffffff;--line:#e2ded6;--err:#c62828}
@media (prefers-color-scheme: dark){:root{--bg:#161514;--fg:#ecebe8;--muted:#9a9a9a;--user:#2a2622;--asst:#1e1d1b;--line:#33302c;--err:#e57373}}
body{background:var(--bg);color:var(--fg);font:14px/1.5 ui-sans-serif,system-ui,sans-serif;margin:0}
main{max-width:860px;margin:0 auto;padding:24px 16px}
section{border:1px solid var(--line);border-radius:8px;padding:10px 14px;margin:10px 0;background:var(--asst)}
section.user{background:var(--user)}section.side{margin-left:24px;opacity:.85}
header{font-weight:600;font-size:12px;text-transform:uppercase;color:var(--muted)}
time{font-weight:400;margin-left:8px}
pre{white-space:pre-wrap;word-break:break-word;font:13px/1.45 ui-monospace,Menlo,monospace;margin:6px 0}
details{margin:4px 0}summary{cursor:pointer;color:var(--muted)}.err summary{color:var(--err)}
</style></head><body><main><h1>Session replay</h1>
`

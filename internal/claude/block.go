package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Managed blocks are sections of a text file (like CLAUDE.md) that
// claude_patcher owns, delimited by HTML comment markers.

func markers(id string) (string, string) {
	return "<!-- claude_patcher:begin " + id + " -->", "<!-- claude_patcher:end " + id + " -->"
}

func HasBlock(path, id string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	begin, _ := markers(id)
	return strings.Contains(string(b), begin)
}

// SetBlock inserts or replaces the block with the given body.
func SetBlock(path, id, body string) error {
	text, err := readText(path)
	if err != nil {
		return err
	}
	text = removeBlock(text, id)
	begin, end := markers(id)
	block := begin + "\n" + strings.TrimSpace(body) + "\n" + end + "\n"
	if text != "" && !strings.HasSuffix(text, "\n\n") {
		text = strings.TrimRight(text, "\n") + "\n\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text+block), 0o644)
}

func RemoveBlock(path, id string) error {
	text, err := readText(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(removeBlock(text, id)), 0o644)
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

func removeBlock(text, id string) string {
	begin, end := markers(id)
	i := strings.Index(text, begin)
	if i < 0 {
		return text
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return text
	}
	rest := strings.TrimLeft(text[i+j+len(end):], "\n")
	head := strings.TrimRight(text[:i], "\n")
	switch {
	case head == "":
		return rest
	case rest == "":
		return head + "\n"
	default:
		return head + "\n\n" + rest
	}
}

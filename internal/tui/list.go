package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// list is a multi-select screen. Checked means "should be on"; enter applies
// the difference between the checked state and what is installed.
type list struct {
	title   string
	items   []Toggle
	current []bool // what is installed right now
	want    []bool // what the user has checked
	cursor  int
	offset  int
	status  []string
}

func newList(title string, items []Toggle) *list {
	l := &list{title: title, items: items}
	l.refresh()
	return l
}

func (l *list) refresh() {
	l.current = make([]bool, len(l.items))
	l.want = make([]bool, len(l.items))
	for i, it := range l.items {
		l.current[i] = it.Enabled()
		l.want[i] = l.current[i]
	}
}

// update handles a key and reports whether the screen should close.
func (l *list) update(k tea.KeyMsg) bool {
	n := len(l.items)
	switch k.String() {
	case "esc", "q", "left", "h":
		return true
	case "up", "k":
		if n > 0 {
			l.cursor = (l.cursor + n - 1) % n
		}
	case "down", "j":
		if n > 0 {
			l.cursor = (l.cursor + 1) % n
		}
	case " ", "x":
		if n > 0 {
			l.want[l.cursor] = !l.want[l.cursor]
		}
	case "a":
		all := true
		for _, w := range l.want {
			all = all && w
		}
		for i := range l.want {
			l.want[i] = !all
		}
	case "enter":
		l.apply()
	}
	return false
}

func (l *list) apply() {
	l.status = nil
	on, off := 0, 0
	for i, it := range l.items {
		if l.want[i] == l.current[i] {
			continue
		}
		var err error
		if l.want[i] {
			err = it.Enable()
		} else {
			err = it.Disable()
		}
		switch {
		case err != nil:
			l.status = append(l.status, badSt.Render(fmt.Sprintf("%s: %v", it.Name(), err)))
		case l.want[i]:
			on++
		default:
			off++
		}
	}
	if on+off == 0 && len(l.status) == 0 {
		l.status = append(l.status, mutedSt.Render("nothing to change"))
	} else if on+off > 0 {
		l.status = append(l.status, goodSt.Render(fmt.Sprintf("enabled %d, disabled %d", on, off)))
	}
	l.refresh()
}

func (l *list) view(width, height int) string {
	var b strings.Builder
	b.WriteString(titleSt.Render(l.title) + "\n\n")
	if len(l.items) == 0 {
		b.WriteString(mutedSt.Render("nothing here yet") + "\n")
	}

	rows := max(height-8-len(l.status), 3)
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	nameW := 0
	for _, it := range l.items {
		nameW = max(nameW, len(it.Name()))
	}
	for i := l.offset; i < len(l.items) && i < l.offset+rows; i++ {
		it := l.items[i]
		box := "[ ]"
		if l.want[i] {
			box = "[x]"
		}
		mark := " "
		if l.want[i] != l.current[i] {
			mark = "*"
		}
		cur := "  "
		name := fmt.Sprintf("%-*s", nameW, it.Name())
		if i == l.cursor {
			cur, box, name = "> ", selSt.Render(box), selSt.Render(name)
		}
		line := cur + box + mark + " " + name
		if room := width - nameW - 10; room > 10 {
			line += "  " + mutedSt.Render(truncate(it.Description(), room))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	for _, s := range l.status {
		b.WriteString(s + "\n")
	}
	b.WriteString(mutedSt.Render("space toggle  a all  enter apply  esc back  (* = pending)"))
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

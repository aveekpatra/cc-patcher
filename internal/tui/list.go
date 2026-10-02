package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// list is a multi-select screen. Checked means "should be on"; enter applies
// the difference between the checked state and what is installed.
type list struct {
	title   string
	items   []Toggle
	current []bool // what is installed right now
	want    []bool // what the user has checked
	cursor  int    // index into visible
	offset  int
	status  []string
	filter  string
	typing  bool  // keys go to the filter
	expand  bool  // show the selected item's full description
	visible []int // indices of items matching filter
}

func newList(title string, items []Toggle) *list {
	l := &list{title: title, items: items, expand: true}
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
	l.applyFilter()
}

// applyFilter keeps items whose name or description contains every word
// of the filter, ignoring case.
func (l *list) applyFilter() {
	words := strings.Fields(strings.ToLower(l.filter))
	l.visible = l.visible[:0]
	for i, it := range l.items {
		text := strings.ToLower(it.Name() + " " + it.Description())
		ok := true
		for _, w := range words {
			ok = ok && strings.Contains(text, w)
		}
		if ok {
			l.visible = append(l.visible, i)
		}
	}
	if l.cursor >= len(l.visible) {
		l.cursor = max(len(l.visible)-1, 0)
	}
}

// launcher is a toggle that, when switched on, runs an interactive
// program in the foreground instead of a plain Enable.
type launcher interface{ Launch() *exec.Cmd }

type launchDone struct{ err error }

// update handles a key, reports whether the screen should close, and may
// return a command to run.
func (l *list) update(k tea.KeyMsg) (bool, tea.Cmd) {
	key := k.String()
	if l.typing {
		switch key {
		case "esc":
			l.typing, l.filter = false, ""
		case "enter", "down", "up", "tab":
			l.typing = false
		case "backspace":
			if r := []rune(l.filter); len(r) > 0 {
				l.filter = string(r[:len(r)-1])
			}
		default:
			if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
				l.filter += string(k.Runes)
			}
		}
		l.applyFilter()
		if key != "down" && key != "up" {
			return false, nil
		}
	}

	n := len(l.visible)
	switch key {
	case "/":
		l.typing = true
	case "i", "tab":
		l.expand = !l.expand
	case "esc":
		if l.filter != "" {
			l.filter = ""
			l.applyFilter()
			return false, nil
		}
		return true, nil
	case "q", "left", "h":
		return true, nil
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
			i := l.visible[l.cursor]
			l.want[i] = !l.want[i]
		}
	case "a":
		all := true
		for _, i := range l.visible {
			all = all && l.want[i]
		}
		for _, i := range l.visible {
			l.want[i] = !all
		}
	case "enter":
		return false, l.apply()
	}
	return false, nil
}

func (l *list) apply() tea.Cmd {
	l.status = nil
	on, off := 0, 0
	var launch *exec.Cmd
	for i, it := range l.items {
		if l.want[i] == l.current[i] {
			continue
		}
		var err error
		if lr, ok := it.(launcher); ok && l.want[i] {
			launch = lr.Launch()
			continue
		}
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
			if n, ok := it.(interface{ Note() string }); ok && n.Note() != "" {
				l.status = append(l.status, n.Note())
			}
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
	if launch != nil {
		return tea.ExecProcess(launch, func(err error) tea.Msg { return launchDone{err} })
	}
	return nil
}

// view draws the list with the filter line on top and status and key
// hints pinned to the bottom, like the home screen.
func (l *list) view(width, height int) string {
	var top []string
	enabled := 0
	for _, c := range l.current {
		if c {
			enabled++
		}
	}
	top = append(top, titleSt.Render(l.title)+mutedSt.Render(fmt.Sprintf("  %d of %d on", enabled, len(l.items))))
	switch {
	case l.typing:
		top = append(top, "/ "+l.filter+selSt.Render("_"))
	case l.filter != "":
		top = append(top, mutedSt.Render("/ "+l.filter+fmt.Sprintf("  (%d match)", len(l.visible))))
	default:
		top = append(top, "")
	}
	top = append(top, "")

	var bottom []string
	bottom = append(bottom, l.status...)
	hints := "space toggle   i details   a all   / filter   enter apply   esc back   * pending"
	if l.typing {
		hints = "type to filter   enter done   esc clear"
	}
	bottom = append(bottom, mutedSt.Render(hints))

	var info []string
	if l.expand && len(l.visible) > 0 {
		info = l.details(l.items[l.visible[l.cursor]], width-6)
	}
	rows := max(height-len(top)-len(bottom)-len(info)-1, 3)
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	if l.offset > max(len(l.visible)-rows, 0) {
		l.offset = max(len(l.visible)-rows, 0)
	}
	nameW := 0
	for _, it := range l.items {
		nameW = max(nameW, len(it.Name()))
	}
	var mid []string
	if len(l.visible) == 0 {
		msg := "nothing here yet"
		if l.filter != "" {
			msg = "no match for " + l.filter
		}
		mid = append(mid, mutedSt.Render(msg))
	}
	for vi := l.offset; vi < len(l.visible) && vi < l.offset+rows; vi++ {
		i := l.visible[vi]
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
		if vi == l.cursor && !l.typing {
			cur, box, name = "> ", selSt.Render(box), selSt.Render(name)
		}
		line := cur + box + mark + " " + name
		if room := width - nameW - 10; room > 10 && !(l.expand && vi == l.cursor) {
			line += "  " + mutedSt.Render(truncate(it.Description(), room))
		}
		mid = append(mid, line)
		if vi == l.cursor && !l.typing {
			mid = append(mid, info...)
		}
	}
	if more := len(l.visible) - (l.offset + rows); more > 0 {
		mid = append(mid, mutedSt.Render(fmt.Sprintf("  ... %d more", more)))
	}

	gap := max(height-len(top)-len(mid)-len(bottom)+1, 1)
	return strings.Join(top, "\n") + "\n" + strings.Join(mid, "\n") +
		strings.Repeat("\n", gap) + strings.Join(bottom, "\n")
}

// details is the expanded block under the selected row: the full
// description wrapped to width, then any extra lines the item offers.
func (l *list) details(it Toggle, width int) []string {
	width = max(width, 20)
	box := lipgloss.NewStyle().Width(width)
	var out []string
	add := func(text string, st lipgloss.Style) {
		for _, line := range strings.Split(box.Render(text), "\n") {
			out = append(out, "      "+st.Render(strings.TrimRight(line, " ")))
		}
	}
	add(it.Description(), lipgloss.NewStyle())
	if d, ok := it.(interface{ Details() string }); ok && d.Details() != "" {
		add(d.Details(), mutedSt)
	}
	return append(out, "")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

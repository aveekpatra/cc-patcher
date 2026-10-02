// Package tui is the interactive front end: splash, main menu, and the
// multi-select screens for skills, config, and patches.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	accent  = lipgloss.AdaptiveColor{Light: "#C15F3C", Dark: "#E08A63"}
	muted   = lipgloss.AdaptiveColor{Light: "#6B6B6B", Dark: "#9A9A9A"}
	good    = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#81C784"}
	bad     = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#E57373"}
	titleSt = lipgloss.NewStyle().Bold(true).Foreground(accent)
	mutedSt = lipgloss.NewStyle().Foreground(muted)
	selSt   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	goodSt  = lipgloss.NewStyle().Foreground(good)
	badSt   = lipgloss.NewStyle().Foreground(bad)
	frame   = lipgloss.NewStyle().Padding(1, 2)
)

const logo = `      _                 _
  ___| | __ _ _   _  __| | ___
 / __| |/ _' | | | |/ _' |/ _ \
| (__| | (_| | |_| | (_| |  __/
 \___|_|\__,_|\__,_|\__,_|\___|
              _       _
  _ __   __ _| |_ ___| |__   ___ _ __
 | '_ \ / _' | __/ __| '_ \ / _ \ '__|
 | |_) | (_| | || (__| | | |  __/ |
 | .__/ \__,_|\__\___|_| |_|\___|_|
 |_|`

// Section is one entry in the main menu.
type Section struct {
	Title   string
	Summary string
	Items   func() []Toggle
}

type screen int

const (
	splashScreen screen = iota
	menuScreen
	listScreen
)

type model struct {
	version  string
	sections []Section
	screen   screen
	cursor   int
	list     *list
	width    int
	height   int
}

type splashDone struct{}

// Run starts the TUI.
func Run(version string, sections []Section) error {
	m := &model{version: version, sections: sections}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *model) Init() tea.Cmd {
	return tea.Tick(1800*time.Millisecond, func(time.Time) tea.Msg { return splashDone{} })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case splashDone:
		if m.screen == splashScreen {
			m.screen = menuScreen
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.screen {
		case splashScreen:
			m.screen = menuScreen
		case menuScreen:
			return m.updateMenu(msg)
		case listScreen:
			if m.list.update(msg) {
				m.screen = menuScreen
			}
		}
	}
	return m, nil
}

func (m *model) updateMenu(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.sections) + 1 // last entry is Quit
	switch k.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		m.cursor = (m.cursor + n - 1) % n
	case "down", "j":
		m.cursor = (m.cursor + 1) % n
	case "enter", " ", "right", "l":
		if m.cursor == len(m.sections) {
			return m, tea.Quit
		}
		s := m.sections[m.cursor]
		m.list = newList(s.Title, s.Items())
		m.screen = listScreen
	}
	return m, nil
}

func (m *model) View() string {
	switch m.screen {
	case splashScreen:
		return m.center(titleSt.Render(logo) + "\n\n" +
			mutedSt.Render("Claude Code skills, config and harness patches  "+m.version) + "\n\n" +
			mutedSt.Render("press any key"))
	case listScreen:
		return frame.Render(m.list.view(m.width-4, m.height-2))
	}
	var b strings.Builder
	b.WriteString(titleSt.Render("claude_patcher") + mutedSt.Render("  "+m.version) + "\n\n")
	for i, s := range m.sections {
		b.WriteString(menuLine(i == m.cursor, s.Title, s.Summary))
	}
	b.WriteString(menuLine(m.cursor == len(m.sections), "Quit", ""))
	b.WriteString("\n" + mutedSt.Render("up/down move  enter open  q quit"))
	return frame.Render(b.String())
}

func menuLine(selected bool, title, summary string) string {
	cur, st := "  ", lipgloss.NewStyle()
	if selected {
		cur, st = "> ", selSt
	}
	line := cur + st.Render(fmt.Sprintf("%-10s", title))
	if summary != "" {
		line += "  " + mutedSt.Render(summary)
	}
	return line + "\n"
}

func (m *model) center(s string) string {
	if m.width == 0 {
		return frame.Render(s)
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, s)
}

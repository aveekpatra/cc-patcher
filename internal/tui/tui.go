// Package tui is the interactive front end: splash, main menu, and the
// multi-select screens for skills, config, and patches.
package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

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
	warnSt  = lipgloss.NewStyle().Foreground(bad).Bold(true)
	frame   = lipgloss.NewStyle().Padding(1, 2)
)

const logo = `      _                 _                        _
  ___| | __ _ _   _  __| | ___     ___ ___   __| | ___
 / __| |/ _' | | | |/ _' |/ _ \   / __/ _ \ / _' |/ _ \
| (__| | (_| | |_| | (_| |  __/  | (_| (_) | (_| |  __/
 \___|_|\__,_|\__,_|\__,_|\___|   \___\___/ \__,_|\___|
                      _       _
          _ __   __ _| |_ ___| |__   ___ _ __
         | '_ \ / _' | __/ __| '_ \ / _ \ '__|
         | |_) | (_| | || (__| | | |  __/ |
         | .__/ \__,_|\__\___|_| |_|\___|_|
         |_|`

// RepoURL is where people star and contribute.
const RepoURL = "https://github.com/aveekpatra/cc-patcher"

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// Section is one entry in the main menu.
type Section struct {
	Title   string
	Summary string
	Items   func() []Toggle
}

type screen int

const (
	homeScreen screen = iota
	listScreen
)

type model struct {
	version  string
	sections []Section
	screen   screen
	cursor   int
	list     *list
	status   string  // one-off message shown in the footer
	input    *string // import path being typed, nil when not prompting
	width    int
	height   int
}

// Run starts the TUI.
func Run(version string, sections []Section) error {
	m := &model{version: version, sections: sections}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case launchDone:
		if a, ok := msg.item.(interface{ AfterLaunch() }); ok {
			a.AfterLaunch()
		}
		if m.list != nil {
			m.list.refresh()
			if msg.err != nil {
				m.list.status = append(m.list.status, badSt.Render(msg.err.Error()))
			}
		}
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.screen == listScreen {
			done, cmd := m.list.update(msg)
			if msg.String() == "enter" {
				saveAuto(m.sections)
			}
			if done {
				m.screen = homeScreen
			}
			return m, cmd
		}
		if m.input != nil {
			return m.updateInput(msg)
		}
		return m.updateHome(msg)
	}
	return m, nil
}

// Home menu: one entry per section, then Export, Import and Quit.
func (m *model) exportIdx() int { return len(m.sections) }
func (m *model) importIdx() int { return len(m.sections) + 1 }
func (m *model) quitIdx() int   { return len(m.sections) + 2 }

func (m *model) updateInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.input, m.status = nil, ""
	case "enter":
		path := strings.TrimSpace(*m.input)
		m.input = nil
		prof, err := LoadProfile(path)
		if err != nil {
			m.status = "import failed: " + err.Error()
			return m, nil
		}
		on, off, report := Apply(m.sections, prof)
		m.status = fmt.Sprintf("imported %s: enabled %d, disabled %d", path, on, off)
		if len(report) > 0 {
			m.status += fmt.Sprintf(", %d problem(s): %s", len(report), strings.Join(report, "; "))
		}
	case "backspace":
		if r := []rune(*m.input); len(r) > 0 {
			*m.input = string(r[:len(r)-1])
		}
	default:
		if k.Type == tea.KeyRunes || k.Type == tea.KeySpace {
			*m.input += string(k.Runes)
		}
	}
	return m, nil
}

func (m *model) export() {
	path := DefaultExportPath()
	if err := Export(m.sections, path, false); err != nil {
		m.status = "export failed: " + err.Error()
		return
	}
	m.status = "exported to " + path
}

func (m *model) startImport() {
	path := DefaultExportPath()
	m.input = &path
	m.status = ""
}

func (m *model) updateHome(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := m.quitIdx() + 1
	key := k.String()
	if i := int(key[0] - '1'); len(key) == 1 && i >= 0 && i < len(m.sections) {
		m.cursor = i
		key = "enter"
	}
	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "e":
		m.cursor = m.exportIdx()
		m.export()
	case "i":
		m.cursor = m.importIdx()
		m.startImport()
	case "g":
		m.status = "opening " + RepoURL
		if err := openBrowser(RepoURL); err != nil {
			m.status = "open " + RepoURL + " in your browser"
		}
	case "up", "k", "shift+tab":
		m.cursor = (m.cursor + n - 1) % n
	case "down", "j", "tab":
		m.cursor = (m.cursor + 1) % n
	case "enter", " ", "right", "l":
		switch m.cursor {
		case m.quitIdx():
			return m, tea.Quit
		case m.exportIdx():
			m.export()
			return m, nil
		case m.importIdx():
			m.startImport()
			return m, nil
		}
		s := m.sections[m.cursor]
		m.list = newList(s.Title, s.Items())
		m.screen = listScreen
	}
	return m, nil
}

func (m *model) View() string {
	if m.screen == listScreen {
		return frame.Render(m.list.view(m.width-4, m.height-2))
	}
	return m.home()
}

// home is the start screen: logo and menu stacked in the middle, key hints
// pinned to the bottom. Its height never changes, so nothing moves.
func (m *model) home() string {
	sumW := 0
	for _, s := range m.sections {
		sumW = max(sumW, len(s.Summary))
	}
	var menu []string
	for i, s := range m.sections {
		menu = append(menu, menuLine(i == m.cursor, s.Title, s.Summary, sumW, fmt.Sprint(i+1)))
	}
	menu = append(menu,
		"",
		menuLine(m.cursor == m.exportIdx(), "Export", "back up your whole setup to a file", sumW, "e"),
		menuLine(m.cursor == m.importIdx(), "Import", "replicate a backup from a file or URL", sumW, "i"),
		menuLine(m.cursor == m.quitIdx(), "Quit", "", sumW, "q"))

	body := lipgloss.JoinVertical(lipgloss.Center,
		titleSt.Render(logo),
		"",
		mutedSt.Render("cc-patcher: skills, config and harness patches for Claude Code"),
		"",
		"",
		lipgloss.JoinVertical(lipgloss.Left, menu...),
	)
	hints := mutedSt.Render("enter open   q quit")
	if m.status != "" {
		hints = mutedSt.Render(truncate(m.status, max(m.width-60, 40)))
	}
	if m.input != nil {
		hints = "import from: " + *m.input + selSt.Render("_") + mutedSt.Render("   enter apply   esc cancel")
	}
	ver := selSt.Render("g") + mutedSt.Render(" GitHub   "+m.version)
	if m.width == 0 {
		return frame.Render(body + "\n\n" + hints)
	}
	right := lipgloss.NewStyle().Width(m.width - 2).Align(lipgloss.Right)
	if gap := m.width - lipgloss.Width(hints) - lipgloss.Width(ver) - 4; gap >= 3 {
		top := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body)
		return top + "\n  " + hints + strings.Repeat(" ", gap) + ver
	}
	top := lipgloss.Place(m.width, m.height-2, lipgloss.Center, lipgloss.Center, body)
	return top + "\n" + right.Render(ver) + "\n  " + hints
}

func menuLine(selected bool, title, summary string, sumW int, key string) string {
	st, sum := lipgloss.NewStyle(), mutedSt
	cur := "  "
	if selected {
		st, cur = selSt, selSt.Render("> ")
	}
	return cur + st.Render(fmt.Sprintf("%-9s", title)) + "  " +
		sum.Render(fmt.Sprintf("%-*s", sumW, summary)) + "   " + mutedSt.Render(key)
}

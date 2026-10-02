package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	choices []string
	cursor  int
}

func initialModel() model {
	return model{choices: []string{"Install config", "Install skills", "Apply harness patches"}}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	s := "claude_patcher\n\n"
	for i, c := range m.choices {
		cur := " "
		if i == m.cursor {
			cur = ">"
		}
		s += fmt.Sprintf("%s %s\n", cur, c)
	}
	return s + "\nq to quit\n"
}

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

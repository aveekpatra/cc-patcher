package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fake struct {
	name string
	on   bool
}

func (f *fake) Name() string        { return f.name }
func (f *fake) Description() string { return "desc of " + f.name }
func (f *fake) Enabled() bool       { return f.on }
func (f *fake) Enable() error       { f.on = true; return nil }
func (f *fake) Disable() error      { f.on = false; return nil }

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestFlow(t *testing.T) {
	a, b := &fake{name: "alpha"}, &fake{name: "beta", on: true}
	m := &model{version: "test", sections: []Section{{Title: "Skills", Items: func() []Toggle { return []Toggle{a, b} }}}}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if !strings.Contains(m.View(), "press any key") {
		t.Fatal("no splash")
	}
	for _, k := range []string{"x", "enter", " ", "j", " ", "enter"} {
		m.Update(key(k))
	}
	v := m.View()
	t.Log("\n" + v)
	if !a.on || b.on {
		t.Fatalf("apply failed: alpha=%v beta=%v", a.on, b.on)
	}
	if !strings.Contains(v, "enabled 1, disabled 1") {
		t.Fatal("missing status")
	}
	m.Update(key("esc"))
	if m.screen != menuScreen {
		t.Fatal("esc did not return to menu")
	}
}

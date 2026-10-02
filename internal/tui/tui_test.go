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
	if !strings.Contains(m.View(), "Skills") || !strings.Contains(m.View(), "|_|") {
		t.Fatal("home lacks logo or menu")
	}
	t.Log("\n" + m.View())
	for _, k := range []string{"enter", " ", "j", " ", "enter"} {
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
	if m.screen != homeScreen {
		t.Fatal("esc did not return to menu")
	}
}

func TestFilter(t *testing.T) {
	var items []Toggle
	for _, n := range []string{"alpha", "beta", "gamma", "alphabet"} {
		items = append(items, &fake{name: n})
	}
	l := newList("Skills", items)
	for _, k := range []string{"/", "a", "l", "p", "enter", "a", "enter"} {
		l.update(key(k))
	}
	v := l.view(80, 20)
	t.Log("\n" + v)
	if len(l.visible) != 2 || !items[0].Enabled() || !items[3].Enabled() || items[1].Enabled() {
		t.Fatalf("filter+all wrong: visible=%v", l.visible)
	}
	if lines := strings.Split(v, "\n"); len(lines) != 20 || !strings.Contains(lines[19], "/ filter") {
		t.Fatalf("hints not pinned to bottom: %d lines", len(lines))
	}
	l.update(key("esc"))
	if len(l.visible) != 4 {
		t.Fatal("esc did not clear filter")
	}
}

func TestDetails(t *testing.T) {
	long := &fake{name: "long"}
	l := newList("Patches", []Toggle{long, &fake{name: "b"}})
	l.update(key("i"))
	v := l.view(50, 20)
	t.Log("\n" + v)
	if !strings.Contains(v, "      desc of long") {
		t.Fatal("details not shown under the row")
	}
	if len(strings.Split(v, "\n")) != 20 {
		t.Fatal("height changed")
	}
}

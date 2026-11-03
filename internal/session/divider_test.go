package session

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDividerBetweenCommands(t *testing.T) {
	var m tea.Model = newModel(Host{Run: func(string) Outcome { return Ran }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	run := func(line string) {
		mm := m.(model)
		mm.input.SetValue(line)
		m, _ = mm.submit()
		m, _ = m.Update(doneMsg{outcome: Ran})
	}
	dividers := func() (n int, last string) {
		for _, l := range strings.Split(ansi.Strip(m.(model).out.View()), "\n") {
			if strings.HasPrefix(l, "──") {
				n++
				last = l
			}
		}
		return n, last
	}

	run("whoami")
	if n, _ := dividers(); n != 0 {
		t.Fatal("no divider before the first command")
	}
	run("tag list")
	n, line := dividers()
	if n != 1 || ansi.StringWidth(line) != 60 || !strings.Contains(line, "s ──") {
		t.Fatalf("want one full-width divider with a time label, got %d %q", n, line)
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	if _, line := dividers(); ansi.StringWidth(line) != 40 {
		t.Fatalf("divider should follow the window width, got %q", line)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	run("note list")
	if n, _ := dividers(); n != 0 {
		t.Fatal("no divider before the first command after a clear")
	}
}

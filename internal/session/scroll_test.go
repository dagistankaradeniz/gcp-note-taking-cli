package session

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPaneScrollsAndFollows(t *testing.T) {
	var m tea.Model = newModel(Host{Intro: "banner"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	for i := range 100 {
		m, _ = m.Update(outputMsg(fmt.Sprintf("line %d\n", i)))
	}
	view := func() string { return m.(model).out.View() }
	if !strings.Contains(view(), "line 99") || !m.(model).follow {
		t.Fatal("pane should follow the newest output")
	}
	if got := rows(m.View().Content); got != 20 {
		t.Fatalf("view is %d rows, want the window's 20", got)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.(model).follow || strings.Contains(view(), "line 99") {
		t.Fatal("PgUp should scroll back and stop following")
	}
	m, _ = m.Update(outputMsg("line 100\n"))
	if strings.Contains(view(), "line 100") {
		t.Fatal("new output must not yank a scrolled-up pane")
	}
	for range 20 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if !m.(model).follow || !strings.Contains(view(), "line 100") {
		t.Fatal("PgDn to the bottom should follow again")
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if strings.Contains(view(), "line 100") {
		t.Fatal("Ctrl+L should clear the pane")
	}
}

func rows(s string) int { return strings.Count(s, "\n") + 1 }

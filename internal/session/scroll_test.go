package session

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPaneScrollsAndFollows(t *testing.T) {
	var m tea.Model = newModel(Host{IntroFrames: []string{"banner"}, Corner: "v9.9.9 · abc1234"})
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
	if first, _, _ := strings.Cut(m.View().Content, "\n"); !strings.Contains(first, "abc1234") || !strings.Contains(ansi.Strip(first), " Quillink ") {
		t.Fatalf("top row should carry the name and version, got %q", first)
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

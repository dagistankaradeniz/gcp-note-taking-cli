package session

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func paneWith(t *testing.T, text string) tea.Model {
	t.Helper()
	var m tea.Model = newModel(Host{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m, _ = m.Update(outputMsg(text))
	return m
}

func click(m tea.Model, x, y int) tea.Model {
	m, _ = m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m, _ = m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	return m
}

func TestDragSelectsAcrossRows(t *testing.T) {
	m := paneWith(t, "\x1b[31mabc123\x1b[0m  first   \nxyz789  second\n")
	m, _ = m.Update(tea.MouseClickMsg{X: 3, Y: 1, Button: tea.MouseLeft})
	m, _ = m.Update(tea.MouseMotionMsg{X: 5, Y: 2, Button: tea.MouseLeft})
	m, cmd := m.Update(tea.MouseReleaseMsg{X: 5, Y: 2, Button: tea.MouseLeft})
	if got, want := m.(model).selectedText(), "123  first\nxyz789"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	if cmd == nil {
		t.Fatal("a finished selection should be copied")
	}
	if !strings.Contains(m.(model).paneView(), "\x1b[7m") {
		t.Fatal("the selection should show")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.(model).pick.on {
		t.Fatal("typing should drop the selection")
	}
}

func TestDragUpwardsSelectsTheSameText(t *testing.T) {
	m := paneWith(t, "abc123\nxyz789\n")
	m, _ = m.Update(tea.MouseClickMsg{X: 2, Y: 2, Button: tea.MouseLeft})
	m, _ = m.Update(tea.MouseMotionMsg{X: 4, Y: 1, Button: tea.MouseLeft})
	if got, want := m.(model).selectedText(), "23\nxyz"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
}

func TestSingleClickSelectsNothing(t *testing.T) {
	m := click(paneWith(t, "abc123\n"), 2, 1)
	if m.(model).pick.on {
		t.Fatal("a click isn't a selection")
	}
}

func TestDoubleClickTakesTheWholeID(t *testing.T) {
	m := paneWith(t, "see a1b2-c3d4_e5.  Title here\n")
	m = click(click(m, 8, 1), 8, 1)
	if got := m.(model).selectedText(); got != "a1b2-c3d4_e5" {
		t.Fatalf("selected %q", got)
	}
}

func TestDoubleClickOnSpaceSelectsNothing(t *testing.T) {
	m := paneWith(t, "abc  def\n")
	m = click(click(m, 3, 1), 3, 1)
	if m.(model).pick.on {
		t.Fatal("no word under the click")
	}
}

func TestTripleClickJoinsAWrappedLine(t *testing.T) {
	long := strings.Repeat("abcde ", 10) // 60 columns in a 40-column pane
	m := paneWith(t, "before\n"+long+"\nafter\n")
	m = click(click(click(m, 1, 3), 1, 3), 1, 3)
	if got, want := m.(model).selectedText(), strings.TrimRight(long, " "); got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
}

func TestSelectionFollowsTheScroll(t *testing.T) {
	var text strings.Builder
	for i := range 50 {
		fmt.Fprintf(&text, "line %d\n", i)
	}
	m := paneWith(t, text.String())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	mm := m.(model)
	want := fmt.Sprintf("line %d", mm.out.YOffset())
	m = click(click(click(m, 0, 1), 0, 1), 0, 1)
	if got := m.(model).selectedText(); got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
}

func TestDragPastThePaneScrolls(t *testing.T) {
	var text strings.Builder
	for i := range 50 {
		fmt.Fprintf(&text, "line %d\n", i)
	}
	m := paneWith(t, text.String())
	first := m.(model)
	before := first.out.YOffset()
	m, _ = m.Update(tea.MouseClickMsg{X: 0, Y: 3, Button: tea.MouseLeft})
	m, _ = m.Update(tea.MouseMotionMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	mm := m.(model)
	if mm.out.YOffset() != before-1 || mm.follow {
		t.Fatalf("offset %d -> %d, follow %v", before, mm.out.YOffset(), mm.follow)
	}
}

func TestFailedCopyAsksTheTerminal(t *testing.T) {
	m := paneWith(t, "abc\n")
	_, ok := m.Update(copiedMsg{text: "abc"})
	if ok == nil {
		t.Fatal("the notice should clear itself")
	}
	m, _ = m.Update(copiedMsg{text: "abc", err: fmt.Errorf("no clipboard")})
	if m.(model).notice != "Sent to the terminal's clipboard" {
		t.Fatalf("notice %q", m.(model).notice)
	}
}

func TestLoaderBecomesTheDivider(t *testing.T) {
	var m tea.Model = newModel(Host{Run: func(string) Outcome { return Ran }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	mm := m.(model)
	mm.input.SetValue("note list")
	m, _ = mm.submit()
	m, _ = m.Update(outputMsg("a note\n"))
	pane := func() string { return ansi.Strip(m.(model).paneView()) }

	if !strings.Contains(pane(), "note list… ") {
		t.Fatalf("want the loader under the output, got %q", pane())
	}
	m, _ = m.Update(doneMsg{outcome: Ran})
	if strings.Contains(pane(), "note list…") || !strings.Contains(pane(), "s ──") {
		t.Fatalf("want the time in the loader's place, got %q", pane())
	}
}

func TestTimeStaysInTheCornerUntilTheEnd(t *testing.T) {
	var m tea.Model = newModel(Host{Run: func(string) Outcome { return Ran }})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	mm := m.(model)
	mm.input.SetValue("note list")
	m, _ = mm.submit()
	for i := range 40 {
		m, _ = m.Update(outputMsg(fmt.Sprintf("line %d\n", i)))
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	lastRow := func() string {
		rows := strings.Split(ansi.Strip(m.(model).paneView()), "\n")
		return rows[len(rows)-1]
	}
	if !strings.Contains(lastRow(), "note list…") {
		t.Fatalf("scrolled up, the loader belongs in the corner, got %q", lastRow())
	}
	m, _ = m.Update(doneMsg{outcome: Ran})
	if r := lastRow(); !strings.HasPrefix(r, "line") || !strings.HasSuffix(r, "s ") {
		t.Fatalf("want a line of output with the time at its right end, got %q", r)
	}
	if strings.Contains(ansi.Strip(m.(model).out.View()), "──") {
		t.Fatal("no divider until the end is reached")
	}
	for range 20 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if r := lastRow(); !strings.HasPrefix(r, "──") || !strings.HasSuffix(r, "s ──") {
		t.Fatalf("want the divider at the end, got %q", r)
	}
}

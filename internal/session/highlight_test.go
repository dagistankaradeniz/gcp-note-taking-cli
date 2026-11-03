package session

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTokenize(t *testing.T) {
	got := tokenize(`note create  "Weekly review" --tags a,b 'open`)
	want := []token{
		{0, 4, "note"}, {5, 11, "create"}, {13, 28, "Weekly review"},
		{29, 35, "--tags"}, {36, 39, "a,b"}, {40, 45, "open"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokenize = %+v\nwant %+v", got, want)
	}
}

func TestHighlightKeepsText(t *testing.T) {
	line := `note create  "Weekly review" --pinned`
	classify := func(w []string) []Kind { return []Kind{Command, Command, Arg, Flag} }
	if got := ansi.Strip(highlight(line, classify)); got != line {
		t.Fatalf("highlight changed the text: %q", got)
	}
}

func TestIntroPlaysThenStays(t *testing.T) {
	var m tea.Model = newModel(Host{IntroFrames: []string{"f1", "f2", "f3"}})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if m.(model).introAt != 0 {
		t.Fatal("should start on the first frame")
	}
	for range 5 {
		m, _ = m.Update(introMsg{})
	}
	if m.(model).introAt != 2 {
		t.Fatalf("should end on the last frame, at %d", m.(model).introAt)
	}

	m = newModel(Host{IntroFrames: []string{"f1", "f2", "f3"}})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.(model).introAt != 2 || m.(model).input.Value() != "x" {
		t.Fatal("a key should finish the banner and still be typed")
	}
}

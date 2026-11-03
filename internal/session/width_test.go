package session

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBoxFitsWidth(t *testing.T) {
	for _, w := range []int{20, 40, 100} {
		box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(w)
		if got := lipgloss.Width(box.Render("hello")); got != w {
			t.Errorf("box width %d rendered %d columns", w, got)
		}
	}
}

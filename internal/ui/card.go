package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Field is one label/value row of a Card.
type Field struct {
	Label string
	Value string
}

// Card is a small rounded panel: a title, then aligned label/value rows.
func Card(title string, fields []Field) string {
	labelW := 0
	for _, f := range fields {
		labelW = max(labelW, ansi.StringWidth(f.Label))
	}
	var b strings.Builder
	b.WriteString(Fg(Brand, title, true))
	// Values too long for the window wrap under themselves, not under
	// the labels (4 = border and padding).
	valueW := max(Width()-4-labelW-2, 16)
	indent := "\n" + strings.Repeat(" ", labelW+2)
	for _, f := range fields {
		pad := strings.Repeat(" ", labelW-ansi.StringWidth(f.Label))
		value := strings.ReplaceAll(ansi.Wrap(f.Value, valueW, " ,"), "\n", indent)
		b.WriteString("\n" + Fg(Muted, f.Label, false) + pad + "  " + value)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Subtle).
		Padding(0, 1)
	return Render(box.Render(b.String())) + "\n"
}

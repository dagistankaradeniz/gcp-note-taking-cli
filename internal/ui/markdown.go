package ui

import (
	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
)

// Markdown renders md for the terminal: headings, lists, quotes and
// highlighted code blocks, wrapped to the window.
func Markdown(md string) (string, error) {
	name := styles.DarkStyle
	if !Dark() {
		name = styles.LightStyle
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(name),
		glamour.WithWordWrap(min(Width(), 120)-2),
	)
	if err != nil {
		return "", err
	}
	out, err := r.Render(md)
	if err != nil {
		return "", err
	}
	return Render(out), nil
}

package ui

import (
	"fmt"
	"os"
)

// Success prints a completed action's message, "✓ Note created." in green
// on a terminal and the plain message everywhere else.
func Success(f *os.File, msg string) {
	if !Rich(f) {
		fmt.Fprintln(f, msg)
		return
	}
	fmt.Fprintln(f, Render(Fg(Green, "✓", true)+" "+msg))
}

// Notice is an informational line, dimmed on a terminal.
func Notice(f *os.File, msg string) {
	if !Rich(f) {
		fmt.Fprintln(f, msg)
		return
	}
	fmt.Fprintln(f, Render(Fg(Muted, "› "+msg, false)))
}

// Failure prints an error: "✗ message" in red with an optional dimmed
// hint on a terminal; the long-standing "Error: message" elsewhere.
func Failure(f *os.File, err error, hint string) {
	if !Rich(f) {
		fmt.Fprintln(f, "Error:", err)
		return
	}
	line := Fg(Red, "✗", true) + " " + Fg(Red, err.Error(), false)
	if hint != "" {
		line += "\n  " + Fg(Muted, hint, false)
	}
	fmt.Fprintln(f, Render(line))
}

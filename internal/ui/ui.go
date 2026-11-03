// Package ui is the CLI's richer terminal look: outcome lines, tables
// fitted to the window, cards and rendered Markdown. Everything here only
// styles output headed for a real terminal (or the interactive session,
// which stands in for one); pipes, files, --json and NO_COLOR get the
// plain text they always had, so scripts never see a change.
package ui

import (
	"bytes"
	"image/color"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"golang.org/x/term"
)

// The palette, matching the banner and the web app: the header yellow for
// the brand, plus status colours readable on dark and light backgrounds.
var (
	Brand   color.Color = lipgloss.Color("#FFC200")
	Green   color.Color = lipgloss.Color("#3FB950")
	Red     color.Color = lipgloss.Color("#F85149")
	Amber   color.Color = lipgloss.Color("#D29922")
	Muted   color.Color = lipgloss.Color("#8B949E")
	Subtle  color.Color = lipgloss.Color("#484F58")
	Accent  color.Color = lipgloss.Color("#58A6FF")
	BrandFg color.Color = lipgloss.Color("#121212")
)

// profile is the real terminal's colour support, read once at start-up,
// before the interactive session swaps stdout for a pipe.
var profile = colorprofile.Detect(os.Stdout, os.Environ())

var (
	mu    sync.Mutex
	width int
	dark  *bool
)

// Rich reports whether styled output may be written to f.
func Rich(f *os.File) bool { return style.Enabled(f) }

// Width is the terminal's width in columns (80 when unknown). The
// interactive session sets it as the window changes, since its stdout is
// a pipe.
func Width() int {
	mu.Lock()
	defer mu.Unlock()
	if width > 0 {
		return width
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

// SetWidth records the terminal width (0 to read it again).
func SetWidth(w int) {
	mu.Lock()
	width = w
	mu.Unlock()
}

// Dark reports whether the terminal background is dark. QUILLINK_THEME
// (dark/light) overrides; otherwise the terminal is asked once, and dark
// is assumed when it doesn't answer.
func Dark() bool {
	mu.Lock()
	defer mu.Unlock()
	if dark == nil {
		d := detectDark()
		dark = &d
	}
	return *dark
}

// SetDark records the background, for the session which asks before it
// takes over the terminal's input.
func SetDark(d bool) {
	mu.Lock()
	dark = &d
	mu.Unlock()
}

func detectDark() bool {
	switch strings.ToLower(os.Getenv("QUILLINK_THEME")) {
	case "light":
		return false
	case "dark":
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return true
	}
	return lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
}

// Render converts styled text to what the terminal supports (fewer
// colours, or none).
func Render(s string) string {
	if profile >= colorprofile.TrueColor {
		return s
	}
	var b bytes.Buffer
	w := &colorprofile.Writer{Forward: &b, Profile: profile}
	_, _ = w.WriteString(s)
	return b.String()
}

// Fg is s in colour c, bold when asked.
func Fg(c color.Color, s string, bold bool) string {
	return lipgloss.NewStyle().Foreground(c).Bold(bold).Render(s)
}

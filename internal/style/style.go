// Package style colours terminal output: the banner, the shell prompt and
// help text. Colour is only ever emitted to an interactive terminal --
// pipes, files and scripts get the plain text unchanged -- and never when
// NO_COLOR is set (https://no-color.org) or TERM is "dumb".
package style

import (
	"os"
	"strings"

	"golang.org/x/term"
)

const reset = "\033[0m"

// Enabled reports whether colour may be written to f.
func Enabled(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if !term.IsTerminal(int(f.Fd())) {
		return false
	}
	return enableVT(f)
}

// Palette wraps text in SGR sequences, or returns it unchanged when off.
type Palette struct{ on bool }

// For returns the palette for writing to f.
func For(f *os.File) Palette { return Palette{on: Enabled(f)} }

// Plain is a palette that never colours.
var Plain = Palette{}

// On reports whether this palette emits colour.
func (p Palette) On() bool { return p.on }

func (p Palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\033[" + code + "m" + s + reset
}

// brand is the web app's header yellow (#FFC200), as truecolor when the
// terminal advertises it, else the nearest xterm-256 colour.
func brand() string {
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return "38;2;255;194;0"
	}
	return "38;5;220"
}

// Brand is the product yellow.
func (p Palette) Brand(s string) string { return p.wrap(brand(), s) }

// BrandBold is the product yellow, bold.
func (p Palette) BrandBold(s string) string { return p.wrap("1;"+brand(), s) }

// Badge is reverse-video product yellow, for the BETA tag.
func (p Palette) Badge(s string) string { return p.wrap("1;7;"+brand(), s) }

// Bold is bold default colour.
func (p Palette) Bold(s string) string { return p.wrap("1", s) }

// Dim is faint default colour.
func (p Palette) Dim(s string) string { return p.wrap("2", s) }

// Grey is a mid grey, for network edges.
func (p Palette) Grey(s string) string { return p.wrap("38;5;245", s) }

// Arg colours positional arguments in help text.
func (p Palette) Arg(s string) string { return p.wrap("36", s) }

// Flag colours flags in help text.
func (p Palette) Flag(s string) string { return p.wrap("32", s) }

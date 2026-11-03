// Package banner draws the Quillink header shown when the CLI starts an
// interactive session: a short node/edge network animation, the block
// wordmark, the slogan, the version line and a row of suggested commands.
// Everything here is pure text generation except Play, which animates.
package banner

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
)

// Width is the full banner width, indent and BETA badge included. Narrower
// terminals get the compact header and no animation.
const Width = 69

const indent = "  "

// Slogan is the first sentence of the web login page's hero line.
const Slogan = "Everything you capture, connected and put to work."

// Block letters, six rows each. QUILL is drawn in the terminal's own
// colour and INK in the product yellow.
var (
	glyphQ = []string{" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║▄▄ ██║", "╚██████╔╝", " ╚══▀▀═╝ "}
	glyphU = []string{"██╗   ██╗", "██║   ██║", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "}
	glyphI = []string{"██╗", "██║", "██║", "██║", "██║", "╚═╝"}
	glyphL = []string{"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"}
	glyphN = []string{"███╗   ██╗", "████╗  ██║", "██╔██╗ ██║", "██║╚██╗██║", "██║ ╚████║", "╚═╝  ╚═══╝"}
	glyphK = []string{"██╗  ██╗", "██║ ██╔╝", "█████╔╝ ", "██╔═██╗ ", "██║  ██╗", "╚═╝  ╚═╝"}
)

// Info is what the version line shows.
type Info struct {
	Version string
	Commit  string
	Built   time.Time
}

// VersionLine is e.g. "v1.4.0 · built 2026-11-02 14:31 UTC · cf66d93";
// unknown parts are left out.
func VersionLine(info Info) string {
	v := info.Version
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		v = "v" + v
	}
	parts := []string{v}
	if !info.Built.IsZero() {
		parts = append(parts, "built "+info.Built.UTC().Format("2006-01-02 15:04 UTC"))
	}
	if info.Commit != "" {
		parts = append(parts, info.Commit)
	}
	return strings.Join(parts, " · ")
}

// Header is the wordmark, slogan and version line, ending in a newline.
// Below Width columns it falls back to a one-line wordmark.
func Header(p style.Palette, info Info, columns int) string {
	var b strings.Builder
	version := VersionLine(info)
	if columns < Width {
		b.WriteString(indent + p.Bold("Quillink") + " " + p.Badge(" BETA ") + "\n")
		b.WriteString(indent + Slogan + "\n\n")
		b.WriteString(indent + p.Dim(version) + "\n")
		return b.String()
	}
	b.WriteString("\n")
	for row := range glyphQ {
		var quill, ink strings.Builder
		for _, g := range [][]string{glyphQ, glyphU, glyphI, glyphL, glyphL} {
			quill.WriteString(g[row])
		}
		for _, g := range [][]string{glyphI, glyphN, glyphK} {
			ink.WriteString(g[row])
		}
		b.WriteString(indent + quill.String() + p.BrandBold(ink.String()))
		if row == len(glyphQ)-1 {
			b.WriteString("  " + p.Badge(" BETA "))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + indent + Slogan + "\n\n\n")
	pad := Width - utf8.RuneCountInString(version)
	if pad < len(indent) {
		pad = len(indent)
	}
	b.WriteString(strings.Repeat(" ", pad) + p.Dim(version) + "\n")
	return b.String()
}

// Suggested is the "Suggested:" row, wrapped to fit columns, ending in a
// newline.
func Suggested(p style.Palette, commands []string, columns int) string {
	const label = "Suggested: "
	sep := " · "
	limit := columns
	var b strings.Builder
	b.WriteString(indent + p.Dim(label))
	used := len(indent) + len(label)
	lead := strings.Repeat(" ", used)
	for i, c := range commands {
		w := utf8.RuneCountInString(c)
		if i > 0 {
			if used+len(sep)+w > limit {
				b.WriteString("\n" + lead)
				used = len(lead)
			} else {
				b.WriteString(p.Dim(sep))
				used += utf8.RuneCountInString(sep)
			}
		}
		b.WriteString(p.Brand(c))
		used += w
	}
	b.WriteString("\n")
	return b.String()
}

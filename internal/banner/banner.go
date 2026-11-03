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

// line is one rendered row and its visible width (escape codes excluded).
type line struct {
	text  string
	width int
}

// Suggestion is one row of the suggested-commands panel.
type Suggestion struct {
	Command, Args, Desc string
}

// The space between the wordmark block and the panel, with a thin
// vertical divider in its middle: as wide as the window allows, within
// these bounds.
const (
	minPanelGap = 10
	maxPanelGap = 16
)

// Header is the wordmark, slogan and version line, ending in a newline.
// Below Width columns it falls back to a one-line wordmark.
func Header(p style.Palette, info Info, columns int) string {
	return Compose(p, info, columns, nil)
}

// Compose is the header with the suggested-commands panel: beside the
// wordmark (neofetch style) when the terminal is wide enough, otherwise
// listed below it.
func Compose(p style.Palette, info Info, columns int, suggestions []Suggestion) string {
	left := headerLines(p, info, columns)
	panel := panelLines(p, suggestions)
	var b strings.Builder
	b.WriteString("\n")
	if len(panel) > 0 && columns >= Width && columns >= Width+minPanelGap+maxWidth(panel) {
		panelGap := min(columns-Width-maxWidth(panel), maxPanelGap)
		half := panelGap / 2
		for i := 0; i < max(len(left), len(panel)); i++ {
			l, r := line{}, line{}
			if i < len(left) {
				l = left[i]
			}
			if i < len(panel) {
				r = panel[i]
			}
			row := l.text + strings.Repeat(" ", Width+half-l.width) + p.Grey("│") + strings.Repeat(" ", panelGap-half-1) + r.text
			b.WriteString(strings.TrimRight(row, " ") + "\n")
		}
		return b.String()
	}
	for _, l := range left {
		b.WriteString(l.text + "\n")
	}
	if len(panel) > 0 {
		b.WriteString("\n")
		for _, l := range panel {
			b.WriteString(indent + l.text + "\n")
		}
	}
	return b.String()
}

// headerLines leaves the version line out when info is empty (the
// session shows it in its top-right corner instead).
func headerLines(p style.Palette, info Info, columns int) []line {
	version := VersionLine(info)
	vw := utf8.RuneCountInString(version)
	if columns < Width {
		out := []line{
			{indent + p.Bold("Quillink") + " " + p.Badge(" BETA "), len(indent) + 15},
			{indent + Slogan, len(indent) + utf8.RuneCountInString(Slogan)},
		}
		if version == "" {
			return out
		}
		return append(out, line{}, line{indent + p.Dim(version), len(indent) + vw})
	}
	var out []line
	for row := range glyphQ {
		var quill, ink strings.Builder
		for _, g := range [][]string{glyphQ, glyphU, glyphI, glyphL, glyphL} {
			quill.WriteString(g[row])
		}
		for _, g := range [][]string{glyphI, glyphN, glyphK} {
			ink.WriteString(g[row])
		}
		text := indent + quill.String() + p.BrandBold(ink.String())
		w := len(indent) + utf8.RuneCountInString(quill.String()+ink.String())
		if row == len(glyphQ)-1 {
			text += "  " + p.Badge(" BETA ")
			w += 8
		}
		out = append(out, line{text, w})
	}
	out = append(out, line{}, line{indent + Slogan, len(indent) + utf8.RuneCountInString(Slogan)})
	if version == "" {
		return out
	}
	pad := max(Width-vw, len(indent))
	return append(out, line{}, line{}, line{strings.Repeat(" ", pad) + p.Dim(version), pad + vw})
}

// panelLines renders the bold "Suggested" heading and one row per
// command: command yellow, arguments cyan, description dim, aligned.
func panelLines(p style.Palette, suggestions []Suggestion) []line {
	if len(suggestions) == 0 {
		return nil
	}
	col := 0
	for _, s := range suggestions {
		col = max(col, utf8.RuneCountInString(strings.TrimSpace(s.Command+" "+s.Args)))
	}
	col += 2
	out := []line{{p.Bold("Suggested"), 9}}
	for _, s := range suggestions {
		plain := strings.TrimSpace(s.Command + " " + s.Args)
		text := p.Brand(s.Command)
		if s.Args != "" {
			text += " " + p.Arg(s.Args)
		}
		pad := col - utf8.RuneCountInString(plain)
		text += strings.Repeat(" ", pad) + p.Dim(s.Desc)
		out = append(out, line{text, col + utf8.RuneCountInString(s.Desc)})
	}
	return out
}

func maxWidth(lines []line) int {
	w := 0
	for _, l := range lines {
		w = max(w, l.width)
	}
	return w
}

// Package pager shows long command output inside the full-screen
// interactive session, where the terminal keeps no scrollback: arrows,
// PgUp/PgDn, space, g/G to move, q or Esc to go back to the prompt.
package pager

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
)

// Wrap splits text into screen rows of at most width visible characters.
// Colour escape sequences don't count toward the width, and the colour
// active at a break is carried onto the next row.
func Wrap(text string, width int) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	if width < 1 {
		width = 1
	}
	var rows []string
	for _, l := range strings.Split(text, "\n") {
		rows = append(rows, wrapLine(strings.TrimRight(l, "\r"), width)...)
	}
	return rows
}

func wrapLine(l string, width int) []string {
	var (
		rows   []string
		cur    strings.Builder
		n      int
		active string // last SGR sequence still in effect
	)
	for i := 0; i < len(l); {
		if l[i] == 0x1b {
			j := i + 1
			if j < len(l) && l[j] == '[' {
				j++
				for j < len(l) && (l[j] < 0x40 || l[j] > 0x7e) {
					j++
				}
				j++
			}
			j = min(j, len(l))
			seq := l[i:j]
			cur.WriteString(seq)
			if strings.HasSuffix(seq, "m") {
				if seq == "\x1b[0m" || seq == "\x1b[m" {
					active = ""
				} else {
					active = seq
				}
			}
			i = j
			continue
		}
		r, size := utf8.DecodeRuneInString(l[i:])
		if n == width {
			if active != "" {
				cur.WriteString("\x1b[0m")
			}
			rows = append(rows, cur.String())
			cur.Reset()
			cur.WriteString(active)
			n = 0
		}
		cur.WriteRune(r)
		n++
		i += size
	}
	return append(rows, cur.String())
}

// Keys a terminal sends in raw mode.
const (
	keyQuit = iota + 1
	keyUp
	keyDown
	keyPageUp
	keyPageDown
	keyHome
	keyEnd
)

func parseKey(b []byte) int {
	s := string(b)
	switch s {
	case "q", "Q", "\x1b", "\x03":
		return keyQuit
	case "\x1b[A", "\x1bOA", "k":
		return keyUp
	case "\x1b[B", "\x1bOB", "j", "\r", "\n":
		return keyDown
	case "\x1b[5~", "b":
		return keyPageUp
	case "\x1b[6~", " ", "f":
		return keyPageDown
	case "\x1b[H", "\x1b[1~", "g":
		return keyHome
	case "\x1b[F", "\x1b[4~", "G":
		return keyEnd
	}
	return 0
}

// Show pages rows on out (a terminal in raw mode), reading keys from in,
// until the user quits. height is the terminal height; the last row is a
// status line.
func Show(in io.Reader, out io.Writer, p style.Palette, rows []string, height int) error {
	view := max(height-1, 1)
	top := 0
	last := max(len(rows)-view, 0)
	buf := make([]byte, 16)
	for {
		var b strings.Builder
		b.WriteString("\x1b[H\x1b[2J")
		for i := top; i < top+view && i < len(rows); i++ {
			b.WriteString(rows[i] + "\x1b[0m\r\n")
		}
		status := fmt.Sprintf(" lines %d–%d of %d · ↑↓ PgUp PgDn space · q to return ",
			top+1, min(top+view, len(rows)), len(rows))
		fmt.Fprintf(&b, "\x1b[%d;1H%s", height, p.Badge(status))
		if _, err := io.WriteString(out, b.String()); err != nil {
			return err
		}
		n, err := in.Read(buf)
		if err != nil {
			return err
		}
		switch parseKey(buf[:n]) {
		case keyQuit:
			_, err := io.WriteString(out, "\x1b[H\x1b[2J")
			return err
		case keyUp:
			top = max(top-1, 0)
		case keyDown:
			top = min(top+1, last)
		case keyPageUp:
			top = max(top-view, 0)
		case keyPageDown:
			top = min(top+view, last)
		case keyHome:
			top = 0
		case keyEnd:
			top = last
		}
	}
}

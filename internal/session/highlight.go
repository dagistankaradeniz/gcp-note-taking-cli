package session

import (
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// Kind is what a word of the input line is, for colouring it.
type Kind int

const (
	// Plain: no colour.
	Plain Kind = iota
	// Command: a command or subcommand name, or a session built-in.
	Command
	// Flag: --name or -n.
	Flag
	// Arg: an argument or flag value.
	Arg
	// Unknown: not a command where one was expected.
	Unknown
)

// token is one word of a line: its byte range, including any quotes, and
// its text with the quotes removed.
type token struct {
	start, end int
	text       string
}

// tokenize splits a line into words the way the shell does: on spaces,
// with '...' and "..." kept together. An unclosed quote runs to the end.
func tokenize(line string) []token {
	var out []token
	i := 0
	for i < len(line) {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i >= len(line) {
			break
		}
		t := token{start: i}
		var b strings.Builder
		for i < len(line) && line[i] != ' ' {
			if q := line[i]; q == '"' || q == '\'' {
				i++
				for i < len(line) && line[i] != q {
					b.WriteByte(line[i])
					i++
				}
				if i < len(line) {
					i++
				}
				continue
			}
			b.WriteByte(line[i])
			i++
		}
		t.end, t.text = i, b.String()
		out = append(out, t)
	}
	return out
}

// highlight colours a line by the kinds classify gives its words.
func highlight(line string, classify func(words []string) []Kind) string {
	toks := tokenize(line)
	if len(toks) == 0 || classify == nil {
		return line
	}
	words := make([]string, len(toks))
	for i, t := range toks {
		words[i] = t.text
	}
	kinds := classify(words)
	var b strings.Builder
	at := 0
	for i, t := range toks {
		b.WriteString(line[at:t.start])
		k := Plain
		if i < len(kinds) {
			k = kinds[i]
		}
		b.WriteString(paint(k, line[t.start:t.end]))
		at = t.end
	}
	b.WriteString(line[at:])
	return b.String()
}

func paint(k Kind, s string) string {
	switch k {
	case Command:
		return ui.Fg(ui.Brand, s, false)
	case Flag:
		return ui.Fg(ui.Green, s, false)
	case Arg:
		return ui.Fg(ui.Cyan, s, false)
	case Unknown:
		return ui.Fg(ui.Red, s, false)
	}
	return s
}

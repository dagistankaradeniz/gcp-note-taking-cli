// Package shell holds the pure parts of the interactive session: turning
// a typed line into arguments and completing command names.
package shell

import (
	"errors"
	"strings"
)

// ErrUnterminatedQuote is returned for a line with an open quote.
var ErrUnterminatedQuote = errors.New("unterminated quote")

// Split breaks a line into arguments the way a POSIX shell would for the
// simple cases: whitespace separates, single quotes are literal, double
// quotes allow \" and \\, and a backslash outside quotes escapes the next
// character. No variables, globbing or pipes.
func Split(line string) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inArg   bool
		quote   rune
		escaped bool
	)
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inArg = true, true
		case r == '\'' || r == '"':
			quote, inArg = r, true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 || escaped {
		return nil, ErrUnterminatedQuote
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

// HasSecretFlag reports whether args pass a credential as a literal flag
// value (--token). Such lines are kept out of the session history.
func HasSecretFlag(args []string) bool {
	for _, a := range args {
		if a == "--token" || strings.HasPrefix(a, "--token=") {
			return true
		}
	}
	return false
}

// Complete returns the completion of word among candidates: the single
// match plus a trailing space, or the longest common prefix of several.
// ok is false when nothing extends the word.
func Complete(word string, candidates []string) (string, bool) {
	var matches []string
	for _, c := range candidates {
		if strings.HasPrefix(c, word) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return matches[0] + " ", true
	}
	prefix := matches[0]
	for _, m := range matches[1:] {
		for !strings.HasPrefix(m, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	if len(prefix) <= len(word) {
		return "", false
	}
	return prefix, true
}

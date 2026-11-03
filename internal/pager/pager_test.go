package pager

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
)

func TestWrapPlain(t *testing.T) {
	rows := Wrap("abcdefgh\nxy\n", 3)
	want := []string{"abc", "def", "gh", "xy"}
	if strings.Join(rows, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q want %q", rows, want)
	}
	if Wrap("", 10) != nil {
		t.Fatal("empty text should have no rows")
	}
}

func TestWrapKeepsColourAcrossBreak(t *testing.T) {
	rows := Wrap("\x1b[32mabcdef\x1b[0m", 3)
	if len(rows) != 2 {
		t.Fatalf("escape codes counted toward width: %q", rows)
	}
	if !strings.HasPrefix(rows[1], "\x1b[32m") || !strings.HasSuffix(rows[0], "\x1b[0m") {
		t.Errorf("colour not carried across the break: %q", rows)
	}
}

func TestShowScrollsAndQuits(t *testing.T) {
	var rows []string
	for i := range 10 {
		rows = append(rows, strings.Repeat("x", i+1))
	}
	in := strings.NewReader("") // EOF ends the loop
	var out bytes.Buffer
	if err := Show(in, &out, style.Plain, rows, 5); err == nil {
		t.Fatal("expected EOF")
	}
	if !strings.Contains(out.String(), "lines 1–4 of 10") {
		t.Errorf("status line missing: %q", out.String())
	}
	for _, k := range []string{"q", "\x1b"} {
		out.Reset()
		if err := Show(strings.NewReader(k), &out, style.Plain, rows, 5); err != nil {
			t.Errorf("quit key %q: %v", k, err)
		}
	}
}

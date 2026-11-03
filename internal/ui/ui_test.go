package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestTableFitsAndShrinksTitlesFirst(t *testing.T) {
	headers := []string{"ID", "TITLE", "UPDATED"}
	rows := [][]string{{"abc123", strings.Repeat("long title ", 10), "2026-01-01T00:00:00Z"}}
	out := Table(headers, rows, 50)
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := ansi.StringWidth(line); w > 50 {
			t.Errorf("line %q is %d columns, want <= 50", ansi.Strip(line), w)
		}
	}
	if !strings.Contains(ansi.Strip(out), "abc123") {
		t.Error("the id column was cut")
	}
	if !strings.Contains(ansi.Strip(out), "…") {
		t.Error("the long title wasn't shortened")
	}
}

func TestRelative(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"2026-11-03T11:59:30Z": "just now",
		"2026-11-03T11:15:00Z": "45m ago",
		"2026-11-03T07:00:00Z": "5h ago",
		"2026-10-31T12:00:00Z": "3d ago",
		"not a date":           "not a date",
	}
	for in, want := range cases {
		if got := relative(in, now); got != want {
			t.Errorf("relative(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCardWrapsWithinWidth(t *testing.T) {
	SetWidth(40)
	defer SetWidth(0)
	out := Card("Signed in", []Field{{Label: "Access", Value: strings.Repeat("notes:read, ", 12)}})
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := ansi.StringWidth(line); w > 40 {
			t.Errorf("card line %d columns wide: %q", w, ansi.Strip(line))
		}
	}
}

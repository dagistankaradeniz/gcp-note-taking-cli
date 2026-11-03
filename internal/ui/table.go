package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// minCol is the narrowest a shrunk column gets.
const minCol = 6

// Table lays rows out to fit width: a bold header, a thin rule, and when
// the table is too wide the widest columns are cut short with "…". Cells
// may already carry colour; widths are measured without it.
func Table(headers []string, rows [][]string, width int) string {
	cols := len(headers)
	widths := make([]int, cols)
	measure := func(cells []string) {
		for i := 0; i < cols && i < len(cells); i++ {
			if w := ansi.StringWidth(cells[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}
	now := time.Now()
	shown := make([][]string, len(rows))
	for r, row := range rows {
		shown[r] = append([]string(nil), row...)
		for i := 0; i < cols && i < len(row); i++ {
			if dateColumns[headers[i]] {
				shown[r][i] = relative(row[i], now)
			}
		}
	}
	rows = shown
	measure(headers)
	for _, r := range rows {
		measure(r)
	}
	fit(headers, widths, width-2*(cols-1))

	var b strings.Builder
	line := func(cells []string, head bool) {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			cell = ansi.Truncate(cell, widths[i], "…")
			switch {
			case head:
				cell = Fg(Muted, strings.ToUpper(ansi.Strip(cell)), true)
			case headers[i] == "ID":
				// Ids are for copying, not reading: kept quiet.
				cell = Fg(Muted, cell, false)
			}
			if i < cols-1 {
				cell += strings.Repeat(" ", widths[i]-ansi.StringWidth(cell)+2)
			}
			b.WriteString(cell)
		}
		b.WriteString("\n")
	}
	line(headers, true)
	total := 2 * (cols - 1)
	for _, w := range widths {
		total += w
	}
	b.WriteString(Fg(Subtle, strings.Repeat("─", total), false) + "\n")
	for _, r := range rows {
		line(r, false)
	}
	return Render(b.String())
}

// flexible columns hold free text and give up width first; ids, dates
// and counts keep theirs as long as possible.
var flexible = map[string]bool{
	"TITLE": true, "NAME": true, "SNIPPET": true, "FILENAME": true,
	"SHARED TAGS": true, "API BASE": true, "EMAIL": true, "FOLDER": true,
}

// dateColumns are shown as "3d ago" / a date instead of a raw timestamp.
var dateColumns = map[string]bool{"UPDATED": true, "SHARED AT": true, "CREATED": true}

// fit narrows columns a column at a time, widest first, flexible ones
// before the rest, until they fit avail or nothing more can give.
func fit(headers []string, widths []int, avail int) {
	for _, pass := range []bool{true, false} {
		for {
			total, widest := 0, -1
			for i, w := range widths {
				total += w
				if (flexible[headers[i]] || !pass) && w > minCol && (widest < 0 || w > widths[widest]) {
					widest = i
				}
			}
			if total <= avail || widest < 0 {
				break
			}
			widths[widest]--
		}
	}
}

// Relative is relative for now.
func Relative(s string) string { return relative(s, time.Now()) }

// relative turns an RFC 3339 timestamp into "just now", "5m ago", "3h
// ago", "4d ago" or a date; anything else is left alone.
func relative(s string, now time.Time) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	d := now.Sub(t)
	switch {
	case d < 0:
		return t.Local().Format("2006-01-02")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("2006-01-02")
}

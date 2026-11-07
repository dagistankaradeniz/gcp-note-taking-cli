package session

import (
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/browser"
)

// A click on a link in the output opens it. Only what's shown is opened,
// so what you read is where you go, and browser.Open takes http(s) only.

var linkPattern = regexp.MustCompile(`https?://[^\s<>"'│…]+`)

// openLink opens a link; tests replace it.
var openLink = defaultOpenLink

func defaultOpenLink(url string) error { return browser.Open(url) }

// openMsg opens url unless the click it came from turned into a
// double-click: that's a copy.
type openMsg struct {
	url string
	at  time.Time
}

type openedMsg struct{ err error }

// links finds the links in plain text as byte ranges. A link the table
// shortened with "…" is left out: it would go to the wrong place.
func links(text string) [][]int {
	var out [][]int
	for _, l := range linkPattern.FindAllStringIndex(text, -1) {
		end := l[1]
		if strings.HasPrefix(text[end:], "…") {
			continue
		}
		for end > l[0] && strings.ContainsRune(".,;:!?)]}", rune(text[end-1])) {
			end--
		}
		out = append(out, []int{l[0], end})
	}
	return out
}

// linkAt is the link under a cell, following it over wrapped rows.
func (m model) linkAt(p point) string {
	first, last := p.row, p.row
	for first > 0 && m.cont[first] {
		first--
	}
	for last+1 < len(m.rows) && m.cont[last+1] {
		last++
	}
	var text strings.Builder
	col := p.col
	for r := first; r <= last; r++ {
		plain := ansi.Strip(m.rows[r])
		if r < p.row {
			col += ansi.StringWidth(plain)
		}
		text.WriteString(plain)
	}
	s := text.String()
	for _, l := range links(s) {
		if from := ansi.StringWidth(s[:l[0]]); col >= from && col < from+ansi.StringWidth(s[l[0]:l[1]]) {
			return s[l[0]:l[1]]
		}
	}
	return ""
}

func (m model) opened(msg openMsg) (tea.Model, tea.Cmd) {
	if m.clicks != 1 || !msg.at.Equal(m.clickTime) {
		return m, nil
	}
	if !browser.Available() {
		cmd := m.say("No browser here. Double-click to copy the link")
		return m, cmd
	}
	url := msg.url
	cmd := m.say("Opening link")
	return m, tea.Batch(cmd, func() tea.Msg { return openedMsg{openLink(url)} })
}

// markLinks underlines the links on the visible rows.
func markLinks(rows []string) {
	for i, row := range rows {
		plain := ansi.Strip(row)
		if !strings.Contains(plain, "http") {
			continue
		}
		found := links(plain)
		for j := len(found) - 1; j >= 0; j-- {
			from := ansi.StringWidth(plain[:found[j][0]])
			rows[i] = mark(rows[i], from, from+ansi.StringWidth(plain[found[j][0]:found[j][1]]), underline)
		}
	}
}

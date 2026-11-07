package session

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// Search through the output: `/` on an empty input, or Ctrl+F. It starts
// at the newest match; Enter or ↑ goes to older ones, ↓ to newer, Esc
// leaves.

// match is a hit in a row, columns start to end (excluded).
type match struct{ row, start, end int }

func (m model) openFind() (tea.Model, tea.Cmd) {
	m.finding = true
	m.sugs = nil
	m.find.Reset()
	m.find.Prompt = promptStyle().Render("/ ")
	m.find.Placeholder = "find in output"
	m.input.Blur()
	return m, m.find.Focus()
}

func (m model) closeFind() (tea.Model, tea.Cmd) {
	m.finding = false
	m.matches = nil
	m.find.Blur()
	m.follow = m.out.AtBottom()
	return m, m.input.Focus()
}

func (m model) findKey(msg tea.KeyPressMsg, k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "ctrl+c", "ctrl+f":
		return m.closeFind()
	case "enter", "up":
		m.step(-1)
		return m, nil
	case "down":
		m.step(1)
		return m, nil
	case "pgup", "pgdown":
		m.out, _ = m.out.Update(msg)
		m.follow = m.out.AtBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.find, cmd = m.find.Update(msg)
	m.search()
	m.at = len(m.matches) - 1
	m.reveal()
	return m, cmd
}

// search fills matches from the current rows, keeping the place in them.
func (m *model) search() {
	m.matches = findRows(m.rows, m.find.Value())
	m.at = min(max(m.at, 0), max(len(m.matches)-1, 0))
}

func (m *model) step(by int) {
	if n := len(m.matches); n > 0 {
		m.at = (m.at + by + n) % n
		m.reveal()
	}
}

// reveal scrolls the current match into view, about mid-pane.
func (m *model) reveal() {
	if len(m.matches) == 0 {
		return
	}
	row, top, h := m.matches[m.at].row, m.out.YOffset(), m.out.Height()
	if row < top+1 || row >= top+h {
		m.out.SetYOffset(row - h/2)
	}
	m.follow = m.out.AtBottom()
}

// findRows finds query in rows, ignoring case. Runes are folded one for
// one, so a position in the folded text is one in the original too.
func findRows(rows []string, query string) []match {
	q := fold(query)
	if len(q) == 0 {
		return nil
	}
	var out []match
	for r, row := range rows {
		plain := []rune(ansi.Strip(row))
		text := fold(string(plain))
		for i := 0; i+len(q) <= len(text); i++ {
			if !equal(text[i:i+len(q)], q) {
				continue
			}
			start := ansi.StringWidth(string(plain[:i]))
			out = append(out, match{r, start, start + ansi.StringWidth(string(plain[i:i+len(q)]))})
			i += len(q) - 1
		}
	}
	return out
}

func fold(s string) []rune {
	out := []rune(s)
	for i, r := range out {
		out[i] = unicode.ToLower(r)
	}
	return out
}

func equal(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// markMatches draws the matches on the visible rows.
func (m model) markMatches(rows []string, top int) {
	current, _, _ := strings.Cut(ui.Render(lipgloss.NewStyle().Background(ui.Brand).Foreground(ui.BrandFg).Render("\x00")), "\x00")
	// Backwards, so marking one match doesn't move the next.
	for i := len(m.matches) - 1; i >= 0; i-- {
		h := m.matches[i]
		if at := h.row - top; at >= 0 && at < len(rows) {
			seq := reverse
			if i == m.at {
				seq = current + reverse
			}
			rows[at] = mark(rows[at], h.start, h.end, seq)
		}
	}
}

// findCorner is the "3 of 12" shown while searching.
func (m model) findCorner() string {
	switch {
	case m.find.Value() == "":
		return ""
	case len(m.matches) == 0:
		return ui.Fg(ui.Red, "no matches", false)
	}
	return ui.Fg(ui.Muted, fmt.Sprintf("%d of %d", m.at+1, len(m.matches)), false)
}

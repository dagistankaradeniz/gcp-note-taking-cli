package session

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/clipboard"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// The session keeps the mouse so the wheel can scroll the pane, which
// takes away the terminal's own selection. So the pane selects for
// itself: drag to select, double-click for a word (an id), triple-click
// for a line. A selection is copied as soon as it's made.

const (
	multiClick = 400 * time.Millisecond
	noticeFor  = 2 * time.Second
	// wordPunct can sit inside a word: ids, emails, paths, links.
	wordPunct = "-_.@/:~#%+=?&"
)

// point is a cell of the pane's content: a row of model.rows and a column.
type point struct{ row, col int }

func (p point) before(o point) bool {
	return p.row < o.row || p.row == o.row && p.col < o.col
}

// selection runs from anchor to head, both included, in either direction.
type selection struct {
	anchor, head point
	on           bool // there's something selected
	dragging     bool
}

func (s selection) ordered() (from, to point) {
	if s.head.before(s.anchor) {
		return s.head, s.anchor
	}
	return s.anchor, s.head
}

// cols is the part of a row of width w that's selected, end excluded.
func (s selection) cols(row, w int) (start, end int, ok bool) {
	from, to := s.ordered()
	if !s.on || row < from.row || row > to.row {
		return 0, 0, false
	}
	end = w
	if row == from.row {
		start = from.col
	}
	if row == to.row {
		end = min(to.col+1, w)
	}
	return start, end, start < end
}

type copiedMsg struct {
	text string
	err  error
}

// wrapRows breaks lines to the pane's width. cont marks a row that
// carries on the line before it, so copying can join them up again; heads
// are the rows where a typed command starts.
func wrapRows(lines []string, w int) (rows []string, cont []bool, heads []int) {
	for _, l := range lines {
		l = strings.ReplaceAll(l, "\r", "") // the viewport would split on it
		if rest, ok := strings.CutPrefix(l, typed); ok {
			l = rest
			heads = append(heads, len(rows))
		}
		lw := ansi.StringWidth(l)
		if lw <= w {
			rows, cont = append(rows, l), append(cont, false)
			continue
		}
		for c := 0; c < lw; c += w {
			rows, cont = append(rows, ansi.Cut(l, c, c+w)), append(cont, c > 0)
		}
	}
	return rows, cont, heads
}

// cell is the content cell under a window position, clamped to the pane
// when inPane is false; ok is false when there's no content there.
func (m model) cell(x, y int, inPane bool) (point, bool) {
	top, height := 1, m.out.Height()
	if inPane && (y < top || y >= top+height) {
		return point{}, false
	}
	row := m.out.YOffset() + min(max(y-top, 0), height-1)
	if inPane && (row >= len(m.rows) || y == top && m.pinned() >= 0) {
		return point{}, false
	}
	if len(m.rows) == 0 {
		return point{}, false
	}
	return point{min(row, len(m.rows)-1), max(x, 0)}, true
}

func (m model) press(e tea.Mouse) (tea.Model, tea.Cmd) {
	if e.Button != tea.MouseLeft {
		return m, nil
	}
	m.pick = selection{}
	p, ok := m.cell(e.X, e.Y, true)
	if !ok {
		m.clicks = 0
		return m, nil
	}
	now := time.Now()
	if p == m.clickAt && now.Sub(m.clickTime) < multiClick {
		m.clicks = m.clicks%3 + 1
	} else {
		m.clicks = 1
	}
	m.clickAt, m.clickTime = p, now
	switch m.clicks {
	case 2:
		if from, to, ok := wordAt(m.rows[p.row], p.col); ok {
			m.pick = selection{anchor: point{p.row, from}, head: point{p.row, to}, on: true}
			return m, m.copySelection()
		}
	case 3:
		if s, ok := m.lineAt(p.row); ok {
			m.pick = s
			return m, m.copySelection()
		}
	default:
		m.pick = selection{anchor: p, head: p, dragging: true}
	}
	return m, nil
}

// drag extends the selection, scrolling when it reaches past the pane.
func (m model) drag(e tea.Mouse) (tea.Model, tea.Cmd) {
	if !m.pick.dragging {
		return m, nil
	}
	switch {
	case e.Y < 1:
		m.out.ScrollUp(1)
	case e.Y > m.out.Height():
		m.out.ScrollDown(1)
	}
	m.follow = m.out.AtBottom()
	if p, ok := m.cell(e.X, e.Y, false); ok {
		m.pick.head = p
		m.pick.on = p != m.pick.anchor
	}
	return m, nil
}

func (m model) release() (tea.Model, tea.Cmd) {
	if !m.pick.dragging {
		return m, nil
	}
	m.pick.dragging = false
	if m.pick.on {
		return m, m.copySelection()
	}
	if url := m.linkAt(m.pick.anchor); url != "" && m.clicks == 1 {
		at := m.clickTime
		return m, tea.Tick(multiClick, func(time.Time) tea.Msg { return openMsg{url, at} })
	}
	return m, nil
}

func (m model) copySelection() tea.Cmd {
	text := m.selectedText()
	if text == "" {
		return nil
	}
	return func() tea.Msg { return copiedMsg{text: text, err: clipboard.Write(text)} }
}

// copied reports the copy. Where the system clipboard can't be reached
// the terminal is asked instead (OSC 52); only then, as some terminals
// prompt the user about it.
func (m model) copied(msg copiedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		cmd := m.say("Sent to the terminal's clipboard")
		return m, tea.Batch(tea.SetClipboard(msg.text), cmd)
	}
	cmd := m.say("Copied")
	return m, cmd
}

// say shows a notice in the pane's corner for a moment. Call it before
// the return statement: `return m, m.say(…)` may copy m first.
func (m *model) say(text string) tea.Cmd {
	m.notice, m.noticeAt = text, time.Now()
	return tea.Tick(noticeFor, func(time.Time) tea.Msg { return redrawMsg{} })
}

// selectedText is the selection as plain text: no colour, no trailing
// padding, and lines the pane wrapped put back together.
func (m model) selectedText() string {
	if !m.pick.on {
		return ""
	}
	from, to := m.pick.ordered()
	var b strings.Builder
	for r := from.row; r <= min(to.row, len(m.rows)-1); r++ {
		start, end, ok := m.pick.cols(r, ansi.StringWidth(m.rows[r]))
		text := ""
		if ok {
			text = ansi.Strip(ansi.Cut(m.rows[r], start, end))
		}
		if r < to.row && r+1 < len(m.rows) && m.cont[r+1] {
			b.WriteString(text)
			continue
		}
		b.WriteString(strings.TrimRight(text, " "))
		if r < to.row {
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// wordAt finds the word around a column of a row.
func wordAt(row string, col int) (from, to int, ok bool) {
	plain := ansi.Strip(row)
	at := func(c int) rune {
		r, _ := utf8.DecodeRuneInString(ansi.Cut(plain, c, c+1))
		return r
	}
	inWord := func(c int) bool {
		r := at(c)
		return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(wordPunct, r)
	}
	if col >= ansi.StringWidth(plain) || !inWord(col) {
		return 0, 0, false
	}
	from, to = col, col
	for from > 0 && inWord(from-1) {
		from--
	}
	for inWord(to + 1) {
		to++
	}
	// Sentence punctuation isn't part of the word.
	for to > from && strings.ContainsRune(".:,?", at(to)) {
		to--
	}
	return from, to, true
}

// lineAt selects the whole line a row belongs to, wrapped rows included.
func (m model) lineAt(row int) (selection, bool) {
	first, last := row, row
	for first > 0 && m.cont[first] {
		first--
	}
	for last+1 < len(m.rows) && m.cont[last+1] {
		last++
	}
	w := ansi.StringWidth(strings.TrimRight(ansi.Strip(m.rows[last]), " "))
	if w == 0 {
		return selection{}, false
	}
	return selection{anchor: point{first, 0}, head: point{last, w - 1}, on: true}, true
}

const (
	reverse   = "\x1b[7m"
	underline = "\x1b[4m"
)

// mark shows columns start to end of a row with the style in seq.
func mark(row string, start, end int, seq string) string {
	w := ansi.StringWidth(row)
	return ansi.Cut(row, 0, start) + seq + ansi.Strip(ansi.Cut(row, start, end)) + "\x1b[m" + ansi.Cut(row, end, w)
}

// overlay draws text at the right end of a row, over what's there.
func overlay(row, text string, w int) string {
	text = ansi.Truncate(text, w-1, "…")
	x := w - ansi.StringWidth(text) - 1
	left := ansi.Cut(row, 0, x)
	return left + "\x1b[m" + strings.Repeat(" ", x-ansi.StringWidth(left)) + ui.Render(text) + " "
}

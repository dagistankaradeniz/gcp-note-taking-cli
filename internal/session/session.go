// Package session is the interactive `quillink` session: a full-window
// view with the app name pinned top left and the version top right,
// command output in a scrollable, searchable pane (mouse wheel, PgUp/PgDn,
// Ctrl+F) that selects, copies and opens links with the mouse, and an
// input box pinned to the bottom with a status line under it. It doesn't run commands itself; the
// Host does (the cmd package).
package session

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/shell"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// Suggestion is one entry of the dropdown under the input.
type Suggestion struct {
	Text string
	Desc string
}

// Status is the line under the input.
type Status struct {
	Workspace string
	Account   string // email, or "" when not signed in
	Plan      string
	Unlocked  []string // e.g. "Vault", "Zero-Knowledge"
	// Production: the workspace talks to the production service. Any
	// other is highlighted so it's clear where commands go.
	Production bool
}

// Outcome is what the Host did with a line.
type Outcome struct {
	action int
	// Exit is the failed command's exit code; 0 when it worked.
	Exit int
}

var (
	// Ran: the line was a command and has finished.
	Ran = Outcome{}
	// Quit: leave the session.
	Quit = Outcome{action: 1}
	// Clear: clear the screen.
	Clear = Outcome{action: 2}
)

// Failed: the line was a command and it failed with this exit code.
func Failed(code int) Outcome { return Outcome{Exit: code} }

// Host runs what the user types.
type Host struct {
	// Run executes one line, writing to os.Stdout/os.Stderr as usual;
	// the session captures both while it runs.
	Run func(line string) Outcome
	// Suggest lists completions for the word before pos, and where that
	// word starts.
	Suggest func(line string, pos int) ([]Suggestion, int)
	// Status reads the status line (it may call the server).
	Status func() Status
	// RefreshAfter reports whether a line can change the status line
	// (login, logout, workspace use, unlock, lock...).
	RefreshAfter func(line string) bool
	// Cacheable reports whether a line only reads, and its output may be
	// kept in memory to show at once the next time, while it refreshes.
	// Lines that change the status line empty that memory.
	Cacheable func(line string) bool
	History   *shell.History
	// Classify tells what each word of a line is, for colouring it.
	Classify func(words []string) []Kind
	// IntroFrames open the output pane (the banner), played one after the
	// other every IntroEvery; the last one stays. A key skips to it.
	IntroFrames []string
	IntroEvery  time.Duration
	// Corner is pinned to the top-right of the window (version, build).
	Corner string
}

// ErrInterrupted is returned by Run when the user pressed Ctrl+C twice
// while a command was still running.
var ErrInterrupted = errors.New("interrupted")

// ErrCancelled is what a masked prompt returns when the user presses Esc.
var ErrCancelled = errors.New("cancelled")

var (
	activeMu sync.Mutex
	active   *tea.Program
)

// Active reports whether a session is running.
func Active() bool {
	activeMu.Lock()
	defer activeMu.Unlock()
	return active != nil
}

func send(msg tea.Msg) bool {
	activeMu.Lock()
	p := active
	activeMu.Unlock()
	if p == nil {
		return false
	}
	p.Send(msg)
	return true
}

// AskSecret shows a masked prompt in the input box and waits for it. It
// is called by a running command, from its own goroutine.
func AskSecret(label string) (string, error) { return ask(label, false) }

// Ask is AskSecret for an answer that isn't secret, such as a yes or no.
func Ask(label string) (string, error) { return ask(label, true) }

func ask(label string, plain bool) (string, error) {
	reply := make(chan secretReply, 1)
	if !send(promptMsg{label: label, plain: plain, reply: reply}) {
		return "", errors.New("no session")
	}
	r := <-reply
	return r.value, r.err
}

// Wiped reports that unlocked keys are gone: the session clears everything
// it has shown, as that may be what they decrypted, and shows text in its
// place. It's false when no session is running.
func Wiped(text string) bool { return send(wipedMsg(text)) }

type secretReply struct {
	value string
	err   error
}

// Messages.
type (
	outputMsg   string
	wipedMsg    string
	doneMsg     struct{ outcome Outcome }
	progressMsg struct{ state *progress.State }
	tickMsg     struct{}
	redrawMsg   struct{}
	introMsg    struct{}
	statusMsg   Status
	promptMsg   struct {
		label string
		plain bool
		reply chan secretReply
	}
)

// Sink sends progress to the session.
type Sink struct{}

func (Sink) Show(s *progress.State) { send(progressMsg{state: s}) }

type model struct {
	host    Host
	input   textinput.Model
	secret  textinput.Model
	out     viewport.Model
	lines   []string // output so far, oldest first
	gen     int      // counts add calls: lines can change at a constant length
	rows    []string // what the pane shows: everything wrapped to its width
	cont    []bool   // rows[i] carries on the line of rows[i-1]
	heads   []int    // the rows holding a typed command, ascending
	shown   paneKey  // what rows was built from
	intro   []string // banner frames; nil once cleared
	introAt int
	follow  bool // keep the pane scrolled to the newest output
	width   int
	height  int
	prompt  *promptMsg
	running string // the line running, "" when idle
	started time.Time
	prog    *progress.State
	frame   int
	ticking bool
	partial string
	sugs    []Suggestion
	sugFrom int
	sel     int
	hist    int // -1: not browsing history
	draft   string
	status  Status
	quitAt  time.Time // first Ctrl+C, for the "again to exit" window
	lastDur time.Duration
	lastEnd time.Time // when the last command finished; zero after a clear
	lastErr int       // the last command's exit code
	quit    bool
	killed  bool

	pick      selection
	clicks    int // 1-3: single, double, triple
	clickAt   point
	clickTime time.Time
	notice    string // shown briefly in the pane's corner
	noticeAt  time.Time

	finding bool
	find    textinput.Model
	matches []match
	at      int // the current match

	cache map[string]cachedOutput // by line
	stale int                     // last lines of the pane that are cached output
	fresh string                  // what the running command has written
}

// cachedOutput is a read-only command's last output.
type cachedOutput struct {
	text string
	at   time.Time
}

// paneKey is everything the pane's rows depend on, so they're only
// rebuilt when output arrives rather than on every key and spinner frame.
type paneKey struct {
	gen, lines, intros, introAt, width int
	partial                            string
	running                            bool
}

const (
	maxSuggestions = 6
	quitWindow     = 2 * time.Second
	// maxLines caps the output kept for scrolling back.
	maxLines = 5000
	// divider marks a line in the pane that is drawn as a rule after a
	// command's output; the rest of it is the label.
	divider = "\x00divider\x00"
	// typed marks the line showing a command as it was typed.
	typed = "\x00typed\x00"
	// failedMark starts a failed command's divider label.
	failedMark = "✗"
	// cacheFor and maxCached bound the output kept for showing at once.
	cacheFor  = 5 * time.Minute
	maxCached = 1000 // lines
)

func promptStyle() lipgloss.Style { return lipgloss.NewStyle().Foreground(ui.Brand).Bold(true) }

func newModel(h Host) model {
	in := textinput.New()
	in.Prompt = promptStyle().Render("› ")
	in.Placeholder = "type a command, or help"
	in.Focus()
	sec := textinput.New()
	sec.EchoMode = textinput.EchoPassword
	sec.EchoCharacter = '•'
	find := textinput.New()
	out := viewport.New()
	out.FillHeight = true
	out.MouseWheelEnabled = true
	// The pane scrolls with PgUp/PgDn and the wheel only: the arrow keys
	// belong to the input (suggestions, history).
	out.KeyMap = viewport.KeyMap{PageUp: out.KeyMap.PageUp, PageDown: out.KeyMap.PageDown}
	return model{host: h, input: in, secret: sec, find: find, out: out, hist: -1, follow: true, width: ui.Width(), intro: h.IntroFrames, cache: map[string]cachedOutput{}}
}

func (m model) animating() bool { return m.introAt < len(m.intro)-1 }

func (m model) introTick() tea.Cmd {
	return tea.Tick(max(m.host.IntroEvery, time.Millisecond), func(time.Time) tea.Msg { return introMsg{} })
}

// add appends output lines to the pane.
func (m *model) add(text string) {
	m.gen++
	m.lines = append(m.lines, strings.Split(text, "\n")...)
	if over := len(m.lines) - maxLines; over > 0 {
		m.lines = m.lines[over:]
	}
}

// layout sizes the output pane to the space above the input box and
// refills it when its content changed. A selection doesn't outlive that:
// the rows it points at may have moved.
func (m *model) layout() {
	w := max(m.width, 20)
	m.out.SetWidth(w)
	m.out.SetHeight(max(m.height-lipgloss.Height(m.bottom())-1, 1))
	key := paneKey{m.gen, len(m.lines), len(m.intro), m.introAt, w, m.partial, m.running != ""}
	if key != m.shown {
		m.shown = key
		m.pick = selection{}
		m.rows, m.cont, m.heads = wrapRows(m.paneLines(w), w)
		m.out.SetContentLines(m.rows)
		if m.finding {
			m.search()
		}
	}
	m.out.SetYOffset(m.out.YOffset()) // the pane may have grown
	if m.follow {
		m.out.GotoBottom()
	}
}

// paneLines is the pane's content, before wrapping. A running command
// gets an empty last row: paneView draws its loader there.
func (m model) paneLines(w int) []string {
	var lines []string
	if len(m.intro) > 0 {
		lines = strings.Split(strings.TrimRight(m.intro[m.introAt], "\n"), "\n")
	}
	for _, l := range m.lines {
		if label, ok := strings.CutPrefix(l, divider); ok {
			l = rule(label, w)
		}
		lines = append(lines, l)
	}
	if m.partial != "" {
		lines = append(lines, m.partial)
	}
	if m.running != "" {
		lines = append(lines, "")
	}
	return lines
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.input.Focus(), m.fetchStatus()}
	if m.animating() {
		cmds = append(cmds, m.introTick())
	}
	return tea.Batch(cmds...)
}

func (m model) fetchStatus() tea.Cmd {
	if m.host.Status == nil {
		return nil
	}
	return func() tea.Msg { return statusMsg(m.host.Status()) }
}

func tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *model) startTicking() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return tick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm := next.(model)
	nm.layout()
	return nm, cmd
}

func (m model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		ui.SetWidth(msg.Width)
		m.input.SetWidth(max(msg.Width-8, 10))
		m.find.SetWidth(max(msg.Width-8, 10))
		return m, nil

	case tea.MouseWheelMsg:
		m.out, _ = m.out.Update(msg)
		m.follow = m.out.AtBottom()
		return m, nil

	case tea.MouseClickMsg:
		return m.press(msg.Mouse())

	case tea.MouseMotionMsg:
		return m.drag(msg.Mouse())

	case tea.MouseReleaseMsg:
		return m.release()

	case copiedMsg:
		return m.copied(msg)

	case openMsg:
		return m.opened(msg)

	case openedMsg:
		if msg.err != nil {
			cmd := m.say("Couldn't open the link")
			return m, cmd
		}
		return m, nil

	case introMsg:
		if !m.animating() {
			return m, nil
		}
		m.introAt++
		if m.animating() {
			return m, m.introTick()
		}
		return m, nil

	case tickMsg:
		m.frame++
		if m.running != "" || m.prog != nil {
			return m, tick()
		}
		m.ticking = false
		return m, nil

	case outputMsg:
		m.dropStale()
		m.fresh += string(msg)
		m.partial += string(msg)
		i := strings.LastIndexByte(m.partial, '\n')
		if i < 0 {
			return m, nil
		}
		m.add(m.partial[:i])
		m.partial = m.partial[i+1:]
		return m, nil

	case wipedMsg:
		m.lines, m.partial, m.intro, m.introAt = nil, "", nil, 0
		m.stale, m.fresh = 0, ""
		m.gen++
		m.follow = true
		clear(m.cache)
		m.add(ui.Render(ui.Fg(ui.Green, "✓", true) + " " + string(msg)))
		return m, nil

	case progressMsg:
		m.prog = msg.state
		return m, m.startTicking()

	case statusMsg:
		m.status = Status(msg)
		return m, nil

	case promptMsg:
		m.prompt = &msg
		m.secret.Reset()
		m.secret.EchoMode = textinput.EchoPassword
		m.secret.Prompt = promptStyle().Render("🔑 " + msg.label + ": ")
		if msg.plain {
			m.secret.EchoMode = textinput.EchoNormal
			m.secret.Prompt = promptStyle().Render(msg.label + " ")
		}
		m.input.Blur()
		return m, m.secret.Focus()

	case doneMsg:
		var cmds []tea.Cmd
		m.dropStale()
		if m.partial != "" {
			m.add(m.partial)
			m.partial = ""
		}
		line := m.running
		m.running = ""
		m.prog = nil
		m.lastDur = time.Since(m.started)
		m.lastEnd = time.Now()
		switch msg.outcome {
		case Quit:
			m.quit = true
			return m, tea.Sequence(append(cmds, tea.Quit)...)
		case Clear:
			m.lines, m.intro, m.introAt = nil, nil, 0
			m.lastEnd = time.Time{}
			m.follow = true
		default:
			m.lastErr = msg.outcome.Exit
			m.add(divider + m.stamp())
			m.remember(line, msg.outcome)
		}
		if m.host.RefreshAfter != nil && m.host.RefreshAfter(line) {
			clear(m.cache)
			cmds = append(cmds, m.fetchStatus())
		}
		return m, tea.Sequence(cmds...)

	case tea.KeyPressMsg:
		return m.key(msg)
	}

	var cmd tea.Cmd
	switch {
	case m.prompt != nil:
		m.secret, cmd = m.secret.Update(msg)
	case m.finding:
		m.find, cmd = m.find.Update(msg)
		if _, pasted := msg.(tea.PasteMsg); pasted {
			m.search()
			m.at = len(m.matches) - 1
			m.reveal()
		}
	default:
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	m.pick = selection{}
	if m.animating() {
		m.introAt = len(m.intro) - 1 // any key finishes the banner
	}

	// A masked prompt owns the keyboard until it's answered.
	if m.prompt != nil {
		switch k {
		case "enter":
			m.prompt.reply <- secretReply{value: m.secret.Value()}
		case "esc", "ctrl+c":
			m.prompt.reply <- secretReply{err: ErrCancelled}
		default:
			var cmd tea.Cmd
			m.secret, cmd = m.secret.Update(msg)
			return m, cmd
		}
		m.secret.Reset()
		m.prompt = nil
		return m, m.input.Focus()
	}

	if m.finding {
		return m.findKey(msg, k)
	}
	if k == "ctrl+f" || k == "/" && m.input.Value() == "" {
		return m.openFind()
	}

	if k == "ctrl+c" {
		if m.running == "" && m.input.Value() != "" {
			m.input.Reset()
			m.sugs = nil
			return m, nil
		}
		if time.Since(m.quitAt) < quitWindow {
			m.quit = true
			m.killed = m.running != ""
			return m, tea.Quit
		}
		m.quitAt = time.Now()
		return m, tea.Tick(quitWindow, func(time.Time) tea.Msg { return redrawMsg{} })
	}
	if k == "ctrl+d" && m.running == "" && m.input.Value() == "" {
		m.quit = true
		return m, tea.Quit
	}
	if k == "ctrl+l" {
		m.lines, m.intro, m.introAt, m.stale = nil, nil, 0, 0
		m.lastEnd = time.Time{}
		m.follow = true
		return m, nil
	}
	if k == "pgup" || k == "pgdown" {
		m.out, _ = m.out.Update(msg)
		m.follow = m.out.AtBottom()
		return m, nil
	}

	switch k {
	case "tab":
		if len(m.sugs) > 0 {
			m.accept(m.sugs[m.sel].Text)
		}
		return m, nil
	case "esc":
		m.sugs = nil
		return m, nil
	case "up", "down":
		if len(m.sugs) > 0 {
			if k == "up" {
				m.sel = (m.sel - 1 + len(m.sugs)) % len(m.sugs)
			} else {
				m.sel = (m.sel + 1) % len(m.sugs)
			}
			return m, nil
		}
		m.browseHistory(k == "up")
		return m, nil
	case "enter":
		if m.running != "" {
			return m, nil
		}
		return m.submit()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.hist = -1
	m.suggest()
	return m, cmd
}

// accept puts a suggestion in place of the word being typed.
func (m *model) accept(text string) {
	v := m.input.Value()
	pos := m.input.Position()
	head := v[:m.sugFrom] + text + " "
	m.input.SetValue(head + strings.TrimLeft(v[pos:], " "))
	m.input.SetCursor(len(head))
	m.sugs = nil
	m.suggest()
}

func (m *model) suggest() {
	m.sugs, m.sel = nil, 0
	v := m.input.Value()
	if m.host.Suggest == nil || strings.TrimSpace(v) == "" {
		return
	}
	sugs, from := m.host.Suggest(v, m.input.Position())
	word := v[from:m.input.Position()]
	if len(sugs) == 1 && sugs[0].Text == word {
		return
	}
	if len(sugs) > maxSuggestions {
		sugs = sugs[:maxSuggestions]
	}
	m.sugs, m.sugFrom = sugs, from
}

func (m *model) browseHistory(older bool) {
	h := m.host.History
	if h == nil || h.Len() == 0 {
		return
	}
	switch {
	case older && m.hist < h.Len()-1:
		if m.hist == -1 {
			m.draft = m.input.Value()
		}
		m.hist++
	case !older && m.hist > -1:
		m.hist--
	default:
		return
	}
	if m.hist == -1 {
		m.input.SetValue(m.draft)
	} else {
		m.input.SetValue(h.At(m.hist))
	}
	m.input.CursorEnd()
}

func (m model) submit() (tea.Model, tea.Cmd) {
	line := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	m.sugs = nil
	m.hist = -1
	if line == "" {
		return m, nil
	}
	if m.host.History != nil {
		m.host.History.Add(line)
	}
	m.add(typed + ui.Render(promptStyle().Render("› ")+highlight(line, m.host.Classify)))
	m.showCached(line)
	m.follow = true
	m.running = line
	m.started = time.Now()
	run := func() tea.Msg { return doneMsg{outcome: capture(func() Outcome { return m.host.Run(line) })} }
	return m, tea.Batch(run, m.startTicking())
}

// capture runs fn with stdout and stderr going to the session, line by
// line, in the order they were written.
func capture(fn func() Outcome) Outcome {
	r, w, err := os.Pipe()
	if err != nil {
		return fn()
	}
	prevOut, prevErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	style.TreatAsTerminal(w)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				send(outputMsg(buf[:n]))
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					send(outputMsg(fmt.Sprintf("\n(output stopped: %v)\n", err)))
				}
				return
			}
		}
	}()
	outcome := fn()
	os.Stdout, os.Stderr = prevOut, prevErr
	style.TreatAsTerminal()
	_ = w.Close()
	<-done
	_ = r.Close()
	return outcome
}

func (m model) View() tea.View {
	v := tea.NewView(m.top() + "\n" + m.paneView() + "\n" + m.bottom())
	if m.prompt == nil && !m.finding {
		// The real terminal cursor, in the input line: under the top
		// row, the pane, the hint above the box if any, and its border.
		pre := m.aboveBox()
		y := 1 + m.out.Height() + strings.Count(pre, "\n") + 1
		_, col := m.field(m.fieldWidth())
		v.Cursor = tea.NewCursor(2+col, y)
	}
	v.AltScreen = true
	// Wheel and drag events come to the pane (see selection.go).
	// Shift-drag (Option-drag on macOS) still reaches the terminal's own
	// selection.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// paneView is the pane with what's drawn over it: links, search matches,
// the selection, the command being read, and the running command's
// loader, the last command's time and any notice.
// The loader sits at the end of the output, where the finished command's
// divider takes over from it. Whatever can't be seen there, because the
// pane is scrolled up, goes to the pane's bottom-right corner instead.
func (m model) paneView() string {
	rows := strings.Split(m.out.View(), "\n")
	top := m.out.YOffset()
	w := max(m.width, 20)
	markLinks(rows)
	m.markMatches(rows, top)
	if m.pick.on {
		for i := range rows {
			if r := top + i; r < len(m.rows) {
				if start, end, ok := m.pick.cols(r, ansi.StringWidth(m.rows[r])); ok {
					rows[i] = mark(rows[i], start, end, reverse)
				}
			}
		}
	}
	if h := m.pinned(); h >= 0 {
		rows[0] = pinnedRow(m.rows[h], w)
	}
	corner := ""
	switch loader := m.loader(); {
	case loader != "" && len(m.rows)-1-top < len(rows):
		rows[len(m.rows)-1-top] = ruleStyled(ansi.Truncate(loader, w-6, "…"), ui.Subtle, w)
	case loader != "":
		corner = loader
	case !m.follow && !m.lastEnd.IsZero():
		corner = ui.Fg(ruleColor(m.stamp()), m.stamp(), false)
	}
	if m.finding {
		corner = m.findCorner()
	}
	if m.notice != "" && time.Since(m.noticeAt) < noticeFor {
		corner = ui.Fg(ui.Green, "✓ "+m.notice, false)
	}
	if corner != "" {
		last := len(rows) - 1
		rows[last] = overlay(rows[last], corner, w)
	}
	return strings.Join(rows, "\n")
}

// pinned is the row of the command whose output fills the top of the
// pane, once that command has scrolled out of view; -1 when there's none.
func (m model) pinned() int {
	top := m.out.YOffset()
	i := sort.SearchInts(m.heads, top)
	if i == 0 || m.out.Height() < 3 || i < len(m.heads) && m.heads[i] == top {
		return -1
	}
	return m.heads[i-1]
}

// pinnedRow is a command held at the top of the pane, ruled off from the
// output scrolling under it.
func pinnedRow(row string, w int) string {
	row = ansi.Truncate(row, w, "…")
	fill := w - ansi.StringWidth(row) - 1
	if fill < 1 {
		return row
	}
	return row + " " + ui.Render(ui.Fg(ui.Subtle, strings.Repeat("┄", fill), false))
}

// loader is the running command's spinner or progress bar.
func (m model) loader() string {
	switch {
	case m.prog != nil:
		return progress.Line(m.prog, m.frame)
	case m.running != "":
		label := commandName(m.running)
		if m.stale > 0 {
			label = "Refreshing " + label
		}
		return progress.Line(&progress.State{Label: label, Started: m.started}, m.frame)
	}
	return ""
}

// stamp is when the last command finished and how long it took, led by
// the exit code when it failed.
func (m model) stamp() string {
	s := m.lastEnd.Format("15:04") + " · " + duration(m.lastDur)
	if m.lastErr != 0 {
		s = fmt.Sprintf("%s exit %d · %s", failedMark, m.lastErr, s)
	}
	return s
}

// showCached puts line's last output in the pane until the real one
// comes; a cached entry past its time is dropped.
func (m *model) showCached(line string) {
	m.stale, m.fresh = 0, ""
	c, ok := m.cache[line]
	if !ok {
		return
	}
	if time.Since(c.at) > cacheFor {
		delete(m.cache, line)
		return
	}
	m.add(c.text)
	m.stale = strings.Count(c.text, "\n") + 1
}

// dropStale takes the cached output back out of the pane.
func (m *model) dropStale() {
	if m.stale > 0 {
		m.lines = m.lines[:max(len(m.lines)-m.stale, 0)]
		m.stale = 0
		m.gen++
	}
}

// remember keeps a finished command's output for next time.
func (m *model) remember(line string, o Outcome) {
	text := strings.TrimRight(m.fresh, "\n")
	m.fresh = ""
	if o != Ran || text == "" || strings.Count(text, "\n") >= maxCached ||
		m.host.Cacheable == nil || !m.host.Cacheable(line) {
		delete(m.cache, line)
		return
	}
	m.cache[line] = cachedOutput{text: text, at: time.Now()}
}

// top is the row above the output pane: the app name on the left, as in
// the banner ("ink" in the brand yellow), and the version on the right.
func (m model) top() string {
	w := max(m.width, 20)
	name := " " + ui.Fg(lipgloss.NoColor{}, "Quill", true) + ui.Fg(ui.Brand, "ink", true)
	const nameW = 9
	c := ansi.Truncate(m.host.Corner, max(w-nameW-2, 0), "…")
	gap := max(w-nameW-1-ansi.StringWidth(c), 1)
	return ui.Render(name + strings.Repeat(" ", gap) + ui.Fg(ui.Muted, c, false))
}

// bottom is everything under the output pane: the exit hint, input box,
// suggestions and the status line.
func (m model) bottom() string {
	w := max(m.width, 20)
	var b strings.Builder
	b.WriteString(m.aboveBox())

	field, _ := m.field(m.fieldWidth())
	border := ui.Subtle
	switch {
	case m.prompt != nil:
		field, border = m.secret.View(), ui.Brand
	case m.finding:
		field, border = m.find.View(), ui.Brand
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(w)
	b.WriteString(box.Render(field) + "\n")

	if len(m.sugs) > 0 && m.prompt == nil {
		nameW := 0
		for _, s := range m.sugs {
			nameW = max(nameW, ansi.StringWidth(s.Text))
		}
		for i, s := range m.sugs {
			c := ui.Brand
			if strings.HasPrefix(s.Text, "-") {
				c = ui.Green
			}
			pad := strings.Repeat(" ", nameW-ansi.StringWidth(s.Text))
			marker := "  "
			if i == m.sel {
				marker = ui.Fg(ui.Brand, "› ", true)
			}
			line := marker + ui.Fg(c, s.Text, i == m.sel) + pad + "  " + ui.Fg(ui.Muted, s.Desc, false)
			b.WriteString(ansi.Truncate(line, w, "…") + "\n")
		}
	}

	b.WriteString(m.statusLine(w))
	return ui.Render(b.String())
}

// fieldWidth is the room inside the input box (border and padding off).
func (m model) fieldWidth() int { return max(m.width, 20) - 4 }

// field draws the input line -- prompt and coloured value, scrolled so
// the cursor stays in view -- and the cursor's column within it.
func (m model) field(width int) (string, int) {
	prompt := promptStyle().Render("› ")
	const promptW = 2
	v := m.input.Value()
	if v == "" {
		return prompt + ui.Fg(ui.Subtle, m.input.Placeholder, false), promptW
	}
	room := max(width-promptW-1, 1)
	col := ansi.StringWidth(string([]rune(v)[:min(m.input.Position(), len([]rune(v)))]))
	off := max(col-room, 0)
	shown := highlight(v, m.host.Classify)
	if off > 0 || ansi.StringWidth(v) > room {
		shown = ansi.Cut(shown, off, off+room)
	}
	return prompt + shown, promptW + col - off
}

// aboveBox is what sits between the pane and the input box: the "press
// again to exit" hint.
func (m model) aboveBox() string {
	if m.quitAt.IsZero() || time.Since(m.quitAt) >= quitWindow {
		return ""
	}
	hint := "Press Ctrl+C again to exit"
	if m.running != "" {
		hint = "Still running. Press Ctrl+C again to stop and exit"
	}
	return ui.Fg(ui.Amber, hint, false) + "\n"
}

// statusLine is muted, with the workspace in the brand colour when it
// isn't production and the scrolled-up note in amber. Each piece carries
// its own colour, so they don't cut into one another.
func (m model) statusLine(w int) string {
	muted := func(s string) string { return ui.Fg(ui.Muted, s, false) }
	ws := muted(orDash(m.status.Workspace))
	if m.status.Workspace != "" && !m.status.Production {
		ws = ui.Fg(ui.Brand, m.status.Workspace, false)
	}
	left := []string{ui.Fg(ui.Brand, "◆", false) + " " + ws}
	if m.status.Account != "" {
		acct := m.status.Account
		if m.status.Plan != "" {
			acct += " · " + m.status.Plan
		}
		left = append(left, muted(acct))
	} else if m.status.Workspace != "" {
		left = append(left, muted("not signed in"))
	}
	var right []string
	for _, u := range m.status.Unlocked {
		right = append(right, muted("🔓 "+u))
	}
	if !m.follow {
		right = append(right, ui.Fg(ui.Amber, "scrolled up · PgDn for latest", false))
	}
	l := " " + strings.Join(left, ui.Fg(ui.Subtle, " │ ", false))
	sep := ui.Fg(ui.Subtle, " · ", false)
	// The key hints are the first thing to go in a narrow window.
	hints := muted("Tab complete · ↑ history · PgUp scroll · / find · drag to copy · Ctrl+D exit")
	if m.finding {
		hints = muted("Enter or ↑ older · ↓ newer · Esc close")
	}
	for _, r := range []string{strings.Join(append(right[:len(right):len(right)], hints), sep), strings.Join(right, sep)} {
		r += " "
		if gap := w - ansi.StringWidth(l) - ansi.StringWidth(r); gap >= 1 {
			return l + strings.Repeat(" ", gap) + r
		}
	}
	return ansi.Truncate(l, w, "…")
}

// rule is the divider after a command: a dim line across the pane with
// the command's finish time and duration at its right end.
func rule(label string, w int) string {
	line := ui.Subtle
	if strings.HasPrefix(label, failedMark) {
		line = ui.Red
	}
	return ruleStyled(ui.Fg(ruleColor(label), label, false), line, w)
}

// ruleColor is red for a failed command's label.
func ruleColor(label string) color.Color {
	if strings.HasPrefix(label, failedMark) {
		return ui.Red
	}
	return ui.Muted
}

// ruleStyled is a rule with a label that carries its own colour.
func ruleStyled(label string, line color.Color, w int) string {
	tail := " " + label + " "
	if ansi.StringWidth(tail)+4 > w {
		return ui.Render(ui.Fg(line, strings.Repeat("─", w), false))
	}
	return ui.Render(ui.Fg(line, strings.Repeat("─", w-ansi.StringWidth(tail)-2), false) +
		tail + ui.Fg(line, "──", false))
}

// duration is short: tenths under a minute, whole seconds above.
func duration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func orDash(s string) string {
	if s == "" {
		return "…"
	}
	return s
}

// commandName is the command part of a line ("note list"), for the
// spinner.
func commandName(line string) string {
	args, err := shell.Split(line)
	if err != nil || len(args) == 0 {
		return line
	}
	var words []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") || len(words) == 2 {
			break
		}
		words = append(words, a)
	}
	return strings.Join(words, " ")
}

// Run starts the session and blocks until the user leaves it.
func Run(h Host) error {
	m := newModel(h)
	p := tea.NewProgram(m, tea.WithOutput(os.Stdout), tea.WithInput(os.Stdin))
	activeMu.Lock()
	active = p
	activeMu.Unlock()
	progress.SetSink(Sink{})
	defer func() {
		progress.SetSink(nil)
		activeMu.Lock()
		active = nil
		activeMu.Unlock()
	}()
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(model); ok && fm.killed {
		return ErrInterrupted
	}
	return nil
}

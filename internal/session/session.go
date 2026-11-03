// Package session is the interactive `quillink` session: a full-window
// view with command output in a scrollable pane (mouse wheel, PgUp/PgDn)
// and an input box pinned to the bottom with a status line under it. It
// doesn't run commands itself; the Host does (the cmd package).
package session

import (
	"errors"
	"fmt"
	"io"
	"os"
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
}

// Outcome is what the Host did with a line.
type Outcome int

const (
	// Ran: the line was a command and has finished.
	Ran Outcome = iota
	// Quit: leave the session.
	Quit
	// Clear: clear the screen.
	Clear
)

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
	History      *shell.History
	// Intro is shown at the top of the output pane (the banner).
	Intro string
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
func AskSecret(label string) (string, error) {
	reply := make(chan secretReply, 1)
	if !send(promptMsg{label: label, reply: reply}) {
		return "", errors.New("no session")
	}
	r := <-reply
	return r.value, r.err
}

type secretReply struct {
	value string
	err   error
}

// Messages.
type (
	outputMsg   string
	doneMsg     struct{ outcome Outcome }
	progressMsg struct{ state *progress.State }
	tickMsg     struct{}
	redrawMsg   struct{}
	statusMsg   Status
	promptMsg   struct {
		label string
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
	follow  bool     // keep the pane scrolled to the newest output
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
	quit    bool
	killed  bool
}

const (
	maxSuggestions = 6
	quitWindow     = 2 * time.Second
	// maxLines caps the output kept for scrolling back.
	maxLines = 5000
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
	out := viewport.New()
	out.SoftWrap = true
	out.FillHeight = true
	out.MouseWheelEnabled = true
	// The pane scrolls with PgUp/PgDn and the wheel only: the arrow keys
	// belong to the input (suggestions, history).
	out.KeyMap = viewport.KeyMap{PageUp: out.KeyMap.PageUp, PageDown: out.KeyMap.PageDown}
	m := model{host: h, input: in, secret: sec, out: out, hist: -1, follow: true, width: ui.Width()}
	if h.Intro != "" {
		m.lines = strings.Split(strings.TrimRight(h.Intro, "\n"), "\n")
	}
	return m
}

// add appends output lines to the pane.
func (m *model) add(text string) {
	m.lines = append(m.lines, strings.Split(text, "\n")...)
	if over := len(m.lines) - maxLines; over > 0 {
		m.lines = m.lines[over:]
	}
}

// layout sizes the output pane to the space above the input box and
// refills it.
func (m *model) layout() {
	lines := m.lines
	if m.partial != "" {
		lines = append(lines[:len(lines):len(lines)], m.partial)
	}
	m.out.SetWidth(max(m.width, 20))
	m.out.SetHeight(max(m.height-lipgloss.Height(m.bottom()), 1))
	m.out.SetContentLines(lines)
	if m.follow {
		m.out.GotoBottom()
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.input.Focus(), m.fetchStatus())
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
		return m, nil

	case tea.MouseWheelMsg:
		m.out, _ = m.out.Update(msg)
		m.follow = m.out.AtBottom()
		return m, nil

	case tickMsg:
		m.frame++
		if m.running != "" || m.prog != nil {
			return m, tick()
		}
		m.ticking = false
		return m, nil

	case outputMsg:
		m.partial += string(msg)
		i := strings.LastIndexByte(m.partial, '\n')
		if i < 0 {
			return m, nil
		}
		m.add(m.partial[:i])
		m.partial = m.partial[i+1:]
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
		m.secret.Prompt = promptStyle().Render("🔑 " + msg.label + ": ")
		m.input.Blur()
		return m, m.secret.Focus()

	case doneMsg:
		var cmds []tea.Cmd
		if m.partial != "" {
			m.add(m.partial)
			m.partial = ""
		}
		line := m.running
		m.running = ""
		m.prog = nil
		m.lastDur = time.Since(m.started)
		switch msg.outcome {
		case Quit:
			m.quit = true
			return m, tea.Sequence(append(cmds, tea.Quit)...)
		case Clear:
			m.lines = nil
			m.follow = true
		}
		if m.host.RefreshAfter != nil && m.host.RefreshAfter(line) {
			cmds = append(cmds, m.fetchStatus())
		}
		return m, tea.Sequence(cmds...)

	case tea.KeyPressMsg:
		return m.key(msg)
	}

	var cmd tea.Cmd
	if m.prompt != nil {
		m.secret, cmd = m.secret.Update(msg)
	} else {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()

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
		m.lines = nil
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
	m.add(ui.Render(promptStyle().Render("› ") + line))
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
	v := tea.NewView(m.out.View() + "\n" + m.bottom())
	v.AltScreen = true
	// Wheel events come to the pane; selecting text takes Shift-drag
	// (Option-drag on macOS) while the mouse is captured.
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// bottom is everything under the output pane: spinner, input box,
// suggestions and the status line.
func (m model) bottom() string {
	w := max(m.width, 20)
	var b strings.Builder

	switch {
	case m.prog != nil:
		b.WriteString(progress.Line(m.prog, m.frame) + "\n")
	case m.running != "":
		b.WriteString(progress.Line(&progress.State{Label: commandName(m.running), Started: m.started}, m.frame) + "\n")
	}
	if !m.quitAt.IsZero() && time.Since(m.quitAt) < quitWindow {
		hint := "Press Ctrl+C again to exit"
		if m.running != "" {
			hint = "Still running. Press Ctrl+C again to stop and exit"
		}
		b.WriteString(ui.Fg(ui.Amber, hint, false) + "\n")
	}

	field := m.input.View()
	if m.prompt != nil {
		field = m.secret.View()
	}
	border := ui.Subtle
	if m.prompt != nil {
		border = ui.Brand
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
			name := s.Text + strings.Repeat(" ", nameW-ansi.StringWidth(s.Text))
			line := "  " + name + "  " + ui.Fg(ui.Muted, s.Desc, false)
			if i == m.sel {
				line = ui.Fg(ui.Brand, "› "+name, true) + "  " + ui.Fg(ui.Muted, s.Desc, false)
			}
			b.WriteString(ansi.Truncate(line, w, "…") + "\n")
		}
	}

	b.WriteString(m.statusLine(w))
	return ui.Render(b.String())
}

func (m model) statusLine(w int) string {
	left := []string{ui.Fg(ui.Brand, "◆", false) + " " + orDash(m.status.Workspace)}
	if m.status.Account != "" {
		acct := m.status.Account
		if m.status.Plan != "" {
			acct += " · " + m.status.Plan
		}
		left = append(left, acct)
	} else if m.status.Workspace != "" {
		left = append(left, "not signed in")
	}
	var right []string
	for _, u := range m.status.Unlocked {
		right = append(right, "🔓 "+u)
	}
	if m.lastDur > 0 && m.running == "" {
		right = append(right, fmt.Sprintf("%.1fs", m.lastDur.Seconds()))
	}
	if !m.follow {
		right = append(right, ui.Fg(ui.Amber, "scrolled up · PgDn for latest", false))
	}
	right = append(right, "Tab complete · ↑ history · PgUp scroll · Ctrl+D exit")
	l := " " + strings.Join(left, ui.Fg(ui.Subtle, " │ ", false))
	r := strings.Join(right, ui.Fg(ui.Subtle, " · ", false)) + " "
	gap := w - ansi.StringWidth(l) - ansi.StringWidth(r)
	if gap < 1 {
		return ui.Fg(ui.Muted, ansi.Truncate(l, w, "…"), false)
	}
	return ui.Fg(ui.Muted, l+strings.Repeat(" ", gap)+r, false)
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

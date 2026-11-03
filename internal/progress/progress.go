// Package progress shows what a command is doing while it waits: a
// spinner for work of unknown length, a bar when the total is known. A
// one-shot command draws it on stderr (so stdout stays clean for pipes and
// redirects); the interactive session draws it above its input line.
// Nothing is drawn unless a Sink is installed, which only happens on a
// terminal.
package progress

import (
	"sync"
	"time"
)

// State is what a Sink shows.
type State struct {
	Label   string
	Current int
	Total   int // 0: a spinner, no bar
	Bytes   bool
	Started time.Time
}

// Sink draws the active task; Show(nil) clears it.
type Sink interface {
	Show(s *State)
}

var (
	mu     sync.Mutex
	sink   Sink
	active *Task
)

// SetSink installs (or with nil removes) where progress is drawn.
func SetSink(s Sink) {
	mu.Lock()
	defer mu.Unlock()
	if sink != nil && active != nil {
		sink.Show(nil)
	}
	sink = s
	active = nil
}

// Task is one piece of work being shown. A nil *Task is valid and does
// nothing, so callers never need to check.
type Task struct {
	state State
	quiet bool
}

// Start shows a spinner (total 0) or a bar out of total.
func Start(label string, total int) *Task { return start(label, total, false, false) }

// StartBytes is a bar counting bytes.
func StartBytes(label string, total int) *Task { return start(label, total, true, false) }

// Busy is the background spinner for a network call; it never replaces a
// task already on screen.
func Busy(label string) *Task { return start(label, 0, false, true) }

func start(label string, total int, bytes, quiet bool) *Task {
	mu.Lock()
	defer mu.Unlock()
	if sink == nil || (quiet && active != nil) {
		return nil
	}
	t := &Task{state: State{Label: label, Total: total, Bytes: bytes, Started: time.Now()}, quiet: quiet}
	active = t
	s := t.state
	sink.Show(&s)
	return t
}

// Set moves the bar to n.
func (t *Task) Set(n int) {
	if t == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	t.state.Current = n
	t.show()
}

// Add moves the bar on by n.
func (t *Task) Add(n int) {
	if t == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	t.state.Current += n
	t.show()
}

// SetTotal changes the bar's total once it's known.
func (t *Task) SetTotal(total int) {
	if t == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	t.state.Total = total
	t.show()
}

func (t *Task) show() {
	if sink != nil && active == t {
		s := t.state
		sink.Show(&s)
	}
}

// Done clears the task.
func (t *Task) Done() {
	if t == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if active == t {
		active = nil
		if sink != nil {
			sink.Show(nil)
		}
	}
}

// Reset clears whatever is showing, e.g. when a command returns early.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	if active != nil && sink != nil {
		sink.Show(nil)
	}
	active = nil
}

package progress

import (
	"io"
	"sync"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// delay keeps quick calls from flashing a spinner at all.
const delay = 250 * time.Millisecond

// Line-at-a-time drawing on stderr for one-shot commands: the line is
// redrawn in place and erased when the task ends, so nothing is left
// behind in the terminal or mixed into stdout.
type LineSink struct {
	w     io.Writer
	mu    sync.Mutex
	state *State
	drawn bool
	frame int
	stop  chan struct{}
}

// NewLineSink draws on w (a terminal) until Close.
func NewLineSink(w io.Writer) *LineSink {
	s := &LineSink{w: w, stop: make(chan struct{})}
	go s.loop()
	return s
}

func (s *LineSink) Show(st *State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = st
	if st == nil {
		s.erase()
	}
}

func (s *LineSink) loop() {
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			s.mu.Lock()
			if st := s.state; st != nil && time.Since(st.Started) >= delay {
				s.frame++
				_, _ = io.WriteString(s.w, "\r\033[2K"+ui.Render(Line(st, s.frame)))
				s.drawn = true
			}
			s.mu.Unlock()
		}
	}
}

func (s *LineSink) erase() {
	if s.drawn {
		_, _ = io.WriteString(s.w, "\r\033[2K")
		s.drawn = false
	}
}

// Close stops drawing and erases the line.
func (s *LineSink) Close() {
	close(s.stop)
	s.mu.Lock()
	s.erase()
	s.state = nil
	s.mu.Unlock()
}

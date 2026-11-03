package progress

import (
	"strings"
	"testing"
	"time"
)

type recorder struct{ shown []*State }

func (r *recorder) Show(s *State) { r.shown = append(r.shown, s) }

func TestNilTaskIsSafe(t *testing.T) {
	SetSink(nil)
	task := Start("x", 10)
	if task != nil {
		t.Fatal("no sink: want a nil task")
	}
	task.Add(1)
	task.Set(3)
	task.SetTotal(4)
	task.Done()
}

func TestBusyNeverReplacesATask(t *testing.T) {
	r := &recorder{}
	SetSink(r)
	defer SetSink(nil)
	bar := Start("Decrypting notes", 10)
	if Busy("Contacting Quillink") != nil {
		t.Fatal("Busy replaced an active task")
	}
	bar.Set(5)
	bar.Done()
	if last := r.shown[len(r.shown)-1]; last != nil {
		t.Fatalf("Done should clear, got %+v", last)
	}
	if b := Busy("Contacting Quillink"); b == nil {
		t.Fatal("Busy with nothing active should show")
	}
	Reset()
}

func TestLine(t *testing.T) {
	s := &State{Label: "Decrypting notes", Current: 312, Total: 1240, Started: time.Now()}
	if got := Line(s, 0); !strings.Contains(got, "312 / 1,240") {
		t.Errorf("bar line = %q", got)
	}
	s = &State{Label: "Downloading", Current: 3 << 20, Total: 10 << 20, Bytes: true, Started: time.Now()}
	if got := Line(s, 0); !strings.Contains(got, "3.0 MB / 10.0 MB") {
		t.Errorf("bytes line = %q", got)
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := Count(n); got != want {
			t.Errorf("Count(%d) = %q, want %q", n, got, want)
		}
	}
}

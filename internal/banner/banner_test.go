package banner

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
)

func TestVersionLine(t *testing.T) {
	built := time.Date(2026, 11, 2, 14, 31, 0, 0, time.UTC)
	cases := []struct {
		info Info
		want string
	}{
		{Info{Version: "1.4.0", Commit: "cf66d93", Built: built}, "v1.4.0 · built 2026-11-02 14:31 UTC · cf66d93"},
		{Info{Version: "dev"}, "dev"},
		{Info{Version: "dev", Commit: "abc1234"}, "dev · abc1234"},
	}
	for _, c := range cases {
		if got := VersionLine(c.info); got != c.want {
			t.Errorf("VersionLine(%+v) = %q, want %q", c.info, got, c.want)
		}
	}
}

func TestHeaderPlainFitsWidth(t *testing.T) {
	info := Info{Version: "1.4.0", Commit: "cf66d93", Built: time.Date(2026, 11, 2, 14, 31, 0, 0, time.UTC)}
	out := Header(style.Plain, info, 120)
	if strings.Contains(out, "\033[") {
		t.Fatal("plain palette emitted escape codes")
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > Width {
			t.Errorf("line wider than %d: %d %q", Width, n, l)
		}
	}
	last := lines[len(lines)-1]
	if utf8.RuneCountInString(last) != Width || !strings.HasSuffix(last, "cf66d93") {
		t.Errorf("version line not right-aligned to %d: %q", Width, last)
	}
}

func TestHeaderCompactWhenNarrow(t *testing.T) {
	out := Header(style.Plain, Info{Version: "dev"}, 40)
	if strings.Contains(out, "██") {
		t.Fatal("narrow terminal got the block wordmark")
	}
	if !strings.Contains(out, "Quillink") || !strings.Contains(out, "BETA") {
		t.Fatalf("compact header missing name/badge: %q", out)
	}
}

func TestComposePanelBesideOrBelow(t *testing.T) {
	sugg := []Suggestion{{"note list", "", "your recent notes"}, {"note get", "<id>", "open a note"}, {"exit", "", "leave the session"}}
	info := Info{Version: "1.4.0"}
	wide := Compose(style.Plain, info, 140, sugg)
	lines := strings.Split(wide, "\n")
	if !strings.Contains(lines[1], "██") || !strings.HasSuffix(lines[1], "Suggested") {
		t.Fatalf("panel not beside the wordmark: %q", lines[1])
	}
	// 140 columns leave room for the widest gap, divider in its middle.
	if idx := strings.Index(lines[3], "note get <id>"); utf8.RuneCountInString(lines[3][:idx]) != Width+maxPanelGap {
		t.Errorf("panel column misaligned: %q", lines[3])
	}
	if idx := strings.Index(lines[3], "│"); utf8.RuneCountInString(lines[3][:idx]) != Width+maxPanelGap/2 {
		t.Errorf("divider not in the middle of the gap: %q", lines[3])
	}
	// Just enough room: the gap shrinks to fit instead of moving the panel.
	tight := Compose(style.Plain, info, Width+minPanelGap+maxWidth(panelLines(style.Plain, sugg)), sugg)
	if !strings.HasSuffix(strings.Split(tight, "\n")[1], "Suggested") {
		t.Error("panel should stay beside the wordmark with the minimum gap")
	}
	narrow := Compose(style.Plain, info, 90, sugg)
	if strings.Contains(strings.Split(narrow, "\n")[1], "Suggested") {
		t.Error("panel should move below the wordmark when it doesn't fit")
	}
	if !strings.Contains(narrow, "  Suggested\n") {
		t.Errorf("panel missing below: %q", narrow)
	}
}

func TestNetworkConnectedAndInBounds(t *testing.T) {
	for seed := range uint64(50) {
		n := NewNetwork(rand.New(rand.NewPCG(seed, seed)))
		if len(n.nodes) != netNodes {
			t.Fatalf("seed %d: %d nodes", seed, len(n.nodes))
		}
		if len(n.edges) < netNodes-1 {
			t.Fatalf("seed %d: only %d edges", seed, len(n.edges))
		}
		frames := n.Frames()
		if last := frames[len(frames)-1]; last.Nodes != netNodes {
			t.Fatalf("seed %d: timeline ends with %d nodes", seed, last.Nodes)
		}
		for _, l := range n.Render(style.Plain, n.Final()) {
			if w := utf8.RuneCountInString(l); w > Width {
				t.Fatalf("seed %d: row width %d", seed, w)
			}
		}
	}
}

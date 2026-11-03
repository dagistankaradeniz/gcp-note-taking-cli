package banner

import (
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
)

// The network sits above the wordmark: nodes pop in one at a time and
// straight edges draw between them once both ends exist. Edges are drawn
// in braille cells (2x4 dots each), so diagonals look smooth.
const (
	netRows  = 7
	netCols  = Width - len(indent)
	netNodes = 9
	// PlayDuration is how long the animation runs before the final frame.
	PlayDuration = 1800 * time.Millisecond
)

type cell struct{ col, row int }

// Network is one randomly laid out graph.
type Network struct {
	nodes []cell
	edges [][2]int
}

// NewNetwork lays out netNodes nodes with some spacing, joins each to its
// nearest earlier node (so the graph is connected) and adds two extra
// cross-links.
func NewNetwork(r *rand.Rand) Network {
	var n Network
	for tries := 0; len(n.nodes) < netNodes && tries < 10000; tries++ {
		c := cell{2 + r.IntN(netCols-4), r.IntN(netRows)}
		ok := true
		for _, o := range n.nodes {
			if abs(c.col-o.col) <= 7 && abs(c.row-o.row) <= 2 {
				ok = false
				break
			}
		}
		if ok {
			n.nodes = append(n.nodes, c)
		}
	}
	// Left to right, so the graph grows across the screen.
	for i := 1; i < len(n.nodes); i++ {
		for j := i; j > 0 && n.nodes[j].col < n.nodes[j-1].col; j-- {
			n.nodes[j], n.nodes[j-1] = n.nodes[j-1], n.nodes[j]
		}
	}
	for i := 1; i < len(n.nodes); i++ {
		best := 0
		for j := 1; j < i; j++ {
			if dist(n.nodes[j], n.nodes[i]) < dist(n.nodes[best], n.nodes[i]) {
				best = j
			}
		}
		n.edges = append(n.edges, [2]int{best, i})
	}
	for range 2 {
		if len(n.nodes) < 3 {
			break
		}
		a, b := r.IntN(len(n.nodes)), r.IntN(len(n.nodes))
		if a != b && !n.hasEdge(a, b) {
			n.edges = append(n.edges, [2]int{a, b})
		}
	}
	return n
}

func (n Network) hasEdge(a, b int) bool {
	for _, e := range n.edges {
		if (e[0] == a && e[1] == b) || (e[0] == b && e[1] == a) {
			return true
		}
	}
	return false
}

// Frame is one animation state: how many nodes are shown and how far each
// edge has been drawn (0..1).
type Frame struct {
	Nodes int
	Edges []float64
}

// Frames is the timeline: each node pops in (held for a few frames), then
// every edge whose ends are both shown draws in three steps.
func (n Network) Frames() []Frame {
	prog := make([]float64, len(n.edges))
	var out []Frame
	snap := func(shown int) {
		out = append(out, Frame{Nodes: shown, Edges: append([]float64(nil), prog...)})
	}
	for shown := 1; shown <= len(n.nodes); shown++ {
		for range 3 {
			snap(shown)
		}
		for k, e := range n.edges {
			if max(e[0], e[1]) < shown && prog[k] < 1 {
				for _, t := range []float64{0.34, 0.67, 1} {
					prog[k] = t
					snap(shown)
				}
			}
		}
	}
	return out
}

// Final is the fully drawn state.
func (n Network) Final() Frame {
	prog := make([]float64, len(n.edges))
	for i := range prog {
		prog[i] = 1
	}
	return Frame{Nodes: len(n.nodes), Edges: prog}
}

var brailleBit = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// Render draws one frame as netRows lines (no trailing newlines).
func (n Network) Render(p style.Palette, f Frame) []string {
	var dots [netRows][netCols]rune
	for k, e := range n.edges {
		if k >= len(f.Edges) || f.Edges[k] <= 0 {
			continue
		}
		a, b := n.nodes[e[0]], n.nodes[e[1]]
		x0, y0 := a.col*2+1, a.row*4+2
		x1, y1 := b.col*2+1, b.row*4+2
		steps := max(1, int(math.Hypot(float64(x1-x0), float64(y1-y0))))
		upto := int(float64(steps) * min(f.Edges[k], 1))
		for s := 0; s <= upto; s++ {
			x := int(math.Round(float64(x0) + float64(x1-x0)*float64(s)/float64(steps)))
			y := int(math.Round(float64(y0) + float64(y1-y0)*float64(s)/float64(steps)))
			dots[y/4][x/2] |= brailleBit[x%2][y%4]
		}
	}
	shown := map[cell]bool{}
	for i := 0; i < f.Nodes && i < len(n.nodes); i++ {
		shown[n.nodes[i]] = true
	}
	lines := make([]string, netRows)
	for row := range netRows {
		var b strings.Builder
		b.WriteString(indent)
		for col := range netCols {
			switch {
			case shown[cell{col, row}]:
				b.WriteString(p.BrandBold("●"))
			case dots[row][col] != 0:
				b.WriteString(p.Grey(string(0x2800 + dots[row][col])))
			default:
				b.WriteByte(' ')
			}
		}
		lines[row] = strings.TrimRight(b.String(), " ")
	}
	return lines
}

// Play animates the network on w (a terminal), redrawing in place, and
// always ends on the final frame. Closing stop skips straight to it.
func (n Network) Play(w io.Writer, p style.Palette, stop <-chan struct{}) {
	frames := n.Frames()
	if len(frames) == 0 {
		return
	}
	fmt.Fprint(w, "\033[?25l") // hide the cursor while drawing
	defer fmt.Fprint(w, "\033[?25h")
	tick := time.NewTicker(PlayDuration / time.Duration(len(frames)))
	defer tick.Stop()
	draw := func(f Frame, first bool) {
		var b strings.Builder
		if !first {
			fmt.Fprintf(&b, "\033[%dA", netRows)
		}
		for _, line := range n.Render(p, f) {
			b.WriteString("\033[2K" + line + "\n")
		}
		fmt.Fprint(w, b.String())
	}
	draw(frames[0], true)
	for _, f := range frames[1:] {
		select {
		case <-stop:
			draw(n.Final(), false)
			return
		case <-tick.C:
			draw(f, false)
		}
	}
	draw(n.Final(), false)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func dist(a, b cell) float64 {
	// Rows are about twice as tall as columns are wide.
	return math.Hypot(float64(a.col-b.col), float64(2*(a.row-b.row)))
}

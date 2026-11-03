package progress

import (
	"fmt"
	"strings"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// Frames are the spinner's animation.
var Frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const barWidth = 24

// Line renders s as one line: spinner or bar, label, count, elapsed time.
func Line(s *State, frame int) string {
	elapsed := ui.Fg(ui.Muted, fmt.Sprintf("%.1fs", time.Since(s.Started).Seconds()), false)
	if s.Total <= 0 {
		line := ui.Fg(ui.Brand, Frames[frame%len(Frames)], true) + " " + s.Label + "… "
		if s.Current > 0 {
			// A count with no known total: "Decrypting notes… 312".
			if s.Bytes {
				line += Size(s.Current) + " "
			} else {
				line += Count(s.Current) + " "
			}
		}
		return line + elapsed
	}
	frac := min(float64(s.Current)/float64(s.Total), 1)
	filled := int(frac * barWidth)
	bar := ui.Fg(ui.Brand, strings.Repeat("━", filled), false) + ui.Fg(ui.Subtle, strings.Repeat("─", barWidth-filled), false)
	count := fmt.Sprintf("%s / %s", Count(s.Current), Count(s.Total))
	if s.Bytes {
		count = fmt.Sprintf("%s / %s", Size(s.Current), Size(s.Total))
	}
	return s.Label + " " + bar + " " + count + " " + elapsed
}

// Count formats n with thousands separators.
func Count(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// Size formats a byte count.
func Size(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

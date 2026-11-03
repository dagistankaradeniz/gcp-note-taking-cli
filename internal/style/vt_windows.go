//go:build windows

package style

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT turns on ANSI sequence processing for a Windows console. Older
// consoles that refuse it get no colour rather than raw escape codes.
func enableVT(f *os.File) bool {
	h := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}

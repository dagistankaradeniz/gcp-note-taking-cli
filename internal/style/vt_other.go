//go:build !windows

package style

import "os"

// enableVT is a no-op: Unix terminals understand ANSI sequences natively.
func enableVT(*os.File) bool { return true }

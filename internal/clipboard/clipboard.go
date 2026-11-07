// Package clipboard puts text on the system clipboard using the tools the
// OS already ships or commonly has, so the CLI stays dependency-free.
package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrUnavailable means there's no local clipboard to write to: over SSH
// (it would be the remote machine's), on Windows, or with no tool
// installed. Callers can fall back to asking the terminal (OSC 52).
var ErrUnavailable = errors.New("no system clipboard available")

const timeout = 2 * time.Second

// Write replaces the clipboard's contents with text.
func Write(text string) error {
	tool := find(runtime.GOOS, os.Getenv, exec.LookPath)
	if tool == nil {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool[0], tool[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// find picks the clipboard command for this machine, nil when there is
// none.
func find(goos string, env func(string) string, look func(string) (string, error)) []string {
	if env("SSH_CONNECTION") != "" || env("SSH_TTY") != "" {
		return nil
	}
	var tools [][]string
	switch goos {
	case "darwin":
		tools = [][]string{{"pbcopy"}}
	case "linux":
		if env("WAYLAND_DISPLAY") != "" {
			tools = append(tools, []string{"wl-copy"})
		}
		if env("DISPLAY") != "" {
			tools = append(tools, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		}
	}
	for _, t := range tools {
		if _, err := look(t[0]); err == nil {
			return t
		}
	}
	return nil
}

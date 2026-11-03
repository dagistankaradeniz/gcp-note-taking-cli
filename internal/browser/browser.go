// Package browser opens a sign-in page in the user's default browser, so
// `quillink login` doesn't make them copy the device-login link by hand.
package browser

import (
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
)

// Available reports whether a browser can be opened from here: not over
// SSH (it would open on the remote machine, if anywhere), and on Linux
// only with a desktop session.
func Available() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if runtime.GOOS == "linux" {
		return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	}
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

// Open starts the default browser on rawURL. Only http(s) links are
// accepted: the link comes from the server, and anything else (a file
// path, a custom scheme) must never reach the OS opener.
func Open(rawURL string) error {
	if err := check(rawURL); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reap the opener in the background; its exit status doesn't matter.
	go func() { _ = cmd.Wait() }()
	return nil
}

func check(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("not a web link")
	}
	return nil
}

package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/auth"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"golang.org/x/term"
)

// richTable reports whether the result is going to a person (the default
// table format on a terminal) rather than a script or a file.
func richTable() bool {
	return !jsonOutput && output.Current() == output.FormatTable && ui.Rich(os.Stdout)
}

// oneShotProgress is the stderr spinner/bar for a single command run from
// the shell prompt; the interactive session installs its own instead.
var oneShotProgress *progress.LineSink

// startProgress shows progress for this command on stderr when a person
// is watching it: a terminal, colour allowed, and the table format (a
// --json/--output run is for a script, which gets no extra lines).
func startProgress() {
	if shellActive || oneShotProgress != nil || output.Current() != output.FormatTable || !ui.Rich(os.Stderr) {
		return
	}
	oneShotProgress = progress.NewLineSink(os.Stderr)
	progress.SetSink(oneShotProgress)
	client.Busy = busySpinner
}

func stopProgress() {
	if oneShotProgress == nil {
		return
	}
	progress.SetSink(nil)
	oneShotProgress.Close()
	oneShotProgress = nil
	client.Busy = nil
}

// busySpinner is the spinner for a request on its own; a progress bar
// already on screen takes priority.
func busySpinner() func() {
	return progress.Busy("Contacting Quillink").Done
}

// success reports a finished action ("✓ Created note …" on a terminal,
// the plain message elsewhere).
func success(format string, a ...any) {
	ui.Success(os.Stdout, fmt.Sprintf(format, a...))
}

// printError reports a failed command, with a hint on what to do next.
func printError(err error) {
	ui.Failure(os.Stderr, err, errorHint(err))
}

func errorHint(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case 401:
			return "Your sign-in has expired or was revoked. Run `quillink login`."
		case 403:
			return "This sign-in isn't allowed to do that. Check your plan, or run `quillink login` again."
		case 404:
			return "Check the id. List commands such as `quillink note list` show yours."
		case 429:
			return "Too many requests. Wait a moment and try again."
		}
		if apiErr.Status >= 500 {
			return "Quillink had a problem. Try again in a moment."
		}
		return ""
	}
	if strings.HasPrefix(err.Error(), "unknown command") {
		return "Type `help` to see every command."
	}
	if strings.HasPrefix(err.Error(), "request failed") {
		return "Check your internet connection."
	}
	return ""
}

// loginView shows the device sign-in: on a terminal, the code large in a
// box with a spinner while it waits; elsewhere the plain lines scripts
// already know.
func loginView() auth.LoginView {
	if ui.Rich(os.Stdout) {
		return richLogin{}
	}
	return auth.PlainLogin(func(format string, a ...any) { fmt.Printf(format, a...) })
}

type richLogin struct{}

func (richLogin) ShowCode(uri, completeURI, code string, opened bool) {
	spaced := strings.Join(strings.Split(code, ""), " ")
	fields := []ui.Field{{Label: "Code", Value: ui.Fg(ui.Brand, spaced, true)}}
	if opened {
		fields = append(fields, ui.Field{Label: "Browser", Value: "opened — check the code there matches"})
	} else {
		fields = append(fields, ui.Field{Label: "Open", Value: completeURI})
	}
	fields = append(fields, ui.Field{Label: "Or go to", Value: uri})
	fmt.Print(ui.Card("Sign in to Quillink", fields))
}

func (richLogin) Waiting() func() {
	return progress.Start("Waiting for you to approve in the browser", 0).Done
}

// askSecret reads a password or key without echoing it: in the session's
// own masked input, or straight from the terminal for a one-shot command.
func askSecret(label string) (string, error) {
	if session.Active() {
		return session.AskSecret(label)
	}
	fmt.Fprint(os.Stderr, label+": ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(pw), err
}

// errStdinInSession: the session owns the keyboard, so --*-stdin can't
// read a piped value there; leaving the flag off asks instead.
func errStdinInSession(flag string) error {
	return fmt.Errorf("%s doesn't work inside the session -- leave it off and you'll be asked", flag)
}

package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// Every delete either shows what's about to go and gets a yes, or was
// given --force. TestEveryDeleteConfirms holds new ones to this.

// confirmsDelete marks a command as going through confirmDelete.
var confirmsDelete = map[string]string{"confirms-delete": "true"}

var errNeedsForce = errors.New("nothing was deleted: there's no terminal to confirm on -- add --force to delete without asking")

// canConfirm fails early when a delete can neither ask nor skip asking,
// before any prompt for a key or password.
func canConfirm() error {
	if forceFlag || session.Active() || confirmReader != nil || isTerminalStdin() {
		return nil
	}
	return errNeedsForce
}

// confirmReader stands in for the terminal in tests.
var confirmReader io.Reader

// confirmDelete shows an item and asks whether to delete it. It goes to
// stderr, as stdout belongs to the command's result. A cancelled delete
// isn't a failure.
func confirmDelete(kind string, fields []ui.Field) (bool, error) {
	if forceFlag {
		return true, nil
	}
	if err := canConfirm(); err != nil {
		return false, err
	}
	if ui.Rich(os.Stderr) {
		fmt.Fprint(os.Stderr, ui.Card("Delete this "+kind+"?", fields))
	} else {
		fmt.Fprintf(os.Stderr, "Delete this %s?\n", kind)
		for _, f := range fields {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Label, f.Value)
		}
	}
	answer, err := askConfirm("Delete? [y/N]")
	if err != nil && !errors.Is(err, session.ErrCancelled) && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	}
	ui.Notice(os.Stderr, "Cancelled. Nothing was deleted.")
	return false, nil
}

func askConfirm(label string) (string, error) {
	if session.Active() {
		return session.Ask(label)
	}
	in := confirmReader
	if in == nil {
		in = os.Stdin
	}
	fmt.Fprint(os.Stderr, label+" ")
	return bufio.NewReader(in).ReadString('\n')
}

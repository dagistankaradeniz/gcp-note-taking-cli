package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var noteUnlockPasswordStdin bool

// resolvePassword follows the Decisions Log's safe-input rules: never a
// literal flag value (persists in shell history, visible via `ps aux`).
// Masked interactive prompt, --password-stdin, or QUILLINK_NOTE_PASSWORD
// are the only accepted paths.
func resolvePassword() (string, error) {
	if noteUnlockPasswordStdin {
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	}
	if env := os.Getenv("QUILLINK_NOTE_PASSWORD"); env != "" {
		return env, nil
	}
	if isTerminalStdin() {
		fmt.Fprint(os.Stderr, "Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(pw), nil
	}
	return "", fmt.Errorf("no password provided -- use an interactive terminal, --password-stdin, or QUILLINK_NOTE_PASSWORD")
}

var noteUnlockCmd = &cobra.Command{
	Use:   "unlock <id>",
	Short: "Reveal a password-protected note",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		password, err := resolvePassword()
		if err != nil {
			return err
		}

		c := newClient()
		var resp client.NoteUnlockResponse
		req := client.NoteUnlockRequest{Password: password}
		if err := c.Do("POST", "/v1/notes/"+args[0]+"/unlock", nil, req, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		fmt.Println(bodyToText(resp.Body))
		return nil
	},
}

func init() {
	noteUnlockCmd.Flags().BoolVar(&noteUnlockPasswordStdin, "password-stdin", false, "read the password from stdin instead of prompting")
	noteCmd.AddCommand(noteUnlockCmd)
}

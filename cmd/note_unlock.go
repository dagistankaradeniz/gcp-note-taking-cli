package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notecrypto"
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

// unlockNoteBody verifies password against note id and returns its
// plaintext body. For a client-side encrypted note (the current lock
// mechanism, see `note lock`), the server only ever verifies a
// password-derived verifier hash and returns opaque ciphertext, which is
// decrypted here, client-side, exactly like the web/mobile apps do -- the
// server never learns the password or the plaintext. For a legacy
// bcrypt-gated note (pre-encryption), the server already stores the body
// as plaintext and just gates returning it on a server-side password
// check. Also used by `note delete` to verify a caller actually knows a
// locked note's password before deleting it.
func unlockNoteBody(c *client.Client, id, password string) (map[string]any, error) {
	var note client.Note
	if err := c.Do("GET", "/v1/notes/"+id, nil, nil, &note); err != nil {
		return nil, err
	}
	if !note.Locked {
		return note.Body, nil
	}

	if note.NoteEncrypted {
		if note.LockSalt == nil || note.LockIterations == nil {
			return nil, fmt.Errorf("note is missing lock metadata")
		}
		verifierHash, err := notecrypto.DeriveVerifierHash(password, *note.LockSalt, *note.LockIterations)
		if err != nil {
			return nil, err
		}
		var resp client.NoteUnlockResponse
		req := client.NoteUnlockRequest{VerifierHash: verifierHash}
		if err := c.Do("POST", "/v1/notes/"+id+"/unlock", nil, req, &resp); err != nil {
			return nil, err
		}
		key, err := notecrypto.DeriveKey(password, resp.Salt, resp.Iterations)
		if err != nil {
			return nil, err
		}
		return notecrypto.DecryptNoteBody(key, resp.EncryptedBody)
	}

	// Legacy bcrypt-gated lock: the server decrypts nothing (there was
	// never anything to decrypt) -- it just returns the always-plaintext
	// body once the password checks out.
	var resp client.NoteUnlockResponse
	req := client.NoteUnlockRequest{Password: password}
	if err := c.Do("POST", "/v1/notes/"+id+"/unlock", nil, req, &resp); err != nil {
		return nil, err
	}
	return resp.Body, nil
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
		body, err := unlockNoteBody(c, args[0], password)
		if err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(client.NoteUnlockResponse{Body: body})
		}
		fmt.Println(bodyToText(body))
		return nil
	},
}

func init() {
	noteUnlockCmd.Flags().BoolVar(&noteUnlockPasswordStdin, "password-stdin", false, "read the password from stdin instead of prompting")
	noteCmd.AddCommand(noteUnlockCmd)
}

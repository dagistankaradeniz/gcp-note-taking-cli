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

var noteLockPasswordStdin bool

// resolveNewPassword follows the same safe-input rules as resolvePassword
// (never a literal flag value), but additionally confirms the password
// by prompting twice when read interactively -- mirroring the web app's
// SetPasswordDialog -- since a typo here would lock the note behind an
// unrecoverable password with no way to reset it. Non-interactive input
// (--password-stdin/$QUILLINK_NOTE_PASSWORD) can't be confirmed this way;
// --help says so explicitly.
func resolveNewPassword() (string, error) {
	if noteLockPasswordStdin {
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
	if !isTerminalStdin() {
		return "", fmt.Errorf("no password provided -- use an interactive terminal, --password-stdin, or QUILLINK_NOTE_PASSWORD")
	}
	fmt.Fprint(os.Stderr, "New password: ")
	pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	if string(pw1) != string(pw2) {
		return "", fmt.Errorf("passwords did not match")
	}
	return string(pw1), nil
}

var noteLockCmd = &cobra.Command{
	Use:   "lock <id>",
	Short: "Password-protect a note (client-side encrypted; Plus plan or higher)",
	Long: `Encrypts the note client-side (AES-256-GCM, PBKDF2-SHA256 key derivation) --
the exact same algorithm the web/mobile apps use, so a note locked here can
be unlocked from any of them and vice versa. The backend only ever
receives the ciphertext and a separately-derived verifier hash; it never
sees the password or the plaintext body.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		password, err := resolveNewPassword()
		if err != nil {
			return err
		}

		c := newClient()

		// Fetch the current plaintext body client-side before locking --
		// the lock request itself must already carry ciphertext.
		var note client.Note
		if err := c.Do("GET", "/v1/notes/"+args[0], nil, nil, &note); err != nil {
			return err
		}
		if note.Locked {
			return fmt.Errorf("note is already locked")
		}

		salt, err := notecrypto.GenerateSalt()
		if err != nil {
			return err
		}
		key, err := notecrypto.DeriveKey(password, salt, notecrypto.DefaultIterations)
		if err != nil {
			return err
		}
		verifierHash, err := notecrypto.DeriveVerifierHash(password, salt, notecrypto.DefaultIterations)
		if err != nil {
			return err
		}
		encryptedBody, err := notecrypto.EncryptNoteBody(key, note.Body)
		if err != nil {
			return err
		}

		req := client.NoteLockRequest{
			VerifierHash:  verifierHash,
			Salt:          salt,
			Iterations:    notecrypto.DefaultIterations,
			EncryptedBody: encryptedBody,
		}
		var resp client.Note
		if err := c.Do("POST", "/v1/notes/"+args[0]+"/lock", nil, req, &resp); err != nil {
			return err
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		fmt.Println("Note locked.")
		return nil
	},
}

func init() {
	noteLockCmd.Flags().BoolVar(&noteLockPasswordStdin, "password-stdin", false, "read the new password from stdin instead of prompting (no confirmation step)")
	noteCmd.AddCommand(noteLockCmd)
}

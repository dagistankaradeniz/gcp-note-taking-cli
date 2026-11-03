package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/canvasrender"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notecrypto"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// The Vault over /v1/vault (backend app/routers/v1_vault.py), read-only.
// Its notes and canvases arrive as ciphertext; the master password is
// asked for here, the server only ever gets a verifier derived from it,
// and the Vault key is unwrapped and used on this device. In the
// interactive session the key is kept in memory until 15 min idle, `lock`
// or exit (like the Zero-Knowledge key); otherwise every command asks.

var vaultPasswordStdin bool

func init() {
	rootCmd.PersistentFlags().BoolVar(&vaultPasswordStdin, "vault-password-stdin", false, "read the Vault master password from stdin instead of prompting")
}

var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Read your Vault (decrypted on this device)",
}

// resolveVaultPassword: a masked prompt or --vault-password-stdin -- never
// a flag value or an env var.
func resolveVaultPassword() (string, error) {
	if vaultPasswordStdin {
		line, err := readStdinLine()
		if err != nil {
			return "", fmt.Errorf("read Vault password from stdin: %w", err)
		}
		return line, nil
	}
	if isTerminalStdin() {
		fmt.Fprint(os.Stderr, "Vault master password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read Vault password: %w", err)
		}
		return string(pw), nil
	}
	return "", fmt.Errorf("no Vault password provided -- use an interactive terminal or --vault-password-stdin")
}

// resolveVaultKey returns the Vault key: from the session when unlocked
// there, otherwise by asking for the master password (the web's useVault
// unlock: verifier to the server, KEK derived here, key unwrapped here).
func resolveVaultKey(c *client.Client) ([]byte, error) {
	owner := zkSessionOwner()
	if key := vaultSession.get(owner); key != nil {
		return key, nil
	}
	var status client.VaultStatusResponse
	if err := c.Do("GET", "/v1/vault/status", nil, nil, &status); err != nil {
		return nil, err
	}
	if !status.IsSetup || status.VaultSalt == nil || status.VaultIterations == nil {
		return nil, fmt.Errorf("your Vault isn't set up -- set it up in the app first")
	}
	password, err := resolveVaultPassword()
	if err != nil {
		return nil, err
	}
	verifier, err := notecrypto.DeriveVerifierHash(password, *status.VaultSalt, *status.VaultIterations)
	if err != nil {
		return nil, err
	}
	var resp client.VaultUnlockResponse
	if err := c.Do("POST", "/v1/vault/unlock", nil, client.VaultUnlockRequest{PasswordHash: verifier}, &resp); err != nil {
		return nil, fmt.Errorf("unlock Vault (check the password): %w", err)
	}
	kek, err := notecrypto.DeriveKey(password, resp.VaultSalt, resp.VaultIterations)
	if err != nil {
		return nil, err
	}
	key, err := zkcrypto.UnwrapKey(kek, resp.WrappedDek, resp.WrappedDekIV)
	clear(kek)
	if err != nil {
		return nil, fmt.Errorf("unwrap Vault key (check the password): %w", err)
	}
	if shellActive {
		vaultSession.put(owner, key)
		fmt.Fprintf(os.Stderr, "Vault unlocked for this session. It locks after %d min idle, on `lock`, or on exit.\n", int(zkSessionIdle.Minutes()))
	}
	return key, nil
}

// decryptVaultNote opens title and body, all or nothing; a note that
// doesn't open is marked unreadable and shows none of the server's text.
func decryptVaultNote(key []byte, n *client.VaultNote) {
	unreadable := func() {
		n.Title, n.Body, n.Unreadable = zkUnreadableText, nil, true
	}
	title, err := notecrypto.DecryptString(key, n.Title)
	if err != nil {
		unreadable()
		return
	}
	var body map[string]any
	if n.Body != nil {
		raw, err := json.Marshal(n.Body)
		if err != nil {
			unreadable()
			return
		}
		if body, err = notecrypto.DecryptNoteBody(key, string(raw)); err != nil {
			unreadable()
			return
		}
	}
	n.Title, n.Body = title, body
}

// decryptVaultCanvas opens the title and returns the raw elements, or
// marks the canvas unreadable.
func decryptVaultCanvas(key []byte, cv *client.VaultCanvas) []any {
	unreadable := func() []any {
		cv.Title, cv.Elements, cv.Unreadable = zkUnreadableText, nil, true
		return nil
	}
	title, err := notecrypto.DecryptString(key, cv.Title)
	if err != nil {
		return unreadable()
	}
	cv.Title = title
	if cv.Elements == nil || *cv.Elements == "" {
		cv.Elements = nil
		return []any{}
	}
	plain, err := notecrypto.DecryptString(key, *cv.Elements)
	if err != nil {
		return unreadable()
	}
	var raw []any
	if err := json.Unmarshal([]byte(plain), &raw); err != nil {
		return unreadable()
	}
	cv.Elements = nil
	return raw
}

// vaultContents is the whole Vault, decrypted.
type vaultContents struct {
	notes    []client.VaultNote
	canvases []client.VaultCanvas
	elements map[string][]any
}

func loadVault(c *client.Client, key []byte) (*vaultContents, error) {
	var notes client.VaultNoteListResponse
	if err := c.Do("GET", "/v1/vault/notes", nil, nil, &notes); err != nil {
		return nil, err
	}
	var canvases client.VaultCanvasListResponse
	if err := c.Do("GET", "/v1/vault/canvases", nil, nil, &canvases); err != nil {
		return nil, err
	}
	v := &vaultContents{notes: notes.Notes, canvases: canvases.Canvases, elements: map[string][]any{}}
	for i := range v.notes {
		decryptVaultNote(key, &v.notes[i])
	}
	for i := range v.canvases {
		if raw := decryptVaultCanvas(key, &v.canvases[i]); raw != nil {
			v.elements[v.canvases[i].ID] = raw
		}
	}
	return v, nil
}

func (v *vaultContents) note(id string) *client.VaultNote {
	for i := range v.notes {
		if v.notes[i].ID == id {
			return &v.notes[i]
		}
	}
	return nil
}

func (v *vaultContents) canvas(id string) *client.VaultCanvas {
	for i := range v.canvases {
		if v.canvases[i].ID == id {
			return &v.canvases[i]
		}
	}
	return nil
}

func openVault() (*client.Client, *vaultContents, error) {
	c := newClient()
	key, err := resolveVaultKey(c)
	if err != nil {
		return nil, nil, err
	}
	v, err := loadVault(c, key)
	return c, v, err
}

var vaultUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the Vault for this session",
	Long: `Asks for the Vault master password and keeps the Vault key in memory for
this interactive session (15 min idle, ` + "`lock`" + ` or exit clear it). Outside the
session it only checks the password.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := resolveVaultKey(newClient()); err != nil {
			return err
		}
		if !shellActive {
			fmt.Fprintln(os.Stderr, "Password OK. Run `quillink` with no arguments to keep the Vault unlocked for a session.")
		}
		return nil
	},
}

var vaultLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the Vault (clear its key from memory)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultSession.wipe() {
			fmt.Fprintln(os.Stderr, "Vault locked.")
		} else {
			fmt.Fprintln(os.Stderr, "The Vault isn't unlocked.")
		}
		return nil
	},
}

var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the notes and canvases in your Vault",
	Example: `  quillink vault list
  quillink vault list --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, v, err := openVault()
		if err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(struct {
				Notes    []client.VaultNote   `json:"notes"`
				Canvases []client.VaultCanvas `json:"canvases"`
			}{v.notes, v.canvases})
		}
		rows := make([][]string, 0, len(v.notes)+len(v.canvases))
		for _, n := range v.notes {
			rows = append(rows, []string{n.ID, "note", orDefault(n.Title, "Untitled note"), n.UpdatedAt})
		}
		for _, cv := range v.canvases {
			rows = append(rows, []string{cv.ID, "canvas", orDefault(cv.Title, "Untitled canvas"), cv.UpdatedAt})
		}
		output.Table([]string{"ID", "TYPE", "TITLE", "UPDATED"}, rows)
		return nil
	},
}

var vaultGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show a Vault note (or a canvas's details)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, v, err := openVault()
		if err != nil {
			return err
		}
		if n := v.note(args[0]); n != nil {
			if jsonOutput {
				return output.JSON(n)
			}
			fmt.Printf("%s\n\n%s\n", n.Title, bodyToText(n.Body))
			return nil
		}
		cv := v.canvas(args[0])
		if cv == nil {
			return &client.APIError{Status: 404, Title: "Not Found", Detail: "no Vault note or canvas with that id"}
		}
		elements := canvasrender.NormalizeElements(v.elements[cv.ID])
		if jsonOutput {
			out := make([]any, len(elements))
			for i, el := range elements {
				out[i] = el
			}
			return output.JSON(struct {
				client.VaultCanvas
				Elements []any `json:"elements"`
			}{*cv, out})
		}
		counts := map[string]int{}
		for _, el := range elements {
			counts[el["type"].(string)]++
		}
		fmt.Printf("%s\n\nUpdated:   %s\nElements:  %d%s\n", cv.Title, cv.UpdatedAt, len(elements), elementSummary(counts))
		return nil
	},
}

var (
	vaultExportOut    string
	vaultExportFormat string
	vaultExportScale  float64
	vaultExportDark   bool
	vaultExportForce  bool
)

var vaultExportCmd = &cobra.Command{
	Use:   "export <id>",
	Short: "Export a Vault note as Markdown, or a canvas as PNG/JPEG",
	Example: `  quillink vault export <note-id> --out note.md
  quillink vault export <canvas-id> --out board.png`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultExportOut == "" {
			return fmt.Errorf("--out is required (a file, or - for stdout)")
		}
		if vaultExportOut != "-" {
			if _, err := os.Stat(vaultExportOut); err == nil && !vaultExportForce {
				return fmt.Errorf("%s already exists -- add --force to overwrite it", vaultExportOut)
			}
		}
		_, v, err := openVault()
		if err != nil {
			return err
		}
		if n := v.note(args[0]); n != nil {
			if n.Unreadable {
				return fmt.Errorf("this note couldn't be decrypted")
			}
			md := noteMarkdown(client.Note{Title: n.Title, Body: n.Body}, "\n")
			return writeImage(vaultExportOut, func(w io.Writer) error {
				_, err := io.WriteString(w, md)
				return err
			})
		}
		cv := v.canvas(args[0])
		if cv == nil {
			return &client.APIError{Status: 404, Title: "Not Found", Detail: "no Vault note or canvas with that id"}
		}
		if cv.Unreadable {
			return fmt.Errorf("this canvas couldn't be decrypted")
		}
		format := vaultExportFormat
		if format == "" {
			switch {
			case strings.HasSuffix(strings.ToLower(vaultExportOut), ".png"):
				format = "png"
			case strings.HasSuffix(strings.ToLower(vaultExportOut), ".jpg"), strings.HasSuffix(strings.ToLower(vaultExportOut), ".jpeg"):
				format = "jpeg"
			default:
				return fmt.Errorf("can't tell the image format from %q -- add --format png or --format jpeg", vaultExportOut)
			}
		}
		palette := canvasrender.LightPalette
		if vaultExportDark {
			palette = canvasrender.DarkPalette
		}
		img, err := canvasrender.Render(canvasrender.NormalizeElements(v.elements[cv.ID]), canvasrender.Options{
			Scale:   vaultExportScale,
			Palette: palette,
			// Note/task cards keep their generic label: resolving titles
			// would reach outside the Vault.
		})
		if err != nil {
			return err
		}
		return writeImage(vaultExportOut, func(w io.Writer) error { return canvasrender.Encode(w, img, format) })
	},
}

func init() {
	vaultExportCmd.Flags().StringVar(&vaultExportOut, "out", "", "file to write (.md for a note; .png/.jpg for a canvas), or - for stdout")
	vaultExportCmd.Flags().StringVar(&vaultExportFormat, "format", "", "canvas only: png or jpeg (default: from the --out extension)")
	vaultExportCmd.Flags().Float64Var(&vaultExportScale, "scale", 2, "canvas only: pixels per canvas unit")
	vaultExportCmd.Flags().BoolVar(&vaultExportDark, "dark", false, "canvas only: the app's dark theme colours")
	vaultExportCmd.Flags().BoolVar(&vaultExportForce, "force", false, "overwrite an existing file")
	vaultCmd.AddCommand(vaultUnlockCmd, vaultLockCmd, vaultListCmd, vaultGetCmd, vaultExportCmd)
	rootCmd.AddCommand(vaultCmd)
}

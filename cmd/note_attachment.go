package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notecrypto"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
	"github.com/spf13/cobra"
)

// Read-only, deliberately: v1 has no upload endpoint yet (see backend's
// app/routers/v1_attachments.py module docstring) -- list/download only.
// Encrypted attachments are decrypted here, like the web's
// downloadAttachmentFile: a Zero-Knowledge account's with the account key,
// a locked note's with the key from its password.
var noteAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "View a note's file attachments",
}

// attachmentDownloadMaxBytes bounds one download held in memory for
// decryption (the app's own per-file upload limit is far below it).
const attachmentDownloadMaxBytes = 1 << 30

var noteAttachmentListCmd = &cobra.Command{
	Use:   "list <note-id>",
	Short: "List a note's attachments",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var resp client.AttachmentListResponse
		if err := c.Do("GET", "/v1/notes/"+url.PathEscape(args[0])+"/attachments", nil, nil, &resp); err != nil {
			return err
		}
		for i := range resp.Attachments {
			decryptAttachmentZK(dek, &resp.Attachments[i])
		}

		if jsonOutput {
			return output.JSON(resp)
		}
		rows := make([][]string, 0, len(resp.Attachments))
		for _, a := range resp.Attachments {
			locked := "no"
			if a.ZKEncrypted || a.LockEncrypted {
				locked = "yes"
			}
			rows = append(rows, []string{a.ID, a.OriginalFilename, a.MimeType, fmt.Sprintf("%d", a.SizeBytes), locked})
		}
		output.Table([]string{"ID", "FILENAME", "MIME TYPE", "SIZE", "ENCRYPTED"}, rows)
		return nil
	},
}

var (
	noteAttachmentDownloadOut           string
	noteAttachmentDownloadPasswordStdin bool
)

var noteAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <note-id> <attachment-id>",
	Short: "Download an attachment",
	Example: `  quillink note attachment download <note-id> <attachment-id> --out report.pdf
  quillink note attachment download <locked-note-id> <attachment-id> --out scan.png --password-stdin`,
	Long: `Downloads an attachment's file. An encrypted attachment is decrypted on
this device first: a Zero-Knowledge account's with your account key, a
password-protected note's with the note's password (asked for, or
--password-stdin / QUILLINK_NOTE_PASSWORD), a Vault note's with the Vault
master password (or --vault-password-stdin). The server never sees the
file or a password.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		noteID, attachmentID := args[0], args[1]
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var list client.AttachmentListResponse
		if err := c.Do("GET", "/v1/notes/"+url.PathEscape(noteID)+"/attachments", nil, nil, &list); err != nil {
			return err
		}
		var att *client.Attachment
		for i := range list.Attachments {
			if list.Attachments[i].ID == attachmentID {
				att = &list.Attachments[i]
			}
		}
		if att == nil {
			return &client.APIError{Status: 404, Title: "Not Found", Detail: "attachment not found on this note"}
		}

		// Work out the key before downloading anything.
		var open func([]byte) ([]byte, error)
		switch {
		case dek != nil:
			// A ZK account only ever uploads encrypted files; a plain one
			// was put there by someone else.
			if !att.ZKEncrypted {
				return fmt.Errorf("this attachment can't be decrypted with your key")
			}
			open, err = zkAttachmentOpener(dek, *att)
		case att.ZKEncrypted:
			return fmt.Errorf("this attachment belongs to a Zero-Knowledge account and can't be decrypted here")
		case att.LockEncrypted:
			open, err = lockedAttachmentOpener(c, noteID, att)
		default:
			open = func(b []byte) ([]byte, error) { return b, nil }
		}
		if err != nil {
			return err
		}

		var resp client.AttachmentDownloadResponse
		if err := c.Do("GET", "/v1/notes/"+url.PathEscape(noteID)+"/attachments/"+url.PathEscape(attachmentID)+"/download", nil, nil, &resp); err != nil {
			return err
		}
		// The signed URL itself carries its own short-lived auth (see
		// AttachmentDownloadResponse.expires_in) -- fetched directly,
		// without the bearer token this CLI uses for /v1 calls.
		httpClient := &http.Client{Timeout: 5 * time.Minute}
		fileResp, err := httpClient.Get(resp.DownloadURL)
		if err != nil {
			return fmt.Errorf("download attachment: %w", err)
		}
		defer fileResp.Body.Close()
		if fileResp.StatusCode >= 400 {
			return fmt.Errorf("download attachment: signed URL returned %d", fileResp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(fileResp.Body, attachmentDownloadMaxBytes+1))
		if err != nil {
			return fmt.Errorf("download attachment: %w", err)
		}
		if len(data) > attachmentDownloadMaxBytes {
			return fmt.Errorf("attachment is larger than %d bytes", attachmentDownloadMaxBytes)
		}
		plain, err := open(data)
		if err != nil {
			return err
		}
		if noteAttachmentDownloadOut == "" {
			_, err = os.Stdout.Write(plain)
			return err
		}
		if err := os.WriteFile(noteAttachmentDownloadOut, plain, 0o600); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Saved %s to %s\n", att.OriginalFilename, noteAttachmentDownloadOut)
		return nil
	},
}

type attachmentMetadata struct {
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
}

// decryptAttachmentZK puts a ZK attachment's real name and type on it (the
// server only stores placeholders); one that doesn't open is marked
// unreadable -- the web's decryptAttachmentRead.
func decryptAttachmentZK(dek []byte, a *client.Attachment) {
	if dek == nil {
		return
	}
	if a.ZKEncrypted && a.EncryptedMetadata != nil {
		var meta attachmentMetadata
		plain, err := zkcrypto.NewOpener(dek, "attachment").String("metadata", *a.EncryptedMetadata)
		if err == nil && json.Unmarshal([]byte(plain), &meta) == nil {
			a.OriginalFilename, a.MimeType = meta.Filename, meta.MimeType
			return
		}
	}
	a.OriginalFilename, a.MimeType, a.ZkUnreadable = zkUnreadableText, "application/octet-stream", true
}

func zkAttachmentOpener(dek []byte, a client.Attachment) (func([]byte) ([]byte, error), error) {
	if a.EncryptedMetadata == nil || a.WrappedFileDek == nil || a.WrappedFileDekIV == nil {
		return nil, fmt.Errorf("this attachment can't be decrypted with your key")
	}
	opener := zkcrypto.NewOpener(dek, "attachment")
	var meta attachmentMetadata
	plain, err := opener.String("metadata", *a.EncryptedMetadata)
	if err != nil || json.Unmarshal([]byte(plain), &meta) != nil {
		return nil, fmt.Errorf("this attachment can't be decrypted with your key")
	}
	fileKey, err := zkcrypto.UnwrapFileKey(dek, *a.WrappedFileDek, *a.WrappedFileDekIV, opener.Bind())
	if err != nil {
		return nil, fmt.Errorf("this attachment can't be decrypted with your key")
	}
	return func(data []byte) ([]byte, error) {
		decrypted, err := zkcrypto.DecryptFile(fileKey, data)
		if err != nil {
			return nil, err
		}
		return zkcrypto.UnpadFile(decrypted)
	}, nil
}

// lockedAttachmentOpener asks for the note's password, unlocks it (the
// server checks a derived verifier only) and returns a decrypter using the
// note's lock key.
func lockedAttachmentOpener(c *client.Client, noteID string, a *client.Attachment) (func([]byte) ([]byte, error), error) {
	if a.WrappedFileDek == nil || a.WrappedFileDekIV == nil {
		return nil, fmt.Errorf("this attachment is missing its key")
	}
	var note client.Note
	if err := c.Do("GET", "/v1/notes/"+url.PathEscape(noteID), nil, nil, &note); err != nil {
		return nil, err
	}
	var lockKey []byte
	switch {
	case note.VaultEncrypted:
		// A Vault note's attachments are wrapped with the Vault key.
		key, err := resolveVaultKey(c)
		if err != nil {
			return nil, err
		}
		lockKey = key
	case note.NoteEncrypted:
		noteUnlockPasswordStdin = noteAttachmentDownloadPasswordStdin
		password, err := resolvePassword()
		if err != nil {
			return nil, err
		}
		if lockKey, _, err = unlockEncryptedNote(c, note, password); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("this attachment's note isn't locked any more, so its key is gone -- download it in the app")
	}
	fileKey, err := zkcrypto.UnwrapKey(lockKey, *a.WrappedFileDek, *a.WrappedFileDekIV)
	if err != nil {
		return nil, fmt.Errorf("this attachment can't be decrypted with the note's key")
	}
	if a.EncryptedMetadata != nil {
		if plain, err := notecrypto.DecryptString(lockKey, *a.EncryptedMetadata); err == nil {
			var meta attachmentMetadata
			if json.Unmarshal([]byte(plain), &meta) == nil {
				a.OriginalFilename = meta.Filename
			}
		}
	}
	return func(data []byte) ([]byte, error) { return zkcrypto.DecryptFile(fileKey, data) }, nil
}

func init() {
	noteAttachmentDownloadCmd.Flags().StringVar(&noteAttachmentDownloadOut, "out", "", "write to this file instead of stdout")
	noteAttachmentDownloadCmd.Flags().BoolVar(&noteAttachmentDownloadPasswordStdin, "password-stdin", false, "for a password-protected note: read its password from stdin")
	noteAttachmentCmd.AddCommand(noteAttachmentListCmd, noteAttachmentDownloadCmd)
	noteCmd.AddCommand(noteAttachmentCmd)
}

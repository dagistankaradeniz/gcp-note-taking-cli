package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

// Read-only, deliberately: v1 has no upload endpoint yet (see backend's
// app/routers/v1_attachments.py module docstring) -- list/download only.
var noteAttachmentCmd = &cobra.Command{
	Use:   "attachment",
	Short: "View a note's file attachments",
}

var noteAttachmentListCmd = &cobra.Command{
	Use:   "list <note-id>",
	Short: "List a note's attachments",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.AttachmentListResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/attachments", nil, nil, &resp); err != nil {
			return err
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

var noteAttachmentDownloadOut string

var noteAttachmentDownloadCmd = &cobra.Command{
	Use:   "download <note-id> <attachment-id>",
	Short: "Download an attachment",
	Long: `Downloads an attachment's file bytes. A ZK/Vault/lock-encrypted note's
attachment downloads as still-opaque ciphertext -- this CLI has no decrypt
path for it, same as it has none for a locked note's body.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		var resp client.AttachmentDownloadResponse
		if err := c.Do("GET", "/v1/notes/"+args[0]+"/attachments/"+args[1]+"/download", nil, nil, &resp); err != nil {
			return err
		}

		// The signed URL itself carries its own short-lived auth (see
		// AttachmentDownloadResponse.expires_in) -- fetched directly,
		// without the bearer token this CLI uses for /v1 calls.
		httpClient := &http.Client{Timeout: 60 * time.Second}
		fileResp, err := httpClient.Get(resp.DownloadURL)
		if err != nil {
			return fmt.Errorf("download attachment: %w", err)
		}
		defer fileResp.Body.Close()
		if fileResp.StatusCode >= 400 {
			return fmt.Errorf("download attachment: signed URL returned %d", fileResp.StatusCode)
		}

		out := os.Stdout
		if noteAttachmentDownloadOut != "" {
			f, err := os.Create(noteAttachmentDownloadOut)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()
			out = f
		}
		_, err = io.Copy(out, fileResp.Body)
		return err
	},
}

func init() {
	noteAttachmentDownloadCmd.Flags().StringVar(&noteAttachmentDownloadOut, "out", "", "write to this file instead of stdout")
	noteAttachmentCmd.AddCommand(noteAttachmentListCmd, noteAttachmentDownloadCmd)
	noteCmd.AddCommand(noteAttachmentCmd)
}

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notemd"
	"github.com/spf13/cobra"
)

var (
	noteExportFormat   string
	noteExportAll      bool
	noteExportFolderID string
	noteExportTags     []string
	noteExportMatch    string
	noteExportOut      string
)

// GET /v1/notes/{id}/export and the bulk GET /v1/notes/export -- both
// return plain text (markdown) or NDJSON, not a single JSON document, so
// this goes through Client.DoRaw rather than the usual Do/--json path.
// --json still selects ndjson as the bulk format's natural counterpart,
// but the output itself is never wrapped in the --json envelope (see
// internal/output.JSON) -- it's a file to redirect/pipe, not a value to
// script against.
var noteExportCmd = &cobra.Command{
	Use:   "export [id]",
	Short: "Export a note (or, with --all, every note) as Markdown or NDJSON",
	Example: `  quillink note export <id> --out note.md
  quillink note export --all --format markdown --out notes.md
  quillink note export --all --tags work --out work.ndjson`,
	Long: `Export a single note as a standalone Markdown file, or every active note
at once with --all (Markdown: one file's worth of "# Title" blocks; NDJSON:
one full note object per line, for backups/scripting). Writes to stdout by
default -- redirect or use --out.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && !noteExportAll {
			return fmt.Errorf("pass a note id, or --all to export every note")
		}
		if len(args) == 1 && noteExportAll {
			return fmt.Errorf("pass either a note id or --all, not both")
		}

		c := newClient()
		// A Zero-Knowledge account's notes are ciphertext to the server, so
		// they're decrypted and rendered here instead (the same Markdown as
		// the server's export, see internal/notemd).
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		if dek != nil {
			data, err := exportZk(c, dek, args)
			if err != nil {
				return err
			}
			return writeExport(data)
		}

		var data []byte
		if noteExportAll {
			if noteExportFormat == "" {
				noteExportFormat = "ndjson"
			}
			q := url.Values{}
			q.Set("format", noteExportFormat)
			if noteExportFolderID != "" {
				q.Set("folder_id", noteExportFolderID)
			}
			if err := addTagFilter(q, noteExportTags, noteExportMatch); err != nil {
				return err
			}
			data, err = c.DoRaw("/v1/notes/export", q)
		} else {
			data, err = c.DoRaw("/v1/notes/"+args[0]+"/export", nil)
		}
		if err != nil {
			return err
		}

		return writeExport(data)
	},
}

func writeExport(data []byte) error {
	if noteExportOut != "" {
		return os.WriteFile(noteExportOut, data, 0o600)
	}
	_, err := os.Stdout.Write(data)
	return err
}

// exportZk is note export for a Zero-Knowledge account: the notes are read
// and decrypted here and written in the server export's own formats. A
// note that doesn't decrypt stops the export rather than writing a
// placeholder where the user's content should be.
func exportZk(c *client.Client, dek []byte, args []string) ([]byte, error) {
	if !noteExportAll {
		var n client.Note
		if err := c.Do("GET", "/v1/notes/"+url.PathEscape(args[0]), nil, nil, &n); err != nil {
			return nil, err
		}
		if n.VaultEncrypted {
			return nil, fmt.Errorf("this note is in the Vault -- use quillink vault export")
		}
		decryptNoteZK(dek, &n)
		if n.Title == zkUnreadableText {
			return nil, fmt.Errorf("note %s couldn't be decrypted on this device", n.ID)
		}
		return []byte(noteMarkdown(n, "\n")), nil
	}

	format := noteExportFormat
	if format == "" {
		format = "ndjson"
	}
	if format != "ndjson" && format != "markdown" {
		return nil, fmt.Errorf("--format must be ndjson or markdown")
	}
	if noteExportMatch != "all" && noteExportMatch != "any" {
		return nil, fmt.Errorf("--match must be all or any, got %q", noteExportMatch)
	}
	scan, err := scanZk(c, dek, false)
	if err != nil {
		return nil, err
	}
	defer scan.warnIfPartial()
	notes := scan.notes
	if len(noteExportTags) > 0 {
		notes = scan.withTags(noteExportTags, noteExportMatch == "all")
	}
	var buf bytes.Buffer
	for _, n := range notes {
		// The server's bulk export leaves Vault notes out too.
		if n.VaultEncrypted || (noteExportFolderID != "" && (n.FolderID == nil || *n.FolderID != noteExportFolderID)) {
			continue
		}
		if n.Title == zkUnreadableText {
			return nil, fmt.Errorf("note %s couldn't be decrypted on this device -- nothing was exported", n.ID)
		}
		if format == "markdown" {
			buf.WriteString(noteMarkdown(n, "\n\n---\n\n"))
			continue
		}
		line, err := json.Marshal(n)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// noteMarkdown is one note as the server's export writes it.
func noteMarkdown(n client.Note, end string) string {
	title := orDefault(n.Title, "Untitled")
	if n.Locked || n.NoteEncrypted {
		return "# " + title + "\n\n_(locked -- password required, not exported)_" + end
	}
	return "# " + title + "\n\n" + notemd.ToMarkdown(n.Body) + end
}

func init() {
	noteExportCmd.Flags().StringVar(&noteExportFormat, "format", "", "ndjson or markdown (--all only; default ndjson)")
	noteExportCmd.Flags().BoolVar(&noteExportAll, "all", false, "export every active note instead of a single note")
	noteExportCmd.Flags().StringVar(&noteExportFolderID, "folder", "", "--all only: limit to this folder ID")
	noteExportCmd.Flags().StringSliceVar(&noteExportTags, "tags", nil, "--all only: limit to notes with these tags (comma-separated; see --match)")
	noteExportCmd.Flags().StringVar(&noteExportMatch, "match", "any", "with --tags -- any: at least one tag; all: every tag")
	noteExportCmd.Flags().StringVar(&noteExportOut, "out", "", "write to this file instead of stdout")
	noteCmd.AddCommand(noteExportCmd)
}

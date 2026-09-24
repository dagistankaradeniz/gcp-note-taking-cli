package cmd

import (
	"fmt"
	"net/url"
	"os"

	"github.com/spf13/cobra"
)

var (
	noteExportFormat   string
	noteExportAll      bool
	noteExportFolderID string
	noteExportTags     []string
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
		var data []byte
		var err error
		if noteExportAll {
			if noteExportFormat == "" {
				noteExportFormat = "ndjson"
			}
			q := url.Values{}
			q.Set("format", noteExportFormat)
			if noteExportFolderID != "" {
				q.Set("folder_id", noteExportFolderID)
			}
			for _, t := range noteExportTags {
				q.Add("tags", t)
			}
			data, err = c.DoRaw("/v1/notes/export", q)
		} else {
			data, err = c.DoRaw("/v1/notes/"+args[0]+"/export", nil)
		}
		if err != nil {
			return err
		}

		if noteExportOut != "" {
			return os.WriteFile(noteExportOut, data, 0o600)
		}
		_, err = os.Stdout.Write(data)
		return err
	},
}

func init() {
	noteExportCmd.Flags().StringVar(&noteExportFormat, "format", "", "ndjson or markdown (--all only; default ndjson)")
	noteExportCmd.Flags().BoolVar(&noteExportAll, "all", false, "export every active note instead of a single note")
	noteExportCmd.Flags().StringVar(&noteExportFolderID, "folder", "", "--all only: limit to this folder ID")
	noteExportCmd.Flags().StringSliceVar(&noteExportTags, "tags", nil, "--all only: limit to notes with any of these tags (comma-separated)")
	noteExportCmd.Flags().StringVar(&noteExportOut, "out", "", "write to this file instead of stdout")
	noteCmd.AddCommand(noteExportCmd)
}

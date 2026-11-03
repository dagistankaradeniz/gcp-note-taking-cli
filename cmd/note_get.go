package cmd

import (
	"fmt"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notemd"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

var noteGetIncludeSensitive bool

var noteGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Fetch a single note",
	Example: `  quillink note get <id>
  quillink note get <id> --output yaml
  quillink note get <id> --out note.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var note client.Note
		if err := c.Do("GET", "/v1/notes/"+args[0], nil, nil, &note); err != nil {
			return err
		}
		decryptNoteZK(dek, &note)
		applySensitiveMasking([]client.Note{note}, noteGetIncludeSensitive)

		if jsonOutput {
			return output.JSON(note)
		}
		printNote(note.Title, note.Body, noteMeta(note))
		return nil
	},
}

// printNote shows a note: on a terminal as rendered Markdown (headings,
// lists, highlighted code) under a dimmed details line; elsewhere as the
// plain title and text it always was.
func printNote(title string, body map[string]any, meta string) {
	if richTable() {
		md := "# " + orDefault(title, "Untitled") + "\n\n" + notemd.ToMarkdown(body)
		if out, err := ui.Markdown(md); err == nil {
			if meta != "" {
				fmt.Println(ui.Render("  " + ui.Fg(ui.Muted, meta, false)))
			}
			fmt.Print(out)
			return
		}
	}
	fmt.Printf("%s\n\n%s\n", title, bodyToText(body))
}

// noteMeta is the details line above a rendered note.
func noteMeta(n client.Note) string {
	parts := []string{}
	if n.Pinned {
		parts = append(parts, "📌 pinned")
	}
	if len(n.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(n.Tags, " #"))
	}
	if n.UpdatedAt != "" {
		parts = append(parts, "updated "+ui.Relative(n.UpdatedAt))
	}
	return strings.Join(parts, " · ")
}

func init() {
	noteGetCmd.Flags().BoolVar(&noteGetIncludeSensitive, "include-sensitive", false, "include masked/sensitive fields unmasked")
	noteCmd.AddCommand(noteGetCmd)
}

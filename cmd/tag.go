package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/spf13/cobra"
)

// tags aren't a standalone resource -- they're just a field on each note
// doc (see backend's app/routers/v1_tags.py) -- so there's nothing to
// create/update/delete independent of a note's own `tags` field. `tag
// list` reads the account's tag index; `tag show` lists the notes carrying
// given tags (same as `note list --tags`).
var tagCmd = &cobra.Command{
	Use:   "tag",
	Short: "View tags and the notes that carry them",
}

var (
	tagListCounts bool
	tagShowMatch  string
	tagShowLimit  int
)

var tagListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every tag used across your active notes",
	Long: `List every tag used across your active notes. With --counts, also show
how many notes and canvases carry each tag, most-used first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		if tagListCounts {
			var counts []client.TagCount
			if err := c.Do("GET", "/v1/tags/counts", nil, nil, &counts); err != nil {
				return err
			}
			if jsonOutput {
				return output.JSON(counts)
			}
			rows := make([][]string, 0, len(counts))
			for _, t := range counts {
				rows = append(rows, []string{t.Name, strconv.Itoa(t.Notes), strconv.Itoa(t.Canvases)})
			}
			output.Table([]string{"TAG", "NOTES", "CANVASES"}, rows)
			return nil
		}

		var tags []string
		if err := c.Do("GET", "/v1/tags", nil, nil, &tags); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(tags)
		}
		for _, t := range tags {
			fmt.Println(t)
		}
		return nil
	},
}

var tagShowCmd = &cobra.Command{
	Use:   "show <tag> [tag...]",
	Short: "List the notes carrying these tags (all of them by default)",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(tagShowLimit))
		if err := addTagFilter(q, args, tagShowMatch); err != nil {
			return err
		}
		c := newClient()
		var resp client.NoteListResponse
		if err := c.Do("GET", "/v1/notes", q, nil, &resp); err != nil {
			return err
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		printNoteTable(resp.Notes)
		if resp.HasMore && len(resp.Notes) > 0 {
			last := resp.Notes[len(resp.Notes)-1]
			fmt.Fprintf(os.Stderr, "\nMore notes available. Continue with:\n  quillink note list --tags %s --match %s --start-after %s\n", joinTags(args), tagShowMatch, last.ID)
		}
		return nil
	},
}

// addTagFilter sets the /v1 tag filter query params: tags (repeated) and
// tag_match, "all" (every tag) or "any" (at least one).
func addTagFilter(q url.Values, tags []string, match string) error {
	if len(tags) == 0 {
		return nil
	}
	if match != "all" && match != "any" {
		return fmt.Errorf("--match must be all or any, got %q", match)
	}
	for _, t := range tags {
		q.Add("tags", t)
	}
	q.Set("tag_match", match)
	return nil
}

func joinTags(tags []string) string { return strings.Join(tags, ",") }

func init() {
	tagListCmd.Flags().BoolVar(&tagListCounts, "counts", false, "also show how many notes and canvases carry each tag")
	tagShowCmd.Flags().StringVar(&tagShowMatch, "match", "all", "all: notes with every tag; any: notes with at least one")
	tagShowCmd.Flags().IntVar(&tagShowLimit, "limit", 50, "max notes to return")
	tagCmd.AddCommand(tagListCmd)
	tagCmd.AddCommand(tagShowCmd)
	rootCmd.AddCommand(tagCmd)
}

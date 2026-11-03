package cmd

import (
	"fmt"
	"net/url"
	"os"
	"sort"
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
	Example: `  quillink tag list --counts
  quillink tag list --counts --output csv --out tags.csv`,
	Long: `List every tag used across your active notes. With --counts, also show
how many notes and canvases carry each tag, most-used first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		if dek != nil {
			return printZkTagList(c, dek)
		}
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
	Example: `  quillink tag show work urgent
  quillink tag show work urgent --match any --output csv --out tagged.csv`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(tagShowLimit))
		if err := addTagFilter(q, args, tagShowMatch); err != nil {
			return err
		}
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		if dek != nil {
			return printZkTagged(c, dek, args, tagShowMatch == "all", tagShowLimit, "")
		}
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

// printZkTagList is `tag list` for a Zero-Knowledge account, counted from
// the account's notes decrypted here (canvases aren't counted).
func printZkTagList(c *client.Client, dek []byte) error {
	scan, err := scanZk(c, dek, false)
	if err != nil {
		return err
	}
	counts := scan.tagCounts()
	defer scan.warnIfPartial()
	if tagListCounts {
		if jsonOutput {
			return output.JSON(counts)
		}
		rows := make([][]string, 0, len(counts))
		for _, t := range counts {
			rows = append(rows, []string{t.Name, strconv.Itoa(t.Notes), "-"})
		}
		output.Table([]string{"TAG", "NOTES", "CANVASES"}, rows)
		return nil
	}
	names := make([]string, 0, len(counts))
	for _, t := range counts {
		names = append(names, t.Name)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	if jsonOutput {
		return output.JSON(names)
	}
	for _, t := range names {
		fmt.Println(t)
	}
	return nil
}

// printZkTagged lists a Zero-Knowledge account's notes carrying tags,
// filtered here after decrypting (the server only sees ciphertext tags).
func printZkTagged(c *client.Client, dek []byte, tags []string, matchAll bool, limit int, folderID string) error {
	scan, err := scanZk(c, dek, false)
	if err != nil {
		return err
	}
	notes := scan.withTags(tags, matchAll)
	if folderID != "" {
		kept := notes[:0]
		for _, n := range notes {
			if n.FolderID != nil && *n.FolderID == folderID {
				kept = append(kept, n)
			}
		}
		notes = kept
	}
	hasMore := len(notes) > limit
	if hasMore {
		notes = notes[:limit]
	}
	defer scan.warnIfPartial()
	if jsonOutput {
		return output.JSON(client.NoteListResponse{Notes: notes, Total: len(notes), HasMore: hasMore})
	}
	printNoteTable(notes)
	if hasMore {
		fmt.Fprintf(os.Stderr, "\nMore notes match. Raise --limit (currently %d) to see them.\n", limit)
	}
	return nil
}

func init() {
	tagListCmd.Flags().BoolVar(&tagListCounts, "counts", false, "also show how many notes and canvases carry each tag")
	tagShowCmd.Flags().StringVar(&tagShowMatch, "match", "all", "all: notes with every tag; any: notes with at least one")
	tagShowCmd.Flags().IntVar(&tagShowLimit, "limit", 50, "max notes to return")
	tagCmd.AddCommand(tagListCmd)
	tagCmd.AddCommand(tagShowCmd)
	rootCmd.AddCommand(tagCmd)
}

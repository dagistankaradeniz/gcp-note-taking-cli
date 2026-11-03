package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/canvasrender"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
	"github.com/spf13/cobra"
)

// Canvases over /v1/canvases (backend app/routers/v1_canvases.py), read-
// only. `canvas export` draws the canvas here with internal/canvasrender
// (the web export's framing and look), so it works the same for Zero-
// Knowledge canvases, decrypted on this device, and costs the server
// nothing. A locked or Vault canvas can't be exported from the CLI.

var canvasCmd = &cobra.Command{
	Use:   "canvas",
	Short: "List canvases and export them as images",
}

var (
	canvasListFolder     string
	canvasListTrashed    bool
	canvasListTags       []string
	canvasListLimit      int
	canvasListStartAfter string

	canvasExportOut        string
	canvasExportFormat     string
	canvasExportScale      float64
	canvasExportBackground string
	canvasExportDark       bool
	canvasExportForce      bool
)

// maxRefLookups bounds the note/task fetches one export makes for card
// titles; cards past it show the generic "Note"/"Task" label.
const maxRefLookups = 100

var canvasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List canvases",
	Example: `  quillink canvas list
  quillink canvas list --folder <folder-id>
  quillink canvas list --trashed --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		q := url.Values{}
		if canvasListFolder != "" {
			q.Set("folder_id", canvasListFolder)
		}
		if canvasListTrashed {
			q.Set("status", "trashed")
		}
		for _, t := range canvasListTags {
			q.Add("tags", t)
		}
		q.Set("limit", strconv.Itoa(canvasListLimit))
		if canvasListStartAfter != "" {
			q.Set("start_after", canvasListStartAfter)
		}
		c := newClient()
		dek, err := resolveZkDek(c)
		if err != nil {
			return err
		}
		var resp client.CanvasListResponse
		if err := c.Do("GET", "/v1/canvases", q, nil, &resp); err != nil {
			return err
		}
		for i := range resp.Canvases {
			decryptCanvasTitleZK(dek, &resp.Canvases[i])
		}
		if jsonOutput {
			return output.JSON(resp)
		}
		rows := make([][]string, 0, len(resp.Canvases))
		for _, cv := range resp.Canvases {
			rows = append(rows, []string{cv.ID, canvasTitle(cv), canvasState(cv), cv.UpdatedAt})
		}
		output.Table([]string{"ID", "TITLE", "STATE", "UPDATED"}, rows)
		if resp.HasMore && len(resp.Canvases) > 0 {
			fmt.Fprintf(os.Stderr, "\nMore canvases available. Continue with:\n  quillink canvas list --start-after %s\n", resp.Canvases[len(resp.Canvases)-1].ID)
		}
		return nil
	},
}

var canvasGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show a canvas's details",
	Example: `  quillink canvas get <id>
  quillink canvas get <id> --output json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		cv, elements, err := fetchCanvas(c, args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			// Readable elements are returned in their canonical form.
			if elements != nil {
				cv.Elements = make([]any, len(elements))
				for i, el := range elements {
					cv.Elements[i] = el
				}
				cv.EncryptedElements = nil
			}
			return output.JSON(cv)
		}
		counts := map[string]int{}
		for _, el := range elements {
			counts[el["type"].(string)]++
		}
		fmt.Println(canvasTitle(cv))
		fmt.Println()
		fmt.Printf("State:     %s\n", canvasState(cv))
		fmt.Printf("Updated:   %s\n", cv.UpdatedAt)
		if len(cv.Tags) > 0 {
			fmt.Printf("Tags:      %s\n", strings.Join(cv.Tags, ", "))
		}
		if elements != nil {
			fmt.Printf("Elements:  %d%s\n", len(elements), elementSummary(counts))
		}
		return nil
	},
}

var canvasExportCmd = &cobra.Command{
	Use:   "export <id>",
	Short: "Save a canvas as a PNG or JPEG image",
	Long: `Save a canvas as a PNG or JPEG image, framed around the drawing like the
app's own export. The format comes from --format or the --out file's
extension (.png, .jpg, .jpeg). Use --out - to write the image to stdout.`,
	Example: `  quillink canvas export <id> --out board.png
  quillink canvas export <id> --out board.jpg --scale 1
  quillink canvas export <id> --out board.png --dark
  quillink canvas export <id> --format png --out - > board.png`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if canvasExportOut == "" {
			return fmt.Errorf("--out is required (a .png or .jpg file, or - for stdout)")
		}
		format := canvasExportFormat
		if format == "" {
			switch strings.ToLower(filepath.Ext(canvasExportOut)) {
			case ".png":
				format = "png"
			case ".jpg", ".jpeg":
				format = "jpeg"
			default:
				return fmt.Errorf("can't tell the image format from %q -- add --format png or --format jpeg", canvasExportOut)
			}
		}
		if format != "png" && format != "jpeg" && format != "jpg" {
			return fmt.Errorf("--format must be png or jpeg")
		}
		if canvasExportScale < 0.1 || canvasExportScale > 8 {
			return fmt.Errorf("--scale must be between 0.1 and 8")
		}
		palette := canvasrender.LightPalette
		if canvasExportDark {
			palette = canvasrender.DarkPalette
		}
		if canvasExportBackground != "" {
			bg, err := canvasrender.ParseColor(canvasExportBackground)
			if err != nil {
				return fmt.Errorf("--background: %w", err)
			}
			palette.Background = bg
		}
		if canvasExportOut != "-" {
			if _, err := os.Stat(canvasExportOut); err == nil && !canvasExportForce {
				return fmt.Errorf("%s already exists -- add --force to overwrite it", canvasExportOut)
			}
		}

		c := newClient()
		cv, elements, err := fetchCanvas(c, args[0])
		if err != nil {
			return err
		}
		if elements == nil {
			return fmt.Errorf("%q can't be exported: %s", canvasTitle(cv), unreadableReason(cv))
		}
		rendering := progress.Start("Drawing the canvas", 0)
		img, err := canvasrender.Render(elements, canvasrender.Options{
			Scale:   canvasExportScale,
			Palette: palette,
			RefText: refResolver(c, elements),
		})
		rendering.Done()
		if err != nil {
			return err
		}
		return writeImage(canvasExportOut, func(w io.Writer) error {
			return canvasrender.Encode(w, img, format)
		})
	},
}

// fetchCanvas gets a canvas and its readable, normalised elements -- nil
// when the CLI can't read them (a locked/Vault canvas, or ZK ciphertext
// that doesn't open).
func fetchCanvas(c *client.Client, id string) (client.Canvas, []canvasrender.Element, error) {
	dek, err := resolveZkDek(c)
	if err != nil {
		return client.Canvas{}, nil, err
	}
	var cv client.Canvas
	if err := c.Do("GET", "/v1/canvases/"+url.PathEscape(id), nil, nil, &cv); err != nil {
		return cv, nil, err
	}
	raw, ok := canvasElementsZK(dek, &cv)
	if !ok {
		return cv, nil, nil
	}
	return cv, canvasrender.NormalizeElements(raw), nil
}

// canvasElementsZK decrypts a ZK canvas's title and elements in place and
// returns the raw elements. A ZK account's canvas with plain elements and
// no envelope can only have come from the server, so it's refused (as the
// web's decryptCanvasRead does).
func canvasElementsZK(dek []byte, cv *client.Canvas) ([]any, bool) {
	if cv.NoteEncrypted || cv.VaultEncrypted {
		return nil, false
	}
	if dek == nil {
		return cv.Elements, true
	}
	unreadable := func() ([]any, bool) {
		cv.Title, cv.Elements, cv.ZkUnreadable = zkUnreadableText, nil, true
		return nil, false
	}
	if cv.EncryptedElements == nil || *cv.EncryptedElements == "" {
		if len(cv.Elements) > 0 {
			return unreadable()
		}
	}
	open := zkcrypto.NewOpener(dek, "canvas")
	if cv.Title != "" {
		title, err := open.String("title", cv.Title)
		if err != nil {
			return unreadable()
		}
		cv.Title = title
	}
	if cv.EncryptedElements == nil || *cv.EncryptedElements == "" {
		return []any{}, true
	}
	plain, err := open.String("elements", *cv.EncryptedElements)
	if err != nil {
		return unreadable()
	}
	var raw []any
	if err := json.Unmarshal([]byte(plain), &raw); err != nil {
		return unreadable()
	}
	cv.Elements = raw
	return raw, true
}

// decryptCanvasTitleZK opens just the title, for lists (which carry no
// elements).
func decryptCanvasTitleZK(dek []byte, cv *client.Canvas) {
	if dek == nil || cv.NoteEncrypted || cv.VaultEncrypted || cv.Title == "" {
		return
	}
	title, err := zkcrypto.NewOpener(dek, "canvas").String("title", cv.Title)
	if err != nil {
		cv.Title, cv.ZkUnreadable = zkUnreadableText, true
		return
	}
	cv.Title = title
}

func canvasTitle(cv client.Canvas) string {
	if cv.Title == "" {
		return "Untitled canvas"
	}
	return cv.Title
}

func canvasState(cv client.Canvas) string {
	var parts []string
	if cv.Status == "trashed" {
		parts = append(parts, "in trash")
	}
	if cv.Pinned {
		parts = append(parts, "pinned")
	}
	if cv.NoteEncrypted || cv.Locked {
		parts = append(parts, "locked")
	}
	if cv.VaultEncrypted {
		parts = append(parts, "vault")
	}
	if cv.SharedCount > 0 || cv.CollabEnabled {
		parts = append(parts, "shared")
	}
	return strings.Join(parts, ", ")
}

func unreadableReason(cv client.Canvas) string {
	switch {
	case cv.VaultEncrypted:
		return "it's in the Vault -- use quillink vault export"
	case cv.NoteEncrypted:
		return "it's locked -- unlock it in the app first"
	}
	return "its content couldn't be decrypted on this device"
}

func elementSummary(counts map[string]int) string {
	labels := []struct{ key, name string }{
		{"shape", "shapes"}, {"arrow", "arrows"}, {"ink", "drawings"}, {"text", "texts"},
		{"image", "images"}, {"note_ref", "notes"}, {"task_ref", "tasks"},
	}
	var parts []string
	for _, l := range labels {
		if n := counts[l.key]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, l.name))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// refResolver fetches the titles of the notes and tasks the canvas's cards
// point at, with the signed-in account's own access (the web's
// fetchRefSource). A missing item reads "Deleted"; one that can't be
// fetched (offline, no scope) keeps the generic label.
func refResolver(c *client.Client, elements []canvasrender.Element) canvasrender.RefText {
	type ref struct{ title, status string }
	resolved := map[string]ref{}
	dek, _ := resolveZkDek(c)
	lookups := 0
	for _, el := range elements {
		data := el["data"].(map[string]any)
		kind, key := "note", "note_id"
		if el["type"] == "task_ref" {
			kind, key = "task", "task_id"
		} else if el["type"] != "note_ref" {
			continue
		}
		id := data[key].(string)
		if _, done := resolved[kind+":"+id]; done || lookups >= maxRefLookups {
			continue
		}
		lookups++
		var r ref
		var err error
		if kind == "note" {
			var n client.Note
			if err = c.Do("GET", "/v1/notes/"+url.PathEscape(id), nil, nil, &n); err == nil {
				decryptNoteZK(dek, &n)
				r.title = orDefault(n.Title, "Untitled note")
				if n.Status == "trashed" {
					r.status = "In trash"
				}
			}
		} else {
			var t client.Task
			if err = c.Do("GET", "/v1/tasks/"+url.PathEscape(id), nil, nil, &t); err == nil {
				decryptTaskZK(dek, &t)
				r.title = orDefault(t.Title, "Untitled task")
				if t.Status == "trashed" {
					r.status = "In trash"
				}
			}
		}
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && (apiErr.Status == 404 || apiErr.Status == 410) {
			r = ref{status: "Deleted"}
		}
		resolved[kind+":"+id] = r
	}
	return func(kind, id string) (string, string) {
		r := resolved[kind+":"+id]
		if r.title == "" {
			r.title = "Note"
			if kind == "task" {
				r.title = "Task"
			}
		}
		return r.title, r.status
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// writeImage writes to path through a private temporary file moved into
// place on success, or to stdout for "-".
func writeImage(path string, write func(io.Writer) error) error {
	if path == "-" {
		return write(os.Stdout)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".quillink-canvas-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := write(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("save %s: %w", target, err)
	}
	fmt.Fprintln(os.Stderr, "Saved to", target)
	return nil
}

func init() {
	canvasListCmd.Flags().StringVar(&canvasListFolder, "folder", "", "only canvases in this folder")
	canvasListCmd.Flags().BoolVar(&canvasListTrashed, "trashed", false, "list canvases in the trash")
	canvasListCmd.Flags().StringSliceVar(&canvasListTags, "tag", nil, "only canvases with this tag (repeatable)")
	canvasListCmd.Flags().IntVar(&canvasListLimit, "limit", 50, "how many to list")
	canvasListCmd.Flags().StringVar(&canvasListStartAfter, "start-after", "", "continue after this canvas id")

	canvasExportCmd.Flags().StringVar(&canvasExportOut, "out", "", "image file to write (.png, .jpg), or - for stdout")
	canvasExportCmd.Flags().StringVar(&canvasExportFormat, "format", "", "png or jpeg (default: from the --out extension)")
	canvasExportCmd.Flags().Float64Var(&canvasExportScale, "scale", 2, "pixels per canvas unit (lowered to keep the image within 8192 px)")
	canvasExportCmd.Flags().StringVar(&canvasExportBackground, "background", "", "background colour as #RRGGBB (default: the app's light theme)")
	canvasExportCmd.Flags().BoolVar(&canvasExportDark, "dark", false, "use the app's dark theme colours")
	canvasExportCmd.Flags().BoolVar(&canvasExportForce, "force", false, "overwrite an existing file")

	canvasCmd.AddCommand(canvasListCmd, canvasGetCmd, canvasExportCmd)
	rootCmd.AddCommand(canvasCmd)
}

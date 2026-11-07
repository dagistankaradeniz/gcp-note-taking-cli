// Package output renders command results either as human-readable
// text/tables (default, interactive terminal) or as stable, independently
// versioned JSON (--json, see CLI Access Confluence page Decisions Log).
// It intentionally does not pass through the raw API response shape --
// each command builds its own JSON-able struct so an API v2 change can't
// silently break a script's `jq` pipeline; only a deliberate SchemaVersion
// bump does that.
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/style"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
)

// SchemaVersion is the --json output schema's own version, independent
// from both the API version and the CLI's version (see Decisions Log).
const SchemaVersion = "1"

// Envelope wraps every --json response with the schema version so a
// script can assert compatibility before parsing further.
type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Data          any    `json:"data"`
}

// JSON emits v in the current structured format (Current): the --json
// envelope by default, or CSV / YAML / Markdown rendered from the same
// data. Commands call it whenever a structured format is selected.
func JSON(v any) error {
	switch current {
	case FormatCSV, FormatMarkdown, FormatYAML:
		data, err := toOrdered(v)
		if err != nil {
			return err
		}
		switch current {
		case FormatCSV:
			return writeCSV(os.Stdout, data)
		case FormatMarkdown:
			return writeMarkdown(os.Stdout, data)
		}
		return writeYAML(os.Stdout, object{{key: "schema_version", val: SchemaVersion}, {key: "data", val: data}})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(Envelope{SchemaVersion: SchemaVersion, Data: v})
}

// Listed, when set, is told the ids and titles of a table with an ID
// column on a terminal, and the table gets a "#" column so its rows can
// be picked by number. The interactive session sets it.
var Listed func(ids, titles []string)

// numbered adds the "#" column and reports the table's rows to Listed.
func numbered(headers []string, rows [][]string) ([]string, [][]string) {
	id, title := -1, -1
	for i, h := range headers {
		switch h {
		case "ID":
			id = i
		case "TITLE", "NAME", "FILENAME":
			title = i
		}
	}
	if Listed == nil || id < 0 {
		return headers, rows
	}
	ids, titles := make([]string, len(rows)), make([]string, len(rows))
	out := make([][]string, len(rows))
	for r, row := range rows {
		ids[r] = row[id]
		if title >= 0 {
			titles[r] = row[title]
		}
		out[r] = append([]string{strconv.Itoa(r + 1)}, row...)
	}
	Listed(ids, titles)
	return append([]string{"#"}, headers...), out
}

// Table writes tab-aligned rows to stdout -- the human-readable default.
// On a terminal it's laid out to fit the window instead (ui.Table);
// piped or redirected it stays plain tab-aligned text.
func Table(headers []string, rows [][]string) {
	if style.IsTerminal(os.Stdout) {
		headers, rows = numbered(headers, rows)
	}
	if ui.Rich(os.Stdout) {
		fmt.Fprint(os.Stdout, ui.Table(headers, rows, ui.Width()))
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	defer w.Flush()

	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)

	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}
}

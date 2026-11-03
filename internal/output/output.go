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
	"text/tabwriter"

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

// Table writes tab-aligned rows to stdout -- the human-readable default.
// On a terminal it's laid out to fit the window instead (ui.Table);
// piped or redirected it stays plain tab-aligned text.
func Table(headers []string, rows [][]string) {
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

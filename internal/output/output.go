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

func JSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(Envelope{SchemaVersion: SchemaVersion, Data: v})
}

// Table writes tab-aligned rows to stdout -- the human-readable default.
func Table(headers []string, rows [][]string) {
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

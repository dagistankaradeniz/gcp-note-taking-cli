package output

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Format is the --output format. FormatTable is the human-readable
// default; every other format is rendered from the same data a command
// hands to JSON (see Emit), so all of them stay as stable as --json.
type Format string

const (
	FormatTable    Format = "table"
	FormatJSON     Format = "json"
	FormatCSV      Format = "csv"
	FormatYAML     Format = "yaml"
	FormatMarkdown Format = "md"
)

// Formats lists the accepted --output values, for help text and errors.
var Formats = []Format{FormatTable, FormatJSON, FormatCSV, FormatYAML, FormatMarkdown}

// ParseFormat accepts the --output values plus a few obvious aliases.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table", "text":
		return FormatTable, nil
	case "json":
		return FormatJSON, nil
	case "csv":
		return FormatCSV, nil
	case "yaml", "yml":
		return FormatYAML, nil
	case "md", "markdown":
		return FormatMarkdown, nil
	}
	names := make([]string, len(Formats))
	for i, f := range Formats {
		names[i] = string(f)
	}
	return "", fmt.Errorf("unknown --output %q (use one of: %s)", s, strings.Join(names, ", "))
}

// FormatForPath infers the format from a file name's extension, for
// --out without --output. Unknown extensions (and .txt) mean table.
func FormatForPath(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return FormatJSON
	case ".csv":
		return FormatCSV
	case ".yaml", ".yml":
		return FormatYAML
	case ".md", ".markdown":
		return FormatMarkdown
	}
	return FormatTable
}

// current is the format for this command run (set by the root command
// before RunE, reset for every command in the interactive session).
var current = FormatTable

// SetFormat sets the format for the command about to run.
func SetFormat(f Format) { current = f }

// Current returns the format for the running command.
func Current() Format { return current }

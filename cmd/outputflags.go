package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

// --output picks the format (table, json, csv, yaml, md) and --out saves
// the result to a file instead of printing it. Both apply to every
// command; `note export` and `note attachment download` keep their own
// --out, which writes their raw export/file bytes.

var (
	outputFlag string
	outFile    string
	forceFlag  bool
)

// outState is the file being written for the current command, if any.
var outState struct {
	tmp    *os.File
	target string
	stdout *os.File
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputFlag, "output", "", "output format: table, json, csv, yaml or md (default: table, or from the --out file's extension)")
	rootCmd.PersistentFlags().StringVar(&outFile, "out", "", "save the result to this file instead of printing it")
	rootCmd.PersistentFlags().BoolVar(&forceFlag, "force", false, "with --out, overwrite an existing file; on a delete, skip the confirmation (required in scripts and CI)")
	rootCmd.PersistentPreRunE = prepareOutput
	output.BodyText = bodyToText
}

// prepareOutput resolves the format and, for --out, points stdout at a
// private temporary file next to the target; finishOutput moves it into
// place only when the command succeeds.
func prepareOutput(_ *cobra.Command, _ []string) error {
	format := output.FormatTable
	switch {
	case outputFlag != "":
		f, err := output.ParseFormat(outputFlag)
		if err != nil {
			return err
		}
		if jsonOutput && f != output.FormatJSON {
			return fmt.Errorf("--json and --output %s conflict -- use one", f)
		}
		format = f
	case jsonOutput:
		format = output.FormatJSON
	case outFile != "":
		format = output.FormatForPath(outFile)
	}
	output.SetFormat(format)
	jsonOutput = format != output.FormatTable
	startProgress()

	if outFile == "" {
		return nil
	}
	target, err := filepath.Abs(outFile)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil && !forceFlag {
		return fmt.Errorf("%s already exists -- add --force to overwrite it", outFile)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// CreateTemp makes the file readable by the owner only (0600).
	tmp, err := os.CreateTemp(filepath.Dir(target), ".quillink-out-*")
	if err != nil {
		return fmt.Errorf("create %s: %w", outFile, err)
	}
	outState.tmp, outState.target, outState.stdout = tmp, target, os.Stdout
	os.Stdout = tmp
	return nil
}

// finishOutput restores stdout and, after a successful command, moves the
// written file into place and says where it went. A failed command leaves
// no file behind. It also resets the per-command format, which matters in
// the interactive session.
func finishOutput(runErr error) error {
	stopProgress()
	defer func() {
		output.SetFormat(output.FormatTable)
		jsonOutput = false
	}()
	tmp := outState.tmp
	if tmp == nil {
		return runErr
	}
	os.Stdout = outState.stdout
	target := outState.target
	outState.tmp = nil
	closeErr := tmp.Close()
	if runErr != nil || closeErr != nil {
		_ = os.Remove(tmp.Name())
		if runErr != nil {
			return runErr
		}
		return closeErr
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("save %s: %w", target, err)
	}
	ui.Success(os.Stderr, "Saved to "+target)
	return nil
}

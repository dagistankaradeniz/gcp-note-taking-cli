package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/output"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/ui"
	"github.com/spf13/cobra"
)

func answering(t *testing.T, text string) {
	t.Helper()
	confirmReader = strings.NewReader(text)
	t.Cleanup(func() { confirmReader = nil })
}

func TestConfirmDeleteAnswers(t *testing.T) {
	fields := []ui.Field{{Label: "ID", Value: "abc"}}
	cases := map[string]bool{
		"y\n": true, "YES\n": true, " yes \n": true,
		"n\n": false, "\n": false, "maybe\n": false, "": false,
	}
	for in, want := range cases {
		answering(t, in)
		got, err := confirmDelete("note", fields)
		if err != nil || got != want {
			t.Errorf("%q: got %v, %v", in, got, err)
		}
	}
}

func TestForceSkipsTheQuestion(t *testing.T) {
	forceFlag = true
	t.Cleanup(func() { forceFlag = false })
	if ok, err := confirmDelete("note", nil); !ok || err != nil {
		t.Fatalf("got %v, %v", ok, err)
	}
}

// Tests run without a terminal, like a script or CI.
func TestNoTerminalNeedsForce(t *testing.T) {
	if isTerminalStdin() {
		t.Skip("stdin is a terminal")
	}
	ok, err := confirmDelete("note", nil)
	if ok || !errors.Is(err, errNeedsForce) {
		t.Fatalf("got %v, %v", ok, err)
	}
	if got := exitCode(err); got != output.ExitInvalidInput {
		t.Fatalf("exit %d", got)
	}
}

// A destructive command that doesn't confirm fails here, by its name or
// by the DELETE it sends.
func TestEveryDeleteConfirms(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		switch c.Name() {
		case "delete", "remove", "rm", "purge", "trash", "destroy", "wipe":
			if c.Annotations["confirms-delete"] != "true" {
				t.Errorf("%s doesn't confirm", c.CommandPath())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if s := string(src); strings.Contains(s, `"DELETE"`) && !strings.Contains(s, "confirmDelete(") {
			t.Errorf("%s sends a DELETE without confirmDelete", f)
		}
	}
}

package notemd

import (
	"encoding/json"
	"os"
	"testing"
)

// The fixture's markdown was produced by the backend's tiptap_to_markdown
// from the same body, so the two renderers must agree byte for byte.
func TestMatchesBackendRenderer(t *testing.T) {
	raw, err := os.ReadFile("testdata/note_export_fixture.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Body     map[string]any `json:"body"`
		Markdown string         `json:"markdown"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if got := ToMarkdown(f.Body); got != f.Markdown {
		t.Fatalf("got\n%q\nwant\n%q", got, f.Markdown)
	}
	if ToMarkdown(nil) != "" {
		t.Fatal("nil body")
	}
}

package cmd

import "testing"

// bodyToText is a flat preview approximation, not an exact inverse of
// textToBody -- multi-paragraph input comes back space-joined, not with
// the original blank lines. Single-paragraph text does round-trip exactly.
func TestTextToBodyRoundTrip(t *testing.T) {
	cases := map[string]string{
		"Hello, world.":                    "Hello, world.",
		"Paragraph one.\n\nParagraph two.": "Paragraph one. Paragraph two.",
		"":                                 "",
	}
	for text, want := range cases {
		body := textToBody(text)
		if body["type"] != "doc" {
			t.Fatalf("textToBody(%q): missing doc type", text)
		}
		if got := bodyToText(body); got != want {
			t.Errorf("textToBody(%q) -> bodyToText = %q, want %q", text, got, want)
		}
	}
}

func TestBodyFromJSONInvalid(t *testing.T) {
	if _, err := bodyFromJSON("not json"); err == nil {
		t.Error("expected an error for invalid JSON, got nil")
	}
}

func TestBodyFromJSONValid(t *testing.T) {
	body, err := bodyFromJSON(`{"type":"doc","content":[]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["type"] != "doc" {
		t.Errorf("body[type] = %v, want doc", body["type"])
	}
}

func TestBodyToTextNil(t *testing.T) {
	if got := bodyToText(nil); got != "" {
		t.Errorf("bodyToText(nil) = %q, want empty string", got)
	}
}

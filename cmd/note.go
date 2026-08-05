package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage notes",
}

func init() {
	rootCmd.AddCommand(noteCmd)
}

// textToBody wraps plain text in a minimal Tiptap document -- the shape
// app/models/note.py's NoteCreate/NoteUpdate.body expects (see
// gcp-note-taking-frontend's Tiptap editor for the full schema; the CLI
// only needs to produce something the editor can open, not replicate every
// mark/node type).
func textToBody(text string) map[string]any {
	paragraphs := strings.Split(text, "\n\n")
	content := make([]any, 0, len(paragraphs))
	for _, p := range paragraphs {
		para := map[string]any{"type": "paragraph"}
		if p != "" {
			para["content"] = []any{map[string]any{"type": "text", "text": p}}
		}
		content = append(content, para)
	}
	return map[string]any{"type": "doc", "content": content}
}

// bodyFromJSON parses a raw Tiptap JSON document, e.g. exported from
// another note or hand-written for advanced formatting.
func bodyFromJSON(raw string) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("invalid --body-json: %w", err)
	}
	return body, nil
}

// bodyToText extracts a flat, readable text approximation from a Tiptap
// document -- enough for a terminal table/preview, not a full renderer.
func bodyToText(body map[string]any) string {
	if body == nil {
		return ""
	}
	var sb strings.Builder
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if text, ok := node["text"].(string); ok {
			sb.WriteString(text)
		}
		if content, ok := node["content"].([]any); ok {
			for _, child := range content {
				if childMap, ok := child.(map[string]any); ok {
					walk(childMap)
				}
			}
			if node["type"] == "paragraph" {
				sb.WriteString(" ")
			}
		}
	}
	walk(body)
	return strings.TrimSpace(sb.String())
}

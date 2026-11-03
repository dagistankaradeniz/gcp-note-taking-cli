package cmd

import (
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

func TestCanvasElementsZKFailsClosed(t *testing.T) {
	dek := make([]byte, 32)
	plain := []any{map[string]any{"id": "a", "type": "text"}}

	// A ZK account's canvas with plain elements and no envelope can only
	// have come from the server.
	cv := client.Canvas{Title: "x", Elements: plain}
	if raw, ok := canvasElementsZK(dek, &cv); ok || raw != nil || !cv.ZkUnreadable || cv.Title != zkUnreadableText {
		t.Fatalf("plain elements on ZK accepted: %v %v %+v", raw, ok, cv)
	}

	garbage := `{"v":2,"b":"x","iv":"AA","ct":"AA"}`
	cv = client.Canvas{EncryptedElements: &garbage}
	if _, ok := canvasElementsZK(dek, &cv); ok || !cv.ZkUnreadable {
		t.Fatal("undecryptable envelope accepted")
	}

	cv = client.Canvas{}
	if raw, ok := canvasElementsZK(dek, &cv); !ok || len(raw) != 0 {
		t.Fatal("empty ZK canvas refused")
	}
}

func TestCanvasElementsLockedAndPlain(t *testing.T) {
	plain := []any{map[string]any{"id": "a"}}
	locked := client.Canvas{NoteEncrypted: true, Elements: plain}
	if _, ok := canvasElementsZK(nil, &locked); ok {
		t.Fatal("locked canvas readable")
	}
	vault := client.Canvas{VaultEncrypted: true}
	if _, ok := canvasElementsZK(nil, &vault); ok {
		t.Fatal("vault canvas readable")
	}
	cv := client.Canvas{Elements: plain}
	if raw, ok := canvasElementsZK(nil, &cv); !ok || len(raw) != 1 {
		t.Fatal("plain canvas refused")
	}
}

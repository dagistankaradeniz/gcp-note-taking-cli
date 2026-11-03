package cmd

import (
	"encoding/json"
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/notecrypto"
)

func vaultKeyForTest() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestDecryptVaultNote(t *testing.T) {
	key := vaultKeyForTest()
	title, _ := notecrypto.EncryptString(key, "Secret plan")
	bodyEnc, _ := notecrypto.EncryptNoteBody(key, map[string]any{"type": "doc"})
	var body map[string]any
	_ = json.Unmarshal([]byte(bodyEnc), &body)

	n := client.VaultNote{Title: title, Body: body}
	decryptVaultNote(key, &n)
	if n.Unreadable || n.Title != "Secret plan" || n.Body["type"] != "doc" {
		t.Fatalf("got %+v", n)
	}

	// Server text is never shown: a plaintext title fails closed.
	n = client.VaultNote{Title: "planted", Body: body}
	decryptVaultNote(key, &n)
	if !n.Unreadable || n.Title != zkUnreadableText || n.Body != nil {
		t.Fatalf("plaintext accepted: %+v", n)
	}

	other := make([]byte, 32)
	n = client.VaultNote{Title: title}
	decryptVaultNote(other, &n)
	if !n.Unreadable {
		t.Fatal("wrong key accepted")
	}
}

func TestDecryptVaultCanvas(t *testing.T) {
	key := vaultKeyForTest()
	title, _ := notecrypto.EncryptString(key, "Board")
	elements, _ := notecrypto.EncryptString(key, `[{"id":"a","type":"text"}]`)
	cv := client.VaultCanvas{Title: title, Elements: &elements}
	raw := decryptVaultCanvas(key, &cv)
	if cv.Unreadable || cv.Title != "Board" || len(raw) != 1 || cv.Elements != nil {
		t.Fatalf("got %+v %v", cv, raw)
	}
	bad := `{"ciphertext":"AA","iv":"AA","tag":"AA"}`
	cv = client.VaultCanvas{Title: title, Elements: &bad}
	if raw := decryptVaultCanvas(key, &cv); raw != nil || !cv.Unreadable {
		t.Fatal("bad elements accepted")
	}
}

func TestSessionKeyWipe(t *testing.T) {
	shellActive = true
	defer func() { shellActive = false }()
	key := []byte{1, 2, 3}
	vaultSession.put("me", key)
	if vaultSession.get("other") != nil {
		t.Fatal("another account's key returned")
	}
	if vaultSession.get("me") == nil {
		t.Fatal("key not cached")
	}
	if !zkSessionWipe() || key[0] != 0 {
		t.Fatal("wipe didn't zero the key")
	}
	if vaultSession.get("me") != nil {
		t.Fatal("key survived wipe")
	}
}

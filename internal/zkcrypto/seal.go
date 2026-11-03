package zkcrypto

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// Sealing -- accountCrypto.ts's sealString/sealJson/newBindingId, for the
// CLI's Zero-Knowledge writes. Every field of one item is sealed under the
// same binding id; a new item gets a fresh one, an edit must reuse the
// item's own (read with Opener.Bind) or the other fields stop opening.

const bindBytes = 16

// NewBindingID is 16 random bytes, base64url without padding (22 chars).
func NewBindingID() (string, error) {
	b := make([]byte, bindBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate binding id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pad appends the 0x80 marker and zeros up to the Padmé bucket.
func pad(content []byte) []byte {
	out := make([]byte, PaddedLength(len(content)))
	copy(out, content)
	out[len(content)] = 0x80
	return out
}

// SealString encrypts value as a JSON-stringified v2 envelope.
func SealString(dek []byte, itemType, field, bind, value string) (string, error) {
	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate iv: %w", err)
	}
	return sealWithIV(dek, itemType, field, bind, value, iv)
}

func sealWithIV(dek []byte, itemType, field, bind, value string, iv []byte) (string, error) {
	if !bindPattern.MatchString(bind) {
		return "", fmt.Errorf("invalid binding id")
	}
	gcm, err := newGCM(dek)
	if err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, iv, pad([]byte(value)), aad(itemType, field, bind))
	tagAt := len(sealed) - gcm.Overhead()
	out, err := json.Marshal(EncryptedPayload{
		V:          2,
		B:          bind,
		Ciphertext: base64.StdEncoding.EncodeToString(sealed[:tagAt]),
		IV:         base64.StdEncoding.EncodeToString(iv),
		Tag:        base64.StdEncoding.EncodeToString(sealed[tagAt:]),
	})
	return string(out), err
}

// SealJSON encrypts value's JSON (sealJson).
func SealJSON(dek []byte, itemType, field, bind string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return SealString(dek, itemType, field, bind, string(raw))
}

// SealBody seals a note body and returns the envelope as an object -- the
// shape the API takes for `body` (encryptNoteWrite's JSON.parse(sealJson)).
func SealBody(dek []byte, bind string, body map[string]any) (map[string]any, error) {
	sealed, err := SealJSON(dek, "note", "body", bind, body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal([]byte(sealed), &out)
	return out, err
}

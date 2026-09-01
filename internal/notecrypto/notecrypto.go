// Package notecrypto is a Go port of gcp-note-taking-frontend's
// src/lib/vaultCrypto.ts -- byte-for-byte compatible client-side
// encryption for password-protected notes, so a note locked from the web
// app can be unlocked from the CLI and vice versa. The backend never
// sees a password or plaintext body, only an opaque ciphertext blob and
// a separately-derived verifier hash it can check without learning the
// actual encryption key (see NoteLockRequest's docstring in
// gcp-note-taking-backend/app/models/note.py).
//
// Keep this in lockstep with vaultCrypto.ts if that file's algorithm,
// parameters, or wire format ever change.
package notecrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const (
	// DefaultIterations matches vaultCrypto.ts's PBKDF2_ITERATIONS --
	// used only when creating a new lock. An existing note's actual
	// iteration count (which may differ, e.g. a future lock created with
	// a higher count) always comes from the server's lock_iterations.
	DefaultIterations = 600_000
	saltLength        = 32
	ivLength          = 12
)

// verifierLabel is appended to the salt before deriving the verifier
// hash, keeping it cryptographically independent from the AES key -- see
// vaultCrypto.ts's identical comment on deriveVerifierHash.
var verifierLabel = []byte("vault-password-verifier-v1")

// EncryptedPayload is the exact JSON shape vaultCrypto.ts's encrypt()
// produces and decrypt() consumes -- field names and base64 encoding
// (standard, padded) must match exactly.
type EncryptedPayload struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	Tag        string `json:"tag"`
}

// GenerateSalt returns a fresh, base64-encoded random salt for a new
// lock.
func GenerateSalt() (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

// DeriveKey derives the 256-bit AES-GCM key from a password + salt,
// matching vaultCrypto.ts's deriveKey exactly (PBKDF2-HMAC-SHA256).
func DeriveKey(password, saltB64 string, iterations int) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	return pbkdf2.Key(sha256.New, password, salt, iterations, 32)
}

// DeriveVerifierHash derives the server-checkable verifier, matching
// vaultCrypto.ts's deriveVerifierHash exactly: a second, independent
// PBKDF2 derivation over salt+verifierLabel.
func DeriveVerifierHash(password, saltB64 string, iterations int) (string, error) {
	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return "", fmt.Errorf("decode salt: %w", err)
	}
	verifierSalt := append(append([]byte{}, salt...), verifierLabel...)
	bits, err := pbkdf2.Key(sha256.New, password, verifierSalt, iterations, 32)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bits), nil
}

// encrypt AES-GCM-encrypts plaintext with key and a fresh random IV,
// splitting the sealed output into ciphertext and tag exactly like
// vaultCrypto.ts's encrypt (Web Crypto's AES-GCM output is
// ciphertext||tag, same as Go's cipher.Seal).
func encrypt(key []byte, plaintext string) (EncryptedPayload, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return EncryptedPayload{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedPayload{}, err
	}
	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return EncryptedPayload{}, fmt.Errorf("generate iv: %w", err)
	}
	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)
	tagLen := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagLen]
	tag := sealed[len(sealed)-tagLen:]
	return EncryptedPayload{
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		IV:         base64.StdEncoding.EncodeToString(iv),
		Tag:        base64.StdEncoding.EncodeToString(tag),
	}, nil
}

// decrypt is encrypt's inverse.
func decrypt(key []byte, payload EncryptedPayload) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	tag, err := base64.StdEncoding.DecodeString(payload.Tag)
	if err != nil {
		return "", fmt.Errorf("decode tag: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(payload.IV)
	if err != nil {
		return "", fmt.Errorf("decode iv: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed := append(append([]byte{}, ciphertext...), tag...)
	plaintext, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// EncryptNoteBody JSON-marshals body, encrypts it, and returns the
// doubly-JSON-encoded string the backend expects as encrypted_body --
// matches vaultCrypto.ts's encryptNoteBody exactly.
func EncryptNoteBody(key []byte, body map[string]any) (string, error) {
	if body == nil {
		return "", nil
	}
	plaintext, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal body: %w", err)
	}
	payload, err := encrypt(key, string(plaintext))
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// DecryptNoteBody is EncryptNoteBody's inverse -- matches
// vaultCrypto.ts's decryptNoteBody.
func DecryptNoteBody(key []byte, encryptedBody string) (map[string]any, error) {
	if encryptedBody == "" {
		return nil, nil
	}
	var payload EncryptedPayload
	if err := json.Unmarshal([]byte(encryptedBody), &payload); err != nil {
		return nil, fmt.Errorf("parse encrypted payload: %w", err)
	}
	plaintext, err := decrypt(key, payload)
	if err != nil {
		return nil, err
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(plaintext), &body); err != nil {
		return nil, fmt.Errorf("parse decrypted body: %w", err)
	}
	return body, nil
}

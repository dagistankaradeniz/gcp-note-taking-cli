// Package zkcrypto is a Go port of gcp-note-taking-frontend's
// src/lib/recoveryCredential.ts + src/lib/accountCrypto.ts -- the
// Zero-Knowledge account's KEK/DEK derivation and per-field decrypt.
// Decrypt-only, deliberately: the CLI can read a ZK account's notes but
// not yet write them (see gcp-note-taking-backend's
// require_read_only_scopes_for_zk) -- a write-path bug here has real
// corruption risk a read bug doesn't, so encryption stays browser-only
// until this decrypt port has a track record.
//
// The account KEK requires BOTH halves of the two-part Recovery Credential
// (a passphrase, run through PBKDF2 or Argon2id depending on the account's
// stored kdf_algorithm, combined via HKDF-SHA256 with the raw Secret Key)
// -- same as vaultCrypto.ts's single-passphrase Vault KEK, just with a
// second input. The server never sees the passphrase, the Secret Key, the
// intermediate key, or the KEK: only the already-wrapped DEK and a
// verifier hash independently derived from the same two inputs (see
// GET /v1/zk/status + POST /v1/zk/unlock in gcp-note-taking-backend's
// app/routers/v1_zk.py).
//
// Keep this in lockstep with recoveryCredential.ts/accountCrypto.ts if
// their algorithm, parameters, or wire format ever change.
package zkcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// KdfParams mirrors recoveryCredential.ts's KdfParams -- returned by
// GET /v1/zk/status (see ZkStatusResponse in gcp-note-taking-backend's
// app/models/zk.py).
type KdfParams struct {
	Algorithm  string // "pbkdf2" | "argon2id"
	Salt       string // base64
	Iterations int    // pbkdf2
	MemoryKiB  int    // argon2id
	Ops        int    // argon2id
}

// aesKeyLength matches AES_KEY_LENGTH in vaultCrypto.ts/recoveryCredential.ts.
const aesKeyLength = 32

// argon2Threads is fixed at 1 to match libsodium's crypto_pwhash (the
// browser's Argon2id implementation, via libsodium.js) -- crypto_pwhash
// always uses a single lane regardless of caller-supplied cost params, so
// this must stay a constant, never derived from KdfParams.
const argon2Threads = 1

var (
	kekInfo      = []byte("quillink-zk-kek-v1")
	verifierInfo = []byte("quillink-zk-account-verifier-v1")
)

// EncryptedPayload is the exact JSON shape encryptString/encryptJson
// produce and decryptString/decryptJson consume in accountCrypto.ts --
// identical wire format to notecrypto.go's EncryptedPayload (same
// {ciphertext,iv,tag}, base64, AES-GCM-with-appended-tag shape), kept as
// its own type here so this package has no dependency on notecrypto.
type EncryptedPayload struct {
	Ciphertext string `json:"ciphertext"`
	IV         string `json:"iv"`
	Tag        string `json:"tag"`
}

func deriveIntermediateBits(passphrase string, params KdfParams) ([]byte, error) {
	salt, err := base64.StdEncoding.DecodeString(params.Salt)
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	switch params.Algorithm {
	case "pbkdf2":
		return pbkdf2.Key(sha256.New, passphrase, salt, params.Iterations, aesKeyLength)
	case "argon2id":
		return argon2.IDKey(
			[]byte(passphrase), salt,
			uint32(params.Ops), uint32(params.MemoryKiB), argon2Threads,
			aesKeyLength,
		), nil
	default:
		return nil, fmt.Errorf("unknown kdf algorithm %q", params.Algorithm)
	}
}

// hkdfCombined matches hkdfFromCombined in recoveryCredential.ts: HKDF-
// SHA256 over intermediateBits||secretKeyRaw as IKM, empty salt, the given
// info string.
func hkdfCombined(intermediateBits, secretKeyRaw, info []byte) ([]byte, error) {
	ikm := append(append([]byte{}, intermediateBits...), secretKeyRaw...)
	return hkdf.Key(sha256.New, ikm, nil, string(info), aesKeyLength)
}

// DeriveKek matches recoveryCredential.ts's deriveZkKek -- requires both
// Recovery Credential halves.
func DeriveKek(passphrase string, secretKeyRaw []byte, params KdfParams) ([]byte, error) {
	intermediateBits, err := deriveIntermediateBits(passphrase, params)
	if err != nil {
		return nil, err
	}
	return hkdfCombined(intermediateBits, secretKeyRaw, kekInfo)
}

// DeriveVerifierHash matches recoveryCredential.ts's deriveZkVerifierHash
// -- independent of the KEK by construction (different HKDF info string),
// so a leaked verifier can't be used to derive the KEK, and computing it
// still requires both credential halves, not just the passphrase.
func DeriveVerifierHash(passphrase string, secretKeyRaw []byte, params KdfParams) (string, error) {
	intermediateBits, err := deriveIntermediateBits(passphrase, params)
	if err != nil {
		return "", err
	}
	bits, err := hkdfCombined(intermediateBits, secretKeyRaw, verifierInfo)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bits), nil
}

// UnwrapDek AES-GCM-decrypts the account DEK using the KEK -- matches
// WebCrypto's unwrapKey with an AES-GCM wrapping algorithm, which is just
// AES-GCM decrypt of the raw key bytes.
func UnwrapDek(kek []byte, wrappedDekB64, wrappedDekIVB64 string) ([]byte, error) {
	return decryptRaw(kek, wrappedDekB64, wrappedDekIVB64)
}

func decryptRaw(key []byte, ciphertextB64, ivB64 string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, iv, ciphertext, nil)
}

// decryptEnvelope is decrypt's Go-side twin of accountCrypto.ts's
// decrypt() (via vaultCrypto.ts) -- ciphertext/tag are stored separately
// in the envelope but AES-GCM.Open wants them concatenated (Web Crypto's
// default AES-GCM output shape too, same as notecrypto.go's decrypt).
func decryptEnvelope(dek []byte, payload EncryptedPayload) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(payload.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	tag, err := base64.StdEncoding.DecodeString(payload.Tag)
	if err != nil {
		return nil, fmt.Errorf("decode tag: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(payload.IV)
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	sealed := append(append([]byte{}, ciphertext...), tag...)
	return gcm.Open(nil, iv, sealed, nil)
}

// DecryptString matches accountCrypto.ts's decryptString -- encrypted is
// a JSON-stringified EncryptedPayload (the wire shape of a ZK note's
// title, and of each individual ZK-encrypted tag).
func DecryptString(dek []byte, encrypted string) (string, error) {
	var payload EncryptedPayload
	if err := json.Unmarshal([]byte(encrypted), &payload); err != nil {
		return "", fmt.Errorf("parse encrypted payload: %w", err)
	}
	plaintext, err := decryptEnvelope(dek, payload)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// DecryptTags matches accountCrypto.ts's decryptTags: each tag decrypts
// independently, and one that fails (e.g. leftover plaintext from before
// this account was ZK) is dropped rather than failing the whole list.
func DecryptTags(dek []byte, encrypted []string) []string {
	tags := make([]string, 0, len(encrypted))
	for _, enc := range encrypted {
		if plain, err := DecryptString(dek, enc); err == nil {
			tags = append(tags, plain)
		}
	}
	return tags
}

// DecryptBody matches accountCrypto.ts's decryptJson as used by
// decryptNoteRead: body arrives as the EncryptedPayload object itself
// (not a JSON string like title/tags -- see zkFieldCrypto.ts's
// encryptNoteWrite, which JSON.parses encryptJson's string output before
// assigning it to the body field), so it's re-marshaled to a JSON string
// here before going through the same envelope-decrypt path, then parsed
// as the Tiptap body it decrypts to.
func DecryptBody(dek []byte, body map[string]any) (map[string]any, error) {
	if body == nil {
		return nil, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("re-marshal envelope: %w", err)
	}
	plaintext, err := DecryptString(dek, string(raw))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(plaintext), &out); err != nil {
		return nil, fmt.Errorf("parse decrypted body: %w", err)
	}
	return out, nil
}

// secretKeyRawBytes/secretKeyGroupSize match recoveryCredential.ts's
// SECRET_KEY_RAW_BYTES/SECRET_KEY_GROUP_SIZE.
const secretKeyRawBytes = 16

var base32Encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ParseRecoverySecretKey matches recoveryCredential.ts's
// parseRecoverySecretKey: strips the display hyphens, base32-decodes (RFC
// 4648, no padding -- the same alphabet Go's stdlib base32 already uses),
// and verifies the trailing checksum byte (the first byte of the raw
// key's own SHA-256 digest) so a mistyped character is caught here
// instead of surfacing as an opaque "wrong credential" error much later
// at unlock.
func ParseRecoverySecretKey(formatted string) ([]byte, error) {
	clean := strings.ToUpper(strings.ReplaceAll(formatted, "-", ""))
	clean = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '2' && r <= '7') {
			return r
		}
		return -1
	}, clean)
	decoded, err := base32Encoding.DecodeString(clean)
	if err != nil {
		return nil, fmt.Errorf("decode Secret Key: %w", err)
	}
	if len(decoded) != secretKeyRawBytes+1 {
		return nil, fmt.Errorf("Secret Key has the wrong length")
	}
	raw, checksum := decoded[:secretKeyRawBytes], decoded[secretKeyRawBytes]
	digest := sha256.Sum256(raw)
	if digest[0] != checksum {
		return nil, fmt.Errorf("Secret Key checksum doesn't match -- check for a typo")
	}
	return raw, nil
}

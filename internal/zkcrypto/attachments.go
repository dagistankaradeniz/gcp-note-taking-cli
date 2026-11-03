package zkcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// Attachment files (accountCrypto.ts's encryptFile/decryptFile): a random
// per-file key, wrapped under the account DEK (v2: AAD binds it to the
// attachment's metadata) or, for a locked/Vault note, under that note's
// own key (v1: no AAD). The bytes are a sequence of chunks, each
//   [4-byte big-endian ciphertext length][12-byte IV][ciphertext || tag]
// and a ZK file's plaintext is Padmé-padded before encryption.

const (
	chunkLengthPrefix = 4
	ivLength          = 12
)

// Bind is the binding id the opener has read (after a successful open).
func (o *Opener) Bind() string { return o.bind }

// UnwrapFileKey opens a ZK attachment's file key (accountCrypto.ts's
// unwrapFileKey): AES-GCM under the account DEK with the attachment's
// binding id in the AAD, so it can't be swapped onto another attachment.
func UnwrapFileKey(dek []byte, wrappedB64, ivB64, bind string) ([]byte, error) {
	if !bindPattern.MatchString(bind) {
		return nil, fmt.Errorf("invalid binding id")
	}
	return openGCM(dek, wrappedB64, ivB64, aad("attachment", "file_key", bind))
}

// UnwrapKey opens a v1-wrapped key (vaultCrypto.ts's unwrapDekWithKek):
// a locked or Vault note's attachment file key.
func UnwrapKey(kek []byte, wrappedB64, ivB64 string) ([]byte, error) {
	return openGCM(kek, wrappedB64, ivB64, nil)
}

func openGCM(key []byte, ciphertextB64, ivB64 string, additional []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode wrapped key: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	out, err := gcm.Open(nil, iv, ciphertext, additional)
	if err != nil {
		return nil, fmt.Errorf("unwrap key: %w", err)
	}
	return out, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// DecryptFile decrypts a chunked attachment file.
func DecryptFile(fileKey, data []byte) ([]byte, error) {
	gcm, err := newGCM(fileKey)
	if err != nil {
		return nil, err
	}
	var out []byte
	for off := 0; off < len(data); {
		if len(data)-off < chunkLengthPrefix+ivLength {
			return nil, fmt.Errorf("attachment is truncated")
		}
		n := int(binary.BigEndian.Uint32(data[off:]))
		off += chunkLengthPrefix
		iv := data[off : off+ivLength]
		off += ivLength
		if n > len(data)-off {
			return nil, fmt.Errorf("attachment is truncated")
		}
		plain, err := gcm.Open(nil, iv, data[off:off+n], nil)
		if err != nil {
			return nil, fmt.Errorf("decrypt attachment: %w", err)
		}
		out = append(out, plain...)
		off += n
	}
	return out, nil
}

// UnpadFile strips a ZK file's Padmé padding.
func UnpadFile(plain []byte) ([]byte, error) { return unpad(plain) }

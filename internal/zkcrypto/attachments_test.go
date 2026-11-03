package zkcrypto

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"testing"
)

// encryptFileForTest is accountCrypto.ts's encryptFile with a tiny chunk
// size, so several chunks are exercised.
func encryptFileForTest(t *testing.T, key, plain []byte, chunk int) []byte {
	t.Helper()
	gcm, err := newGCM(key)
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	for off := 0; off == 0 || off < len(plain); off += chunk {
		end := min(off+chunk, len(plain))
		iv := make([]byte, ivLength)
		_, _ = rand.Read(iv)
		ct := gcm.Seal(nil, iv, plain[off:end], nil)
		out = binary.BigEndian.AppendUint32(out, uint32(len(ct)))
		out = append(out, iv...)
		out = append(out, ct...)
	}
	return out
}

func TestDecryptFileChunks(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	plain := bytes.Repeat([]byte("quillink "), 100)
	enc := encryptFileForTest(t, key, plain, 64)
	got, err := DecryptFile(key, enc)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip: %v", err)
	}
	empty := encryptFileForTest(t, key, nil, 64)
	if got, err := DecryptFile(key, empty); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %d", err, len(got))
	}
	enc[len(enc)-1] ^= 1
	if _, err := DecryptFile(key, enc); err == nil {
		t.Fatal("tampered chunk accepted")
	}
	if _, err := DecryptFile(key, enc[:10]); err == nil {
		t.Fatal("truncated file accepted")
	}
}

func TestUnwrapFileKeyBindsToAttachment(t *testing.T) {
	dek := make([]byte, 32)
	fileKey := make([]byte, 32)
	_, _ = rand.Read(dek)
	_, _ = rand.Read(fileKey)
	bind := "AAAAAAAAAAAAAAAAAAAAAA"
	gcm, _ := newGCM(dek)
	iv := make([]byte, ivLength)
	wrapped := gcm.Seal(nil, iv, fileKey, aad("attachment", "file_key", bind))
	w, i := base64.StdEncoding.EncodeToString(wrapped), base64.StdEncoding.EncodeToString(iv)
	got, err := UnwrapFileKey(dek, w, i, bind)
	if err != nil || !bytes.Equal(got, fileKey) {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := UnwrapFileKey(dek, w, i, "BBBBBBBBBBBBBBBBBBBBBB"); err == nil {
		t.Fatal("key opened under another attachment's binding")
	}
	if _, err := UnwrapKey(dek, w, i); err == nil {
		t.Fatal("v2-wrapped key opened without its AAD")
	}
}

func TestUnpadFile(t *testing.T) {
	padded := append([]byte("data"), 0x80, 0, 0)
	if got, err := UnpadFile(padded); err != nil || string(got) != "data" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := UnpadFile([]byte("data")); err == nil {
		t.Fatal("unpadded accepted")
	}
}

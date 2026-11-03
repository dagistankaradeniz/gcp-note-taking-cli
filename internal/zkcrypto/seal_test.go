package zkcrypto

import (
	"strings"
	"testing"
)

// Sealing the shared vector's plaintext with its key, IV and binding id
// must give the web's envelope byte for byte (zkHardening.test.ts opens the
// same vector), so what the CLI writes is what the apps read.
func TestSealMatchesWebVector(t *testing.T) {
	iv := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	got, err := sealWithIV(vectorKey(), "note", "title", "AAECAwQFBgcICQoLDA0ODw", "Zero-Knowledge test note", iv)
	if err != nil {
		t.Fatal(err)
	}
	if got != v2Vector {
		t.Fatalf("got\n%s\nwant\n%s", got, v2Vector)
	}
}

func TestSealRoundTrip(t *testing.T) {
	bind, err := NewBindingID()
	if err != nil || !bindPattern.MatchString(bind) {
		t.Fatalf("binding id %q: %v", bind, err)
	}
	title, _ := SealString(vectorKey(), "note", "title", bind, "Plan")
	body, err := SealBody(vectorKey(), bind, map[string]any{"type": "doc"})
	if err != nil {
		t.Fatal(err)
	}
	open := NewOpener(vectorKey(), "note")
	if got, err := open.String("title", title); err != nil || got != "Plan" {
		t.Fatalf("title %q %v", got, err)
	}
	if got, err := open.Body("body", body); err != nil || got["type"] != "doc" {
		t.Fatalf("body %v %v", got, err)
	}
	if open.Bind() != bind {
		t.Fatal("binding id not shared")
	}
	if _, err := SealString(vectorKey(), "note", "title", "short", "x"); err == nil {
		t.Fatal("invalid binding id accepted")
	}
	// Same plaintext, fresh IV each time.
	again, _ := SealString(vectorKey(), "note", "title", bind, "Plan")
	if again == title || !strings.Contains(again, bind) {
		t.Fatal("IV reused")
	}
}

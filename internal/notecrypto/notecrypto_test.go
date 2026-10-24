package notecrypto

import (
	"errors"
	"testing"
)

func TestDerive_RefusesWeakIterations(t *testing.T) {
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeriveKey("password", salt, 100_000); !errors.Is(err, ErrWeakIterations) {
		t.Errorf("DeriveKey err = %v, want ErrWeakIterations", err)
	}
	if _, err := DeriveVerifierHash("password", salt, 100_000); !errors.Is(err, ErrWeakIterations) {
		t.Errorf("DeriveVerifierHash err = %v, want ErrWeakIterations", err)
	}
}

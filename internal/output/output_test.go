package output

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestJSONEnvelope(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	type payload struct {
		Foo string `json:"foo"`
	}
	if err := JSON(payload{Foo: "bar"}); err != nil {
		t.Fatal(err)
	}
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var got Envelope
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v (%s)", err, buf.String())
	}
	if got.SchemaVersion != SchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, SchemaVersion)
	}
	data, ok := got.Data.(map[string]any)
	if !ok || data["foo"] != "bar" {
		t.Errorf("data = %v, want {foo: bar}", got.Data)
	}
}

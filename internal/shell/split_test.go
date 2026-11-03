package shell

import (
	"reflect"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"note list", []string{"note", "list"}},
		{`note create "Weekly review" --tags work,q4`, []string{"note", "create", "Weekly review", "--tags", "work,q4"}},
		{`note search 'it''s'`, []string{"note", "search", "its"}},
		{`note search "say \"hi\""`, []string{"note", "search", `say "hi"`}},
		{`note create a\ b`, []string{"note", "create", "a b"}},
		{`note create ""`, []string{"note", "create", ""}},
	}
	for _, c := range cases {
		got, err := Split(c.in)
		if err != nil {
			t.Errorf("Split(%q) error: %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Split(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{`note "open`, `note 'open`, `note \`} {
		if _, err := Split(bad); err == nil {
			t.Errorf("Split(%q) should fail", bad)
		}
	}
}

func TestHasSecretFlag(t *testing.T) {
	if !HasSecretFlag([]string{"note", "list", "--token", "x"}) || !HasSecretFlag([]string{"--token=x"}) {
		t.Error("missed --token")
	}
	if HasSecretFlag([]string{"note", "list", "--tokens"}) {
		t.Error("false positive")
	}
}

func TestComplete(t *testing.T) {
	cands := []string{"note", "login", "logout", "org"}
	if got, ok := Complete("no", cands); !ok || got != "note " {
		t.Errorf("single match: %q %v", got, ok)
	}
	if got, ok := Complete("l", cands); !ok || got != "log" {
		t.Errorf("common prefix: %q %v", got, ok)
	}
	if _, ok := Complete("log", cands); ok {
		t.Error("ambiguous with nothing to add should not complete")
	}
	if _, ok := Complete("x", cands); ok {
		t.Error("no match should not complete")
	}
}

package cmd

import (
	"reflect"
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
)

func TestClassifyShellWords(t *testing.T) {
	const (
		C = session.Command
		F = session.Flag
		A = session.Arg
		U = session.Unknown
	)
	cases := []struct {
		line []string
		want []session.Kind
	}{
		{[]string{"note", "list", "--tags", "a,b"}, []session.Kind{C, C, F, A}},
		{[]string{"note", "get", "abc", "--json"}, []session.Kind{C, C, A, F}},
		{[]string{"note", "create", "Ideas", "--pinned", "x"}, []session.Kind{C, C, A, F, A}},
		{[]string{"note", "list", "--output=csv"}, []session.Kind{C, C, F}},
		{[]string{"noet", "list"}, []session.Kind{U, A}},
		{[]string{"note", "lsit"}, []session.Kind{C, U}},
		{[]string{"exit"}, []session.Kind{C}},
		{[]string{"quillink", "whoami"}, []session.Kind{C, C}},
	}
	for _, c := range cases {
		if got := classifyShellWords(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %v, want %v", c.line, got, c.want)
		}
	}
}

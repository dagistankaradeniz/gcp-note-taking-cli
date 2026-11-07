package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func listed(t *testing.T, kind string, ids, titles []string) {
	t.Helper()
	t.Setenv("QUILLINK_TOKEN", "test-token")
	listingKind = kind
	rememberRows(ids, titles)
	t.Cleanup(forgetRows)
}

func TestPickRows(t *testing.T) {
	listed(t, "note", []string{"aaa", "bbb"}, []string{"First", "Second"})
	listed(t, "attachment", []string{"att1"}, []string{"a.png"})

	cases := []struct{ in, want []string }{
		{[]string{"note", "get", "2"}, []string{"note", "get", "bbb"}},
		{[]string{"note", "get", "--json", "1"}, []string{"note", "get", "--json", "aaa"}},
		{[]string{"note", "attachment", "download", "2", "1"}, []string{"note", "attachment", "download", "bbb", "att1"}},
		// Not ids: a title, a query, a tag, and a flag's value.
		{[]string{"note", "create", "2"}, []string{"note", "create", "2"}},
		{[]string{"note", "search", "2"}, []string{"note", "search", "2"}},
		{[]string{"tag", "show", "2"}, []string{"tag", "show", "2"}},
		{[]string{"note", "list", "--limit", "2"}, []string{"note", "list", "--limit", "2"}},
		{[]string{"note", "get", "abc123"}, []string{"note", "get", "abc123"}},
	}
	for _, c := range cases {
		got, err := pickRows(c.in)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: got %v, %v", c.in, got, err)
		}
	}
}

func TestPickRowsErrors(t *testing.T) {
	listed(t, "note", []string{"aaa"}, []string{"First"})
	if _, err := pickRows([]string{"note", "delete", "2"}); err == nil || !strings.Contains(err.Error(), "no row 2") {
		t.Errorf("row past the end: %v", err)
	}
	if _, err := pickRows([]string{"task", "get", "1"}); err == nil || !strings.Contains(err.Error(), "no task list") {
		t.Errorf("a note list says nothing about tasks: %v", err)
	}
}

func TestKindOf(t *testing.T) {
	cases := map[string]string{
		"note list":            "note",
		"tag show x":           "note",
		"shared":               "note",
		"task list":            "task",
		"task category list":   "category",
		"note attachment list": "attachment",
		"canvas list":          "canvas",
	}
	for line, want := range cases {
		if got := kindOf(commandFor(strings.Fields(line))); got != want {
			t.Errorf("%s: got %q, want %q", line, got, want)
		}
	}
}

func TestSuggestIDs(t *testing.T) {
	listed(t, "note", []string{"aaa", "bbb"}, []string{"Shopping list", "📌 Trip plan"})
	listed(t, "folder", []string{"fff"}, []string{"Work"})

	ids := func(line string) []string {
		var out []string
		sugs, _ := suggestShellLine(line, len(line))
		for _, s := range sugs {
			out = append(out, s.Text)
		}
		return out
	}
	cases := []struct {
		line string
		want []string
	}{
		{"note get ", []string{"aaa", "bbb"}},
		{"note get aa", []string{"aaa"}},
		{"note get trip", []string{"bbb"}},
		{"note get 2", []string{"bbb"}},
		{"note list --folder ", []string{"fff"}},
		{"note get aaa ", nil},
		{"note create ", nil},
	}
	for _, c := range cases {
		if got := ids(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v, want %v", c.line, got, c.want)
		}
	}
}

func TestRememberedListKeepsOlderIDsForCompletion(t *testing.T) {
	listed(t, "note", []string{"old"}, []string{"Old"})
	listed(t, "note", []string{"new"}, []string{"New"})
	sugs, _ := suggestShellLine("note get ", len("note get "))
	if len(sugs) != 2 || sugs[0].Text != "new" || sugs[1].Text != "old" {
		t.Fatalf("got %v", sugs)
	}
	if _, err := pickRows([]string{"note", "get", "2"}); err == nil {
		t.Fatal("row numbers only cover the last list")
	}
}

func TestCacheableLine(t *testing.T) {
	t.Setenv("QUILLINK_TOKEN", "test-token")
	cases := map[string]bool{
		"note list":             true,
		"note list --limit 5":   true,
		"task due":              true,
		"note get abc":          false,
		"note create x":         false,
		"note list --json":      false,
		"note list --output=md": false,
		"note list --out a.md":  false,
		"note list --token x":   false,
	}
	for line, want := range cases {
		if got := cacheableLine(line); got != want {
			t.Errorf("%q: got %v, want %v", line, got, want)
		}
	}
}

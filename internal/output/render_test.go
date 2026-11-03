package output

import (
	"bytes"
	"strings"
	"testing"
)

type testNote struct {
	ID    string         `json:"id"`
	Title string         `json:"title"`
	Tags  []string       `json:"tags"`
	Body  map[string]any `json:"body"`
}

type testList struct {
	Notes   []testNote `json:"notes"`
	HasMore bool       `json:"has_more"`
}

func render(t *testing.T, f func(*bytes.Buffer, any) error, v any) string {
	t.Helper()
	data, err := toOrdered(v)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := f(&b, data); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func csvOf(b *bytes.Buffer, v any) error  { return writeCSV(b, v) }
func mdOf(b *bytes.Buffer, v any) error   { return writeMarkdown(b, v) }
func yamlOf(b *bytes.Buffer, v any) error { return writeYAML(b, v) }

func sample() testList {
	doc := map[string]any{"type": "doc", "content": []any{}}
	return testList{Notes: []testNote{
		{ID: "1", Title: "Weekly review", Tags: []string{"work", "q4"}, Body: doc},
		{ID: "2", Title: "a | b\nc", Tags: nil, Body: doc},
	}, HasMore: true}
}

func TestCSVUsesTheListAndKeepsKeyOrder(t *testing.T) {
	BodyText = func(map[string]any) string { return "body text" }
	defer func() { BodyText = nil }()
	got := render(t, csvOf, sample())
	lines := strings.Split(got, "\n")
	if lines[0] != "id,title,tags,body" {
		t.Fatalf("header = %q", lines[0])
	}
	if lines[1] != `1,Weekly review,"work, q4",body text` {
		t.Errorf("row = %q", lines[1])
	}
	if strings.Contains(got, "has_more") {
		t.Error("paging field leaked into CSV")
	}
}

func TestCSVKeyValueForObjects(t *testing.T) {
	got := render(t, csvOf, map[string]any{"email": "a@b.c"})
	if got != "field,value\nemail,a@b.c\n" {
		t.Errorf("got %q", got)
	}
	got = render(t, csvOf, []string{"work", "home"})
	if got != "value\nwork\nhome\n" {
		t.Errorf("scalars: got %q", got)
	}
}

func TestMarkdownEscapes(t *testing.T) {
	got := render(t, mdOf, sample())
	if !strings.HasPrefix(got, "| id | title | tags | body |\n| --- | --- | --- | --- |\n") {
		t.Fatalf("table head = %q", got)
	}
	if !strings.Contains(got, `| a \| b<br>c |`) {
		t.Errorf("pipe/newline not escaped: %q", got)
	}
}

func TestYAML(t *testing.T) {
	got := render(t, yamlOf, map[string]any{
		"title": "Weekly review", "tags": []string{"work", "yes"}, "count": 3, "none": nil, "empty": []string{},
	})
	for _, want := range []string{
		"count: 3", "empty: []", "none: null", `title: "Weekly review"`, "tags:\n  - work\n  - \"yes\"",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestParseFormatAndPath(t *testing.T) {
	if f, err := ParseFormat("Markdown"); err != nil || f != FormatMarkdown {
		t.Errorf("markdown alias: %v %v", f, err)
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("xml should be rejected")
	}
	for path, want := range map[string]Format{"a.CSV": FormatCSV, "a.yml": FormatYAML, "a.json": FormatJSON, "a.md": FormatMarkdown, "a.txt": FormatTable} {
		if got := FormatForPath(path); got != want {
			t.Errorf("%s -> %s, want %s", path, got, want)
		}
	}
}

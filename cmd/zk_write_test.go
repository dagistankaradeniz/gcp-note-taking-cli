package cmd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
)

func zkKeyForTest() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = 9
	}
	return k
}

func TestSealNoteCreateRoundTrip(t *testing.T) {
	dek := zkKeyForTest()
	req := client.NoteCreateRequest{Title: "Plan", Tags: []string{"work", "q4"}}
	if err := sealNoteCreate(nil, dek, &req); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.Title, "Plan") || strings.Contains(strings.Join(req.Tags, ""), "work") {
		t.Fatal("plaintext left in the request")
	}
	n := client.Note{Title: req.Title, Tags: req.Tags}
	decryptNoteZK(dek, &n)
	if n.Title != "Plan" || !reflect.DeepEqual(n.Tags, []string{"work", "q4"}) {
		t.Fatalf("got %+v", n)
	}
}

func TestSealTaskWrites(t *testing.T) {
	dek := zkKeyForTest()
	desc := "Ask about Friday"
	req := client.TaskCreateRequest{Title: "Call", Description: &desc}
	if err := sealTaskCreate(dek, &req); err != nil {
		t.Fatal(err)
	}
	task := client.Task{Title: req.Title, Description: req.Description}
	decryptTaskZK(dek, &task)
	if task.Title != "Call" || *task.Description != desc || task.ZkUnreadable {
		t.Fatalf("got %+v", task)
	}

	due := "2026-11-07T09:00:00Z"
	if err := sealTaskCreate(dek, &client.TaskCreateRequest{Title: "x", DueAt: &due}); err == nil {
		t.Fatal("due date accepted for ZK")
	}

	// Title and description together: fresh binding id, no lookup needed.
	body := map[string]any{"title": "New", "description": ""}
	if err := sealTaskWrite(nil, dek, "task1", body); err != nil {
		t.Fatal(err)
	}
	if body["description"] != nil {
		t.Fatal("cleared description should be sent as null")
	}
	if _, err := zkcrypto.NewOpener(dek, "task").String("title", body["title"].(string)); err != nil {
		t.Fatalf("title doesn't open: %v", err)
	}
}

func TestSealFolderName(t *testing.T) {
	dek := zkKeyForTest()
	sealed, err := sealFolderName(dek, "Projects")
	if err != nil {
		t.Fatal(err)
	}
	f := client.Folder{Name: sealed}
	decryptFolderZK(dek, &f)
	if f.Name != "Projects" {
		t.Fatalf("got %q", f.Name)
	}
}

func TestExtractGraphLinks(t *testing.T) {
	link := func(id, ref string) map[string]any {
		attrs := map[string]any{"id": id}
		if ref != "" {
			attrs["refType"] = ref
		}
		return map[string]any{"type": "noteLink", "attrs": attrs}
	}
	body := map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{link("n1", ""), link("c1", "canvas"), link("n1", "note"), link("n2", "")}},
	}}
	notes, canvases := extractGraphLinks(body)
	if !reflect.DeepEqual(notes, []string{"n1", "n2"}) || !reflect.DeepEqual(canvases, []string{"c1"}) {
		t.Fatalf("got %v %v", notes, canvases)
	}
	if n, c := extractGraphLinks(nil); len(n) != 0 || len(c) != 0 {
		t.Fatal("nil body")
	}
}

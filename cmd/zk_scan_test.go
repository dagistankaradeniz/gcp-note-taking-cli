package cmd

import (
	"testing"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

func scanOf(notes ...client.Note) *zkScan {
	for i := range notes {
		if notes[i].Status == "" {
			notes[i].Status = "active"
		}
	}
	return &zkScan{notes: notes, canvases: map[string]string{"c1": "Board"}}
}

func TestZkTagCounts(t *testing.T) {
	s := scanOf(
		client.Note{ID: "a", Tags: []string{"Work", "urgent", "work"}},
		client.Note{ID: "b", Tags: []string{"work"}},
		client.Note{ID: "c", Tags: []string{"home"}},
	)
	got := s.tagCounts()
	if len(got) != 3 || got[0].Name != "Work" || got[0].Notes != 2 || got[1].Name != "home" {
		t.Fatalf("got %+v", got)
	}
}

func TestZkWithTags(t *testing.T) {
	s := scanOf(
		client.Note{ID: "a", Tags: []string{"work", "urgent"}},
		client.Note{ID: "b", Tags: []string{"work"}},
	)
	if n := s.withTags([]string{"Work", "urgent"}, true); len(n) != 1 || n[0].ID != "a" {
		t.Fatalf("all: %+v", n)
	}
	if n := s.withTags([]string{"urgent", "home"}, false); len(n) != 1 {
		t.Fatalf("any: %+v", n)
	}
}

func TestZkRelatedMirrorsServerScoring(t *testing.T) {
	s := scanOf(
		client.Note{ID: "src", Title: "Source", Tags: []string{"a", "b"}, LinksTo: []string{"hub"}},
		client.Note{ID: "hub", Title: "Hub", LinksTo: []string{"x"}},
		client.Note{ID: "x", Title: "Two hops", Tags: []string{"z"}},
		client.Note{ID: "y", Title: "Same tags", Tags: []string{"a", "b"}},
		client.Note{ID: "w", Title: "Weak", Tags: []string{"a", "q", "r", "s"}},
		client.Note{ID: "v", Title: "Vault", Tags: []string{"a", "b"}, VaultEncrypted: true},
	)
	got, ok := s.related("src", 5)
	if !ok {
		t.Fatal("source not found")
	}
	ids := map[string]client.NoteRelatedResult{}
	for _, r := range got {
		ids[r.ID] = r
	}
	if got[0].ID != "y" || got[0].Score != 1 {
		t.Fatalf("top %+v", got[0])
	}
	if _, linked := ids["hub"]; linked {
		t.Fatal("already-linked note suggested")
	}
	// x: one path through hub (degree 2): 0.25 · 2/log2(4) = 0.25 < 0.3.
	if _, ok := ids["x"]; ok {
		t.Fatal("x should fall under the minimum score")
	}
	// w: jaccard 1/5 = 0.2 < 0.3.
	if _, ok := ids["w"]; ok {
		t.Fatal("weak tag overlap suggested")
	}
	if _, ok := ids["v"]; ok {
		t.Fatal("vault note suggested")
	}
	if _, ok := s.related("missing", 5); ok {
		t.Fatal("missing note found")
	}
}

func TestZkTitles(t *testing.T) {
	s := scanOf(client.Note{ID: "a"})
	m := s.titles()
	if m["a"] != "Untitled note" || m["c1"] != "Board" {
		t.Fatalf("%v", m)
	}
}

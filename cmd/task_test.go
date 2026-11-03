package cmd

import (
	"testing"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

func TestParseDue(t *testing.T) {
	due, allDay, err := parseDue("2026-11-05")
	if err != nil || !allDay {
		t.Fatalf("date: %v %v", allDay, err)
	}
	want := time.Date(2026, 11, 5, 9, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)
	if due != want {
		t.Errorf("all-day due = %s, want %s (09:00 local)", due, want)
	}

	due, allDay, err = parseDue("2026-11-05 14:30")
	if err != nil || allDay {
		t.Fatalf("datetime: %v %v", allDay, err)
	}
	if want := time.Date(2026, 11, 5, 14, 30, 0, 0, time.Local).UTC().Format(time.RFC3339); due != want {
		t.Errorf("timed due = %s, want %s", due, want)
	}

	for _, bad := range []string{"tomorrow", "2026-13-01", "05/11/2026"} {
		if _, _, err := parseDue(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}

func TestFormatDue(t *testing.T) {
	at := time.Date(2026, 11, 5, 14, 30, 0, 0, time.Local).UTC().Format(time.RFC3339)
	if got := formatDue(client.Task{DueAt: &at}); got != "2026-11-05 14:30" {
		t.Errorf("timed: %q", got)
	}
	if got := formatDue(client.Task{DueAt: &at, AllDay: true}); got != "2026-11-05" {
		t.Errorf("all day: %q", got)
	}
	occ := "2026-11-12"
	if got := formatDue(client.Task{DueAt: &at, AllDay: true, OccurrenceDate: &occ}); got != occ {
		t.Errorf("occurrence: %q", got)
	}
	if got := formatDue(client.Task{}); got != "-" {
		t.Errorf("no due: %q", got)
	}
}

func TestDecryptTaskZKFailsClosed(t *testing.T) {
	desc := "server-planted text"
	task := client.Task{Title: "not an envelope", Description: &desc}
	decryptTaskZK(make([]byte, 32), &task)
	if task.Title != zkUnreadableText || task.Description != nil || !task.ZkUnreadable {
		t.Errorf("undecryptable task leaked server text: %+v", task)
	}

	plain := client.Task{Title: "Buy milk"}
	decryptTaskZK(nil, &plain)
	if plain.Title != "Buy milk" {
		t.Error("non-ZK task changed")
	}
}

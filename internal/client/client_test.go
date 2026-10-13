package client

import (
	"net/url"
	"testing"
)

func TestRequireHTTPS(t *testing.T) {
	cases := map[string]bool{
		"https://note-taking-app-prod.web.app/v1/notes": true,
		"http://localhost:8000/v1/notes":                true,
		"http://127.0.0.1:8000/v1/notes":                true,
		"http://[::1]:8000/v1/notes":                    true,
		"http://example.com/v1/notes":                   false,
		"http://localhost.evil.com/v1/notes":            false,
		"ftp://example.com/":                            false,
	}
	for raw, ok := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got := requireHTTPS(u) == nil; got != ok {
			t.Errorf("requireHTTPS(%s) allowed=%v, want %v", raw, got, ok)
		}
	}
}

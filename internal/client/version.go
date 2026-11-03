package client

import (
	"runtime/debug"
	"time"
)

// Version is the CLI's own version string, sent as part of the User-Agent
// on every request (see API Access -> Usage tracking) and overridden at
// build time via -ldflags "-X .../internal/client.Version=...".
var Version = "dev"

// Commit and BuildDate (RFC 3339) are injected by release builds the same
// way (see .goreleaser.yaml). Local builds leave them empty and fall back
// to the VCS stamp Go embeds in the binary (BuildStamp).
var (
	Commit    = ""
	BuildDate = ""
)

// BuildStamp returns the short commit and the commit time in UTC for the
// banner's version line. Either may be empty when nothing is known (e.g.
// `go run` outside a git checkout).
func BuildStamp() (commit string, at time.Time) {
	commit, date := Commit, BuildDate
	if commit == "" || date == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch {
				case s.Key == "vcs.revision" && commit == "":
					commit = s.Value
				case s.Key == "vcs.time" && date == "":
					date = s.Value
				}
			}
		}
	}
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if t, err := time.Parse(time.RFC3339, date); err == nil {
		at = t.UTC()
	}
	return commit, at
}

package client

// Version is the CLI's own version string, sent as part of the User-Agent
// on every request (see API Access -> Usage tracking) and overridden at
// build time via -ldflags "-X .../internal/client.Version=...".
var Version = "dev"

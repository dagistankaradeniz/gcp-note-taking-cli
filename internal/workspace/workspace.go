// Package workspace stores named environment/account profiles (a
// "workspace" = an API base URL + optional OAuth client_id) in a small
// shared registry file, so a user can define e.g. "prod", "staging", and
// "work" once and switch between them without re-typing --api-base every
// time. The registry holds no secrets -- the actual bearer credential for
// each workspace still lives in the OS keyring (or its file fallback),
// keyed by workspace name, via internal/auth.Store.
//
// This file lives at ~/.config/quillink/workspaces.json and is
// deliberately shared, byte-for-byte compatible JSON, with
// gcp-note-taking-mcp's Python auth module -- defining a workspace once
// with the CLI makes it visible to the MCP server too via
// QUILLINK_WORKSPACE (each tool still keeps its own separate keyring
// entry for the actual token).
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Workspace is one named environment/account profile.
type Workspace struct {
	Name     string `json:"name"`
	APIBase  string `json:"api_base"`
	ClientID string `json:"client_id,omitempty"`
}

// Registry is the on-disk shape of ~/.config/quillink/workspaces.json.
type Registry struct {
	Current    string               `json:"current,omitempty"`
	Workspaces map[string]Workspace `json:"workspaces"`
}

func registryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "quillink", "workspaces.json"), nil
}

// Load reads the registry, returning an empty one (not an error) if it
// doesn't exist yet -- most users never create a workspace.
func Load() (*Registry, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Registry{Workspaces: map[string]Workspace{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspaces.json: %w", err)
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse workspaces.json: %w", err)
	}
	if r.Workspaces == nil {
		r.Workspaces = map[string]Workspace{}
	}
	return &r, nil
}

// Save writes the registry back, chmod 600 like the credential file
// fallback (it contains no secrets, but api_base/client_id per workspace
// is still not something to leave world-readable).
func (r *Registry) Save() error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode workspaces.json: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write workspaces.json: %w", err)
	}
	return nil
}

// Get returns the named workspace, if defined.
func (r *Registry) Get(name string) (Workspace, bool) {
	w, ok := r.Workspaces[name]
	return w, ok
}

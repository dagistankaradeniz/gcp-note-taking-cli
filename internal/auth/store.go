// Package auth handles local credential storage and the device-grant
// login flow (see CLI Access Confluence page, "Local credential storage"
// and "Authentication" sections).
package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "quillink-cli"

	// defaultName is the legacy single-credential slot used by
	// `quillink login`/`logout`/`whoami` before workspaces existed.
	// NewStore() always resolves to this name, so an existing user who
	// never touches `quillink workspace` sees no change at all.
	defaultName = "default"
)

// Store persists the CLI's credential (a PAT or a device-grant-issued
// token) in the OS credential store where available, falling back to a
// chmod 600 file -- headless Linux CI runners commonly lack a Secret
// Service daemon, so this is a real fallback path, not an edge case.
//
// Each Store is scoped to a name -- the legacy single slot ("default",
// via NewStore) or a named workspace (via NewStoreFor) -- so multiple
// environments/accounts can each hold their own credential side by side
// (see internal/workspace).
type Store struct {
	name string
}

func NewStore() *Store { return &Store{name: defaultName} }

// NewStoreFor returns a Store scoped to a named workspace's credential
// slot, independent from the legacy default slot and from every other
// workspace's slot.
func NewStoreFor(name string) *Store { return &Store{name: name} }

func (s *Store) Save(token string) error {
	if err := keyring.Set(keyringService, s.name, token); err == nil {
		return nil
	}
	return s.saveToFile(token)
}

func (s *Store) Load() (string, error) {
	if token, err := keyring.Get(keyringService, s.name); err == nil {
		return token, nil
	}
	return s.loadFromFile()
}

func (s *Store) Delete() error {
	_ = keyring.Delete(keyringService, s.name)
	path, err := s.credentialFilePath()
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) saveToFile(token string) error {
	path, err := s.credentialFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		return fmt.Errorf("write credential file: %w", err)
	}
	return nil
}

func (s *Store) loadFromFile() (string, error) {
	path, err := s.credentialFilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("not logged in")
		}
		return "", err
	}
	return string(data), nil
}

// credentialFilePath keeps the legacy default slot's path byte-for-byte
// unchanged (~/.config/quillink/credential) and puts every named
// workspace's file-fallback credential in its own file alongside it.
func (s *Store) credentialFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if s.name == defaultName {
		return filepath.Join(home, ".config", "quillink", "credential"), nil
	}
	return filepath.Join(home, ".config", "quillink", "credentials", s.name), nil
}

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
	keyringUser    = "default"
)

// Store persists the CLI's credential (a PAT or a device-grant-issued
// token) in the OS credential store where available, falling back to a
// chmod 600 file -- headless Linux CI runners commonly lack a Secret
// Service daemon, so this is a real fallback path, not an edge case.
type Store struct{}

func NewStore() *Store { return &Store{} }

func (s *Store) Save(token string) error {
	if err := keyring.Set(keyringService, keyringUser, token); err == nil {
		return nil
	}
	return s.saveToFile(token)
}

func (s *Store) Load() (string, error) {
	if token, err := keyring.Get(keyringService, keyringUser); err == nil {
		return token, nil
	}
	return s.loadFromFile()
}

func (s *Store) Delete() error {
	_ = keyring.Delete(keyringService, keyringUser)
	path, err := credentialFilePath()
	if err != nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) saveToFile(token string) error {
	path, err := credentialFilePath()
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
	path, err := credentialFilePath()
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

func credentialFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "quillink", "credential"), nil
}

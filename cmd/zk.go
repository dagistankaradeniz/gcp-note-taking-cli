package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
	"golang.org/x/term"
)

// Zero-Knowledge accounts get read-only CLI access (see this repo's
// AGENTS.md / the backend's require_read_only_scopes_for_zk): every note/
// folder read command below calls resolveZkDek once, which is a no-op
// (nil, nil) for a non-ZK account and the only place a ZK account's
// Recovery Credential (passphrase + Secret Key) is ever asked for. The
// derived DEK lives in memory for this single process invocation only --
// never written to the credential store, keychain, or any file. Every new
// `quillink` invocation re-derives it from scratch.

var (
	zkPassphraseStdin bool
	zkSecretKeyStdin  bool
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&zkPassphraseStdin, "zk-passphrase-stdin", false, "read the Zero-Knowledge Recovery Passphrase from stdin instead of prompting")
	rootCmd.PersistentFlags().BoolVar(&zkSecretKeyStdin, "zk-secret-key-stdin", false, "read the Zero-Knowledge Recovery Secret Key from stdin instead of prompting")
}

func readStdinLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// resolveZkPassphrase follows the same safe-input rules as
// note_unlock.go's resolvePassword -- never a literal flag value.
func resolveZkPassphrase() (string, error) {
	if zkPassphraseStdin {
		line, err := readStdinLine()
		if err != nil {
			return "", fmt.Errorf("read Recovery Passphrase from stdin: %w", err)
		}
		return line, nil
	}
	if env := os.Getenv("QUILLINK_ZK_PASSPHRASE"); env != "" {
		return env, nil
	}
	if isTerminalStdin() {
		fmt.Fprint(os.Stderr, "Zero-Knowledge Recovery Passphrase: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read Recovery Passphrase: %w", err)
		}
		return string(pw), nil
	}
	return "", fmt.Errorf("no Recovery Passphrase provided -- use an interactive terminal, --zk-passphrase-stdin, or QUILLINK_ZK_PASSPHRASE")
}

// resolveZkSecretKey mirrors resolveZkPassphrase, then parses/checksum-
// verifies the formatted Secret Key via zkcrypto.ParseRecoverySecretKey.
func resolveZkSecretKey() ([]byte, error) {
	var formatted string
	var err error
	switch {
	case zkSecretKeyStdin:
		formatted, err = readStdinLine()
		if err != nil {
			return nil, fmt.Errorf("read Recovery Secret Key from stdin: %w", err)
		}
	case os.Getenv("QUILLINK_ZK_SECRET_KEY") != "":
		formatted = os.Getenv("QUILLINK_ZK_SECRET_KEY")
	case isTerminalStdin():
		fmt.Fprint(os.Stderr, "Zero-Knowledge Recovery Secret Key: ")
		key, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("read Recovery Secret Key: %w", err)
		}
		formatted = string(key)
	default:
		return nil, fmt.Errorf("no Recovery Secret Key provided -- use an interactive terminal, --zk-secret-key-stdin, or QUILLINK_ZK_SECRET_KEY")
	}
	return zkcrypto.ParseRecoverySecretKey(formatted)
}

// isZkAccount is a cheap, prompt-free check for commands (like `note
// export`) that need to refuse cleanly for a Zero-Knowledge account
// *before* asking for the Recovery Credential -- asking for it only to
// then say "not supported" would be a worse experience than not asking
// at all.
func isZkAccount(c *client.Client) (bool, error) {
	var status client.ZkStatusResponse
	if err := c.Do("GET", "/v1/zk/status", nil, nil, &status); err != nil {
		return false, err
	}
	return status.SecurityTier == "zero_knowledge", nil
}

// resolveZkDek is a no-op for a non-ZK account (the common case -- every
// existing command's behavior is unchanged). For a ZK account it prompts
// for the Recovery Credential, derives the KEK/verifier locally, unlocks
// via POST /v1/zk/unlock, and returns the raw account DEK for this
// invocation's decrypt calls only.
func resolveZkDek(c *client.Client) ([]byte, error) {
	var status client.ZkStatusResponse
	if err := c.Do("GET", "/v1/zk/status", nil, nil, &status); err != nil {
		return nil, err
	}
	if status.SecurityTier != "zero_knowledge" {
		return nil, nil
	}
	if status.KdfAlgorithm == nil || status.Salt == nil {
		return nil, fmt.Errorf("account is Zero-Knowledge but has no key material on record -- contact support")
	}

	passphrase, err := resolveZkPassphrase()
	if err != nil {
		return nil, err
	}
	secretKeyRaw, err := resolveZkSecretKey()
	if err != nil {
		return nil, err
	}

	params := zkcrypto.KdfParams{Algorithm: *status.KdfAlgorithm, Salt: *status.Salt}
	if status.KdfIterations != nil {
		params.Iterations = *status.KdfIterations
	}
	if status.KdfMemoryKiB != nil {
		params.MemoryKiB = *status.KdfMemoryKiB
	}
	if status.KdfOps != nil {
		params.Ops = *status.KdfOps
	}

	kek, err := zkcrypto.DeriveKek(passphrase, secretKeyRaw, params)
	if err != nil {
		return nil, fmt.Errorf("derive account key: %w", err)
	}
	verifierHash, err := zkcrypto.DeriveVerifierHash(passphrase, secretKeyRaw, params)
	if err != nil {
		return nil, fmt.Errorf("derive verifier: %w", err)
	}

	var unlockResp client.ZkUnlockResponse
	req := client.ZkUnlockRequest{VerifierHash: verifierHash}
	if err := c.Do("POST", "/v1/zk/unlock", nil, req, &unlockResp); err != nil {
		return nil, fmt.Errorf("unlock (check your Recovery Passphrase and Secret Key): %w", err)
	}

	dek, err := zkcrypto.UnwrapDek(kek, unlockResp.WrappedDek, unlockResp.WrappedDekIV)
	if err != nil {
		return nil, fmt.Errorf("unwrap account key (check your Recovery Passphrase and Secret Key): %w", err)
	}
	return dek, nil
}

// decryptNoteZK decrypts n's title/body/tags in place when dek is non-nil
// -- a no-op otherwise. A locked (per-item password) or Vault note is
// opaque under a DIFFERENT key entirely (see zkFieldCrypto.ts's identical
// exclusion) and is left untouched. Title/body decrypt together, all-or-
// nothing: a note created before this account went ZK (or a race with an
// in-flight migration) may still be plaintext, so a decrypt failure
// leaves the note exactly as fetched rather than showing a half-decrypted
// result. Tags decrypt independently per-tag (see DecryptTags).
func decryptNoteZK(dek []byte, n *client.Note) {
	if dek == nil || n.NoteEncrypted || n.VaultEncrypted {
		return
	}
	title := n.Title
	if n.Title != "" {
		decrypted, err := zkcrypto.DecryptString(dek, n.Title)
		if err != nil {
			return
		}
		title = decrypted
	}
	var body map[string]any
	if n.Body != nil {
		decrypted, err := zkcrypto.DecryptBody(dek, n.Body)
		if err != nil {
			return
		}
		body = decrypted
	}
	n.Title = title
	n.Body = body
	n.Tags = zkcrypto.DecryptTags(dek, n.Tags)
}

// decryptFolderZK is decryptNoteZK's twin for a folder's name.
func decryptFolderZK(dek []byte, f *client.Folder) {
	if dek == nil {
		return
	}
	if f.Name == "" {
		return
	}
	if decrypted, err := zkcrypto.DecryptString(dek, f.Name); err == nil {
		f.Name = decrypted
	}
}

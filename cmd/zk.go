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

// stdinReader is shared, so --zk-passphrase-stdin and --zk-secret-key-stdin
// together read one line each: a reader per call could buffer past its line
// and swallow the next.
var stdinReader = bufio.NewReader(os.Stdin)

func readStdinLine() (string, error) {
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// The Recovery Credential is never read from the environment (workspace
// ZK_HARDENING_PLAN.md S10): env vars leak through shell history, CI logs
// and process listings, and this credential unlocks everything. A script
// that still sets one gets told how to switch instead of a silent prompt.
func refuseZkEnv(name, flag string) error {
	if os.Getenv(name) == "" {
		return nil
	}
	return fmt.Errorf("%s is no longer read -- pipe the value in with %s instead", name, flag)
}

// resolveZkPassphrase: an interactive masked prompt, or --zk-passphrase-stdin
// -- never a literal flag value or an env var.
func resolveZkPassphrase() (string, error) {
	if err := refuseZkEnv("QUILLINK_ZK_PASSPHRASE", "--zk-passphrase-stdin"); err != nil {
		return "", err
	}
	if zkPassphraseStdin {
		line, err := readStdinLine()
		if err != nil {
			return "", fmt.Errorf("read Recovery Passphrase from stdin: %w", err)
		}
		return line, nil
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
	return "", fmt.Errorf("no Recovery Passphrase provided -- use an interactive terminal or --zk-passphrase-stdin")
}

// resolveZkSecretKey mirrors resolveZkPassphrase, then parses/checksum-
// verifies the formatted Secret Key via zkcrypto.ParseRecoverySecretKey.
func resolveZkSecretKey() ([]byte, error) {
	if err := refuseZkEnv("QUILLINK_ZK_SECRET_KEY", "--zk-secret-key-stdin"); err != nil {
		return nil, err
	}
	var formatted string
	var err error
	switch {
	case zkSecretKeyStdin:
		formatted, err = readStdinLine()
		if err != nil {
			return nil, fmt.Errorf("read Recovery Secret Key from stdin: %w", err)
		}
	case isTerminalStdin():
		fmt.Fprint(os.Stderr, "Zero-Knowledge Recovery Secret Key: ")
		key, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return nil, fmt.Errorf("read Recovery Secret Key: %w", err)
		}
		formatted = string(key)
	default:
		return nil, fmt.Errorf("no Recovery Secret Key provided -- use an interactive terminal or --zk-secret-key-stdin")
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
// nothing, and fail closed (workspace ZK_HARDENING_PLAN.md S4): a note
// that doesn't decrypt shows zkUnreadableText and an empty body, never the
// server's text -- a malicious server could otherwise plant content that
// reads as the user's own. Tags decrypt independently per-tag (see
// DecryptTags).
func decryptNoteZK(dek []byte, n *client.Note) {
	if dek == nil || n.NoteEncrypted || n.VaultEncrypted {
		return
	}
	unreadable := func() {
		n.Title = zkUnreadableText
		if n.Body != nil {
			n.Body = map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}}
		}
		n.Tags = []string{}
	}
	title := n.Title
	if n.Title != "" {
		decrypted, err := zkcrypto.DecryptString(dek, n.Title)
		if err != nil {
			unreadable()
			return
		}
		title = decrypted
	}
	var body map[string]any
	if n.Body != nil {
		decrypted, err := zkcrypto.DecryptBody(dek, n.Body)
		if err != nil {
			unreadable()
			return
		}
		body = decrypted
	}
	n.Title = title
	n.Body = body
	n.Tags = zkcrypto.DecryptTags(dek, n.Tags)
}

// zkUnreadableText replaces a Zero-Knowledge field that doesn't decrypt --
// the same words the web and mobile apps show.
const zkUnreadableText = "Can't decrypt this item"

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
	} else {
		f.Name = zkUnreadableText
	}
}

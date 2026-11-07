package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/progress"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/session"
	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/zkcrypto"
)

// Zero-Knowledge accounts get read-only CLI access (see this repo's
// AGENTS.md / the backend's require_read_only_scopes_for_zk): every note/
// folder read command below calls resolveZkDek once, which is a no-op
// (nil, nil) for a non-ZK account and the only place a ZK account's
// Recovery Credential (passphrase + Secret Key) is ever asked for. The
// derived DEK lives in memory for this single process invocation only --
// never written to the credential store, keychain, or any file. Every new
// `quillink` invocation re-derives it from scratch. Inside the interactive
// session (bare `quillink`, interactive.go) it is kept for the session
// instead, still in memory only, and wiped after zkSessionIdle without
// use, on `lock`, and when the session ends. Locking also clears the
// session's output, so what the key decrypted doesn't outlive it.

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
	if session.Active() {
		return "", errStdinInSession("reading from stdin")
	}
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
		pw, err := askSecret("Zero-Knowledge Recovery Passphrase")
		if err != nil {
			return "", fmt.Errorf("read Recovery Passphrase: %w", err)
		}
		return pw, nil
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
		key, err := askSecret("Zero-Knowledge Recovery Secret Key")
		if err != nil {
			return nil, fmt.Errorf("read Recovery Secret Key: %w", err)
		}
		formatted = key
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
	owner := zkSessionOwner()
	if dek := zkSessionGet(owner); dek != nil {
		return dek, nil
	}

	passphrase, err := resolveZkPassphrase()
	if err != nil {
		return nil, err
	}
	secretKeyRaw, err := resolveZkSecretKey()
	if err != nil {
		return nil, err
	}
	defer clear(secretKeyRaw)

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

	unlocking := progress.Start("Unlocking your account key", 0)
	defer unlocking.Done()
	kek, err := zkcrypto.DeriveKek(passphrase, secretKeyRaw, params)
	if err != nil {
		return nil, fmt.Errorf("derive account key: %w", err)
	}
	defer clear(kek)
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
	if shellActive {
		zkSessionPut(owner, dek)
		fmt.Fprintf(os.Stderr, "Unlocked for this session. It locks after %d min idle, on `lock`, or on exit.\n", int(zkSessionIdle.Minutes()))
	}
	return dek, nil
}

const zkSessionIdle = 15 * time.Minute

// sessionKey is an unlocked key kept for the interactive session (the
// Zero-Knowledge account key, the Vault key). owner ties it to one API
// base + credential, so `login`, `logout` or `workspace use` mid-session
// never reuse another account's key.
type sessionKey struct {
	sync.Mutex
	owner string
	key   []byte
	timer *time.Timer
}

var zkSession, vaultSession sessionKey

func zkSessionOwner() string {
	sum := sha256.Sum256([]byte(resolveAPIBase() + "\x00" + resolveToken()))
	return hex.EncodeToString(sum[:])
}

// get returns the cached key for owner (and restarts the idle timer), or
// nil. Outside the interactive session nothing is cached.
func (s *sessionKey) get(owner string) []byte {
	s.Lock()
	defer s.Unlock()
	if !shellActive || s.key == nil || s.owner != owner {
		return nil
	}
	s.timer.Reset(zkSessionIdle)
	return s.key
}

func (s *sessionKey) put(owner string, key []byte) {
	s.wipe()
	s.Lock()
	defer s.Unlock()
	s.owner, s.key = owner, key
	s.timer = time.AfterFunc(zkSessionIdle, func() {
		if s.wipe() {
			session.Wiped(fmt.Sprintf("Locked after %d min idle. Output was cleared.", int(zkSessionIdle.Minutes())))
		}
	})
}

// wipe zeroes and drops the cached key; it reports whether one was held.
func (s *sessionKey) wipe() bool {
	s.Lock()
	defer s.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	held := s.key != nil
	clear(s.key)
	s.key, s.owner = nil, ""
	return held
}

func zkSessionGet(owner string) []byte      { return zkSession.get(owner) }
func zkSessionPut(owner string, dek []byte) { zkSession.put(owner, dek) }

// zkSessionWipe drops every key the session holds (account and Vault); it
// reports whether any was held.
func zkSessionWipe() bool {
	zk := zkSession.wipe()
	vault := vaultSession.wipe()
	return zk || vault
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
	open := zkcrypto.NewOpener(dek, "note")
	title := n.Title
	if n.Title != "" {
		decrypted, err := open.String("title", n.Title)
		if err != nil {
			unreadable()
			return
		}
		title = decrypted
	}
	var body map[string]any
	if n.Body != nil {
		decrypted, err := open.Body("body", n.Body)
		if err != nil {
			unreadable()
			return
		}
		body = decrypted
	}
	n.Title = title
	n.Body = body
	n.Tags = open.Tags("tags", n.Tags)
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
	if decrypted, err := zkcrypto.NewOpener(dek, "folder").String("name", f.Name); err == nil {
		f.Name = decrypted
	} else {
		f.Name = zkUnreadableText
	}
}

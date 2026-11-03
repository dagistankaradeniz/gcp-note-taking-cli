# Quillink CLI

Command-line client for [Quillink](https://github.com/dagistankaradeniz/gcp-note-taking-frontend), built on the same `/v1` API and authentication as the [API Access](https://dagistan.atlassian.net/wiki/spaces/GNTA/pages/34013185) layer. See the [CLI Access](https://dagistan.atlassian.net/wiki/spaces/GNTA/pages/34045953) Confluence page (GNTA space) for the full design and decisions log.

**Pro-only**, same gating as the API.

## Status

v1 command surface (auth + note CRUD) is implemented and verified end-to-end against a real browser + backend on both staging and prod: `quillink login` → approve via the `/device` page → CLI receives and stores the token → authenticated `/v1` calls succeed. OAuth clients are registered on both environments. `https://note-taking-app-prod.web.app` (the CLI's default `--api-base`) is live.

## Building from source

```sh
go build -o quillink .
```

Requires Go 1.26+.

## Usage

```sh
quillink login                         # device-grant browser login (opens the sign-in page; --no-browser to just print it)
quillink whoami

quillink note create "Title" --content "Body text"
quillink note list
quillink note get <id>
quillink note update <id> --title "New title" --pinned
quillink note search "query"
quillink note lock <id>                # password-protect a note (Plus plan+), prompts twice to confirm
quillink note unlock <id>              # reveal a password-protected note, prompts for a masked password
quillink note delete <id>              # prompts for the password first if the note is locked

quillink tag list --counts             # every tag with its note/canvas counts
quillink tag show work urgent          # notes carrying every one of these tags (--match any: at least one)
quillink note list --tags work,urgent  # same filter on note list; note search takes --tags/--match too

quillink task list --open              # open tasks; --from/--to YYYY-MM-DD for a date range (max 92 days)
quillink task due                      # due today or overdue
quillink task create "Send report" --due 2026-11-07          # all day; "2026-11-07 14:30" for a time
quillink task update <id> --due 2026-11-10 | --clear-due
quillink task done <id>                # task reopen <id> to undo; --date for one occurrence of a repeating task
quillink task delete <id>              # moves it to the trash; task restore <id> brings it back
quillink task category list

quillink canvas list                   # --folder <id>, --tag <tag>, --trashed
quillink canvas get <id>               # title, state and what it contains
quillink canvas export <id> --out board.png   # or .jpg; --scale 1, --dark, --background "#ffffff"; --out - for stdout

quillink org info                      # Team plan: your organization's plan/seats/status
quillink org members                   # Team plan: your organization's members

quillink logout
```

### Interactive session

Run `quillink` with no arguments in a terminal to open a session: a short banner with suggested commands (version and build pinned top right), then an input box that stays at the bottom of the window with a status line under it (workspace, account and plan, what's unlocked). Type commands without the `quillink` prefix (`note list`, `note get <id>`, `help`, `exit`). What you type is coloured as you go: commands yellow, flags green, arguments cyan, and a command that doesn't exist red. A workspace other than production shows in yellow on the status line. The session takes the whole window, like `less` or `vim`: output goes into a pane above the box that scrolls with the mouse wheel or PgUp/PgDn (Ctrl+L clears it), and leaving the session returns your terminal as it was. While the session owns the mouse, select text with Shift-drag (Option-drag in macOS Terminal and iTerm2).

- While you type, a list of matching commands and flags appears under the box: ↑/↓ to pick, Tab to accept, Esc to hide. Without the list, ↑/↓ recall earlier commands. History is kept in memory only, and lines that pass `--token` are never recorded.
- A running command shows a spinner with its elapsed time, or a progress bar when the total is known. Ctrl-C twice leaves the session (a running command is stopped with it).
- Passwords and keys are asked for in the box itself, masked, and never enter history. `--*-stdin` flags don't work inside the session; leave them off to be asked.
- A Zero-Knowledge account is unlocked once per session instead of per command. The key stays in memory only and is wiped after 15 minutes without use, on `lock`, and on `exit` / Ctrl-D.
- `clear` (or Ctrl-L) clears the screen. Ctrl-C during the banner animation skips it.
- Scripts are unaffected: when stdin or stdout isn't a terminal, or with `--json`, bare `quillink` prints help as before.
- `NO_COLOR` turns colour off everywhere; `QUILLINK_NO_ANIMATION=1` shows the banner without the animation; `QUILLINK_THEME=light` or `dark` overrides the detected terminal background. Help text is coloured on a terminal and plain when piped.

### On a terminal vs. in a script

On a terminal, results are laid out for reading: tables fit the window (long titles are shortened with "…", dates read "3h ago"), pinned/locked/Vault notes and task status are marked, overdue tasks show in red, `note get` renders the note's Markdown, `whoami`, `org info` and `note stats` are small cards, and successes and errors start with ✓ / ✗ (errors with a hint). Slow steps show a spinner or progress bar on stderr. When output is piped or redirected, or with `--json`/`--output`, everything is the plain text it always was, byte for byte.

### Output formats and saving to a file

- `--output table|json|csv|yaml|md` on every command (default `table`; `--json` is the same as `--output json`). CSV and Markdown turn list results into one row per item.
- `--out <file>` saves the result instead of printing it. Without `--output`, the format comes from the extension (`.csv`, `.json`, `.yaml`, `.md`; anything else is a table). Files are readable only by you, an existing file is never overwritten without `--force`, and a failed command leaves no file behind.

```sh
quillink note list --output csv --out notes.csv
quillink tag list --counts --out tags.md
quillink note get <id> --output yaml
```

Every command supports `--json` for scripting (stable, independently versioned output schema — see `internal/output`), and non-zero exit codes for common failure cases (`internal/output/exitcodes.go`): 2 = auth failure, 3 = not found, 4 = rate limited, 5 = invalid input.

### Auth for scripts/CI

```sh
export QUILLINK_TOKEN=qlk_pat_...      # from the Tokens tab on the Developer page
quillink note list --json
```

`--token` is also accepted per-invocation. Never pass a note's unlock password as a literal flag — use an interactive terminal, `--password-stdin`, or `QUILLINK_NOTE_PASSWORD`.

### Password-protected notes

```sh
quillink note lock <id>       # New password: / Confirm password: (interactive)
quillink note unlock <id>     # Password: (interactive) -- prints the decrypted content
quillink note delete <id>     # Password: (interactive) -- refuses to delete without it
```

`lock`/`unlock` use the exact same client-side AES-256-GCM + PBKDF2-SHA256 encryption as the web/mobile apps (`internal/notecrypto`, ported from `vaultCrypto.ts`) — the backend only ever receives an opaque ciphertext blob and a separately-derived verifier hash, never the password or plaintext body. A note locked from any client can be unlocked from any other. `note delete` on a locked note requires the password too: knowing you own a note isn't treated as enough to delete content you haven't proven you can actually read.

A locked note's attachments decrypt the same way: `note attachment download <note-id> <attachment-id> --out file` asks for the note's password (or `--password-stdin`).

### Vault

```sh
quillink vault unlock                 # Vault master password (interactive, or --vault-password-stdin)
quillink vault list                   # notes and canvases, decrypted here
quillink vault get <id>
quillink vault export <id> --out note.md      # a canvas: --out board.png
```

The server checks only a verifier derived from the master password and returns the wrapped Vault key; the key is unwrapped and used on this device (`/v1/vault/*`, scope `vault:read`, read-only). In the interactive session the key stays in memory until 15 minutes idle, `vault lock` / `lock`, or exit; a one-off command asks each time. Vault notes' attachments download decrypted too. The password is never accepted as a flag value or env var.

### Zero-Knowledge accounts

Everything that reads content decrypts on this device with the account key (Recovery Passphrase + Secret Key, asked once per session). Where the server can't help because it only holds ciphertext, the CLI reads the account's notes and works it out locally: `tag list`/`tag show`, `note list --tags`, `note export` (same Markdown/NDJSON as the server's export, via `internal/notemd`, a mirror of the backend's `note_export.py`), graph/backlink titles and `note related` (the server's scoring, mirrored). Those scans stop at 5,000 notes and say so. Writes are encrypted here too: `note create`/`update`, `task create`/`update` and `folder create`/`update` seal titles, content, tags and names (envelope v2, the web's format) before sending, and the server refuses plain text from a Zero-Knowledge account. Changing only some fields of an item needs the existing ones to decrypt on this device; otherwise the CLI refuses rather than mixing keys. Zero-Knowledge tasks can't have due dates or categories. `quillink login` and write tokens work for Zero-Knowledge accounts; `note search` stays unavailable.

`login` and `workspace login` open the sign-in page in your browser with the code filled in. They don't over SSH or without a desktop session, and `--no-browser` or `QUILLINK_NO_BROWSER=1` turns it off; the link and code are always printed too.

### Pointing at a different environment

```sh
export QUILLINK_API_BASE=http://localhost:8000   # local backend, no Firebase Hosting rewrite
```

Defaults to `https://note-taking-app-prod.web.app`. The API base must be `https://`; plain `http://` is accepted only for `localhost` / `127.0.0.1` (the CLI refuses to send a token, or receive one at login, in clear text). Release binaries are built with the **prod** OAuth `client_id` baked in (see `.goreleaser.yaml`), so `quillink login` against a non-prod `--api-base` also needs a matching `--client-id` (or `QUILLINK_CLIENT_ID`) registered in that environment.

### Workspaces (multiple environments/accounts)

`--api-base`/`--client-id` above are per-invocation overrides for a single command. To switch between several environments or accounts *repeatedly* without retyping URLs every time, define named workspaces instead:

```sh
quillink workspace add staging --api-base https://staging.example.com
quillink workspace login staging          # OAuth device grant, stored under "staging"

quillink workspace add work-account --api-base https://note-taking-app-prod.web.app
quillink workspace login work-account     # a second, independent login against the SAME prod environment

quillink workspace use staging            # sets the default for future commands
quillink note list                        # runs against staging
quillink note list --workspace work-account   # one-off override, doesn't change the default

quillink workspace list                   # name, api_base, current marker, logged-in status
quillink workspace remove work-account
```

`workspace login` runs the OAuth device grant by default, which needs a `client_id` registered against that environment (see below) -- for an environment without one yet (e.g. a fresh local backend), attach a Personal Access Token directly instead and it skips OAuth entirely:

```sh
quillink workspace add local --api-base http://localhost:8000
quillink workspace login local --token qlk_pat_...
# or: QUILLINK_TOKEN=qlk_pat_... quillink workspace login local
```

Resolution order for every command: `--token`/`--api-base` (or `$QUILLINK_TOKEN`/`$QUILLINK_API_BASE`) → `--workspace`/`$QUILLINK_WORKSPACE` (or the workspace set via `workspace use`) → the original single global login (unchanged for anyone who never touches `workspace`). Workspace metadata (no secrets) lives in `~/.config/quillink/workspaces.json`; each workspace's actual credential is a separate OS keyring entry (or file fallback), so logging into one never touches another's — and this same registry file is shared with `gcp-note-taking-mcp`, so a workspace defined here is immediately usable there via `QUILLINK_WORKSPACE`.

## Architecture

```
main.go              entrypoint
cmd/                 Cobra command tree (root, login, logout, whoami, note *, workspace *)
internal/client/      /v1 + /api/oauth HTTP client, RFC 7807 error handling, wire types
internal/auth/        credential storage (OS keychain + file fallback), device-grant flow
internal/workspace/    named api-base/client_id registry (~/.config/quillink/workspaces.json)
internal/output/      --json envelope + table rendering, exit codes
internal/canvasrender/ draws a canvas to PNG/JPEG (Canvas v1 contract + renderer mirror)
```

`canvas export` renders on this device, like the web's own PNG export (same framing, 2 px per unit, 8192 px cap), so a Zero-Knowledge canvas is decrypted here and the server does no rendering. `internal/canvasrender/contract.go` mirrors the web's `contract.ts` and is tested against an identical copy of the shared fixtures (`testdata/canvas_contract_fixtures.json`) — update both when the contract changes. Note/task cards show titles only (the web's "Show content" previews aren't drawn), text uses Lexend (SIL OFL 1.1, `internal/canvasrender/fonts/LEXEND_OFL.txt`) and Go Mono, and serif text falls back to Lexend; scripts neither font covers (e.g. CJK, Arabic) can't be drawn yet. Locked canvases can't be exported from the CLI; Vault canvases export with `vault export`.

`internal/client/types.go` mirrors `app/models/note.py` / `app/models/folder.py` / `app/models/organization.py` / `app/models/api_access.py` in `gcp-note-taking-backend` field-for-field (snake_case JSON tags) — keep them in sync if those models change.

`org` is read-only: no `org invite`/`org remove`/`org set-role`/etc, since `/v1` has no `organizations:write` scope yet — those admin actions stay web-only for now (see the backend's `app/routers/v1_organizations.py` docstring).

## Known gaps vs. the CLI Access spec

- **`--include-sensitive`**: the flag exists on `note list`/`get`/`search` but is currently a no-op — the `/v1` API has no `include_sensitive` parameter yet (checked against the current backend; only the browser-facing API has any sensitive-field handling). Wire this up once the backend adds it.
- **`note delete`**: required adding `DELETE /v1/notes/{id}` to the backend (soft-delete, mirroring `v1_folders.trash_folder`) since it didn't exist — see `gcp-note-taking-backend` commit history.
- **OAuth client registration**: `quillink login` needs a real `client_id` registered per environment via the admin-only `POST /api/admin/oauth-clients` endpoint. `internal/client/oauth_client.go` ships a local-dev placeholder; release builds inject the real prod one via `-ldflags` (see `.github/workflows/release.yml`, secret `QUILLINK_CLI_OAUTH_CLIENT_ID`). Registered on both **staging** (`tkvd27Xu6-EZBhmiV6d_Sw`) and **prod** (`f-140e7BhV1gFg0eq6nQYg`).

## Distribution

- [x] GoReleaser config (`.goreleaser.yaml`) + release workflow, triggered on `v*` tags.
- [x] Register OAuth clients on staging and prod.
- [x] Ship the API Access/Developer/CLI feature to prod (`main`→`prod` PRs merged on `gcp-note-taking-infra`, `gcp-note-taking-backend`, `gcp-note-taking-frontend`).
- [x] Set `QUILLINK_CLI_OAUTH_CLIENT_ID` as a repo secret.
- [x] Cut real release tags (`v0.1.0`, `v0.1.1`) — binaries live on [GitHub Releases](https://github.com/dagistankaradeniz/gcp-note-taking-cli/releases).
- [x] Shell completions (bash/zsh/fish/powershell) — Cobra generates these for free via `quillink completion <shell>`, no extra work needed.
- [x] `homebrew_casks:` block wired to [`dagistankaradeniz/homebrew-quillink`](https://github.com/dagistankaradeniz/homebrew-quillink) (`brew install --cask dagistankaradeniz/quillink/quillink`).
- [ ] Set `HOMEBREW_TAP_GITHUB_TOKEN` as a repo secret (cross-repo push access to the tap) — the default `GITHUB_TOKEN` can't push to a different repo. Blocks the first cask publish.
- [x] Signed build provenance (GitHub artifact attestations, SLSA) for every release archive and `checksums.txt` — verify a download with `gh attestation verify <file> --repo dagistankaradeniz/gcp-note-taking-cli`.
- [ ] `winget` manifest submission to `microsoft/winget-pkgs` — manual PR process, not automatable from CI.

## Local credential storage

macOS Keychain / Linux Secret Service (`libsecret`) / Windows Credential Manager via [`zalando/go-keyring`](https://github.com/zalando/go-keyring), falling back to `~/.config/quillink/credential` (`chmod 600`) when no OS credential store is available (e.g. headless Linux CI).

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
quillink login                         # device-grant browser login
quillink whoami

quillink note create "Title" --content "Body text"
quillink note list
quillink note get <id>
quillink note update <id> --title "New title" --pinned
quillink note search "query"
quillink note unlock <id>              # prompts for a masked password
quillink note delete <id>

quillink logout
```

Every command supports `--json` for scripting (stable, independently versioned output schema — see `internal/output`), and non-zero exit codes for common failure cases (`internal/output/exitcodes.go`): 2 = auth failure, 3 = not found, 4 = rate limited, 5 = invalid input.

### Auth for scripts/CI

```sh
export QUILLINK_TOKEN=qlk_pat_...      # from the Tokens tab on the Developer page
quillink note list --json
```

`--token` is also accepted per-invocation. Never pass a note's unlock password as a literal flag — use an interactive terminal, `--password-stdin`, or `QUILLINK_NOTE_PASSWORD`.

### Pointing at a different environment

```sh
export QUILLINK_API_BASE=http://localhost:8000   # local backend, no Firebase Hosting rewrite
```

Defaults to `https://note-taking-app-prod.web.app`. Release binaries are built with the **prod** OAuth `client_id` baked in (see `.goreleaser.yaml`), so `quillink login` against a non-prod `--api-base` also needs a matching `--client-id` (or `QUILLINK_CLIENT_ID`) registered in that environment.

## Architecture

```
main.go              entrypoint
cmd/                 Cobra command tree (root, login, logout, whoami, note *)
internal/client/      /v1 + /api/oauth HTTP client, RFC 7807 error handling, wire types
internal/auth/        credential storage (OS keychain + file fallback), device-grant flow
internal/output/      --json envelope + table rendering, exit codes
```

`internal/client/types.go` mirrors `app/models/note.py` / `app/models/folder.py` / `app/models/api_access.py` in `gcp-note-taking-backend` field-for-field (snake_case JSON tags) — keep them in sync if those models change.

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
- [x] `brews:` block wired to [`dagistankaradeniz/homebrew-quillink`](https://github.com/dagistankaradeniz/homebrew-quillink) (`brew install dagistankaradeniz/quillink/quillink`).
- [ ] Set `HOMEBREW_TAP_GITHUB_TOKEN` as a repo secret (cross-repo push access to the tap) — the default `GITHUB_TOKEN` can't push to a different repo. Blocks the first formula publish.
- [ ] `winget` manifest submission to `microsoft/winget-pkgs` — manual PR process, not automatable from CI.

## Local credential storage

macOS Keychain / Linux Secret Service (`libsecret`) / Windows Credential Manager via [`zalando/go-keyring`](https://github.com/zalando/go-keyring), falling back to `~/.config/quillink/credential` (`chmod 600`) when no OS credential store is available (e.g. headless Linux CI).

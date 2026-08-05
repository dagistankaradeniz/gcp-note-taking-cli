# Quillink CLI

Command-line client for [Quillink](https://github.com/dagistankaradeniz/gcp-note-taking-frontend), built on the same `/v1` API and authentication as the [API Access](https://dagistan.atlassian.net/wiki/spaces/GNTA/pages/34013185) layer. See the [CLI Access](https://dagistan.atlassian.net/wiki/spaces/GNTA/pages/34045953) Confluence page (GNTA space) for the full design and decisions log.

**Pro-only**, same gating as the API.

## Status

v1 command surface (auth + note CRUD) is implemented, smoke-tested against a local backend, and has a **registered OAuth client on staging**. **Prod has none of this yet** — `gcp-note-taking-backend`/`gcp-note-taking-frontend` only ever auto-deployed to staging this whole feature never went through a `main`→`prod` PR, so `https://note-taking-app-prod.web.app` (the CLI's default `--api-base`) currently 404s on `/v1/*` and has no OAuth client registered. Not yet released — no tagged binaries, Homebrew tap, or winget manifest exist yet (see "Distribution" below). Cutting a release is on hold until prod is promoted, since a binary built against the default `--api-base` wouldn't work out of the box today.

**To try it today**, point at staging:

```sh
export QUILLINK_API_BASE=https://note-taking-app-staging.web.app
export QUILLINK_CLIENT_ID=tkvd27Xu6-EZBhmiV6d_Sw   # "Quillink CLI" client registered on staging
quillink login
```

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
- **OAuth client registration**: `quillink login` needs a real `client_id` registered per environment via the admin-only `POST /api/admin/oauth-clients` endpoint. `internal/client/oauth_client.go` ships a local-dev placeholder; release builds inject the real one via `-ldflags` (see `.github/workflows/release.yml`). Registered on **staging** (`tkvd27Xu6-EZBhmiV6d_Sw`); **prod has no client registered yet** since prod doesn't run this backend feature at all (see "Status").

## Distribution (not yet done)

- [x] GoReleaser config (`.goreleaser.yaml`) + release workflow, triggered on `v*` tags.
- [x] Register an OAuth client on staging (`tkvd27Xu6-EZBhmiV6d_Sw`) for manual/`--api-base` testing.
- [ ] Ship the API Access/Developer/CLI feature to prod (`main`→`prod` PRs on `gcp-note-taking-backend` and `gcp-note-taking-frontend`) — blocks everything below.
- [ ] Register a prod OAuth client and set `QUILLINK_CLI_OAUTH_CLIENT_ID` as a repo secret (`.github/workflows/release.yml` already reads it).
- [ ] Cut a `v0.1.0` tag once prod is ready, to publish real binaries.
- [ ] Homebrew tap (`homebrew-tap` repo + formula) — GoReleaser can publish to it once created.
- [ ] `winget` manifest submission to `microsoft/winget-pkgs` — manual PR process, not automatable from CI.
- [ ] Shell completions (bash/zsh/fish) — Cobra generates these for free via `quillink completion <shell>`; wire into packaging once binaries ship.

## Local credential storage

macOS Keychain / Linux Secret Service (`libsecret`) / Windows Credential Manager via [`zalando/go-keyring`](https://github.com/zalando/go-keyring), falling back to `~/.config/quillink/credential` (`chmod 600`) when no OS credential store is available (e.g. headless Linux CI).

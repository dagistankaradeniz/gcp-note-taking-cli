# AGENTS.md — gcp-note-taking-cli

Instructions for **any** agentic coding tool (Claude Code, OpenCode, Codex, Cursor, Gemini CLI, …).
This is the **canonical** conventions file for this repo. `CLAUDE.md` imports it. Read it fully and follow it before making changes.

**What this repo is:** the Quillink CLI — a Go/Cobra command-line client for the `/v1` API defined in `gcp-note-taking-backend`. Ships as a single, dependency-free binary per platform (Linux/macOS/Windows). Contains no server or infra code; it's a client of the API Access layer, same as the web frontend.

**Source of truth:** Confluence space **GNTA**, "CLI Access" page (client-specific decisions) and "API Access" page (shared auth/rate-limiting/versioning, read that one first). If code and docs diverge, stop and flag it rather than guessing.

---

## Stack

- Go **1.26+**, [Cobra](https://cobra.dev/) for the command tree, [`zalando/go-keyring`](https://github.com/zalando/go-keyring) for credential storage, `golang.org/x/term` for masked password input.
- [GoReleaser](https://goreleaser.com/) for cross-compiling + GitHub Releases.

## Repository layout

```
main.go               entrypoint
cmd/                  Cobra commands (root, login, logout, whoami, note *)
internal/client/       /v1 + /api/oauth HTTP client, wire types (mirrors backend Pydantic models)
internal/auth/         credential storage (keychain + file fallback), device-grant flow
internal/output/       --json envelope + table rendering, exit codes
.goreleaser.yaml       release build/packaging config
```

## Style

- `gofmt` clean, `go vet ./...` clean, `go test ./...` green — all CI gates.
- Keep `internal/client/types.go` in lockstep with `gcp-note-taking-backend`'s `app/models/*.py` — snake_case JSON tags, field-for-field. If a backend model changes, update this file in the same PR (or immediately after) and note the drift risk in the commit message.
- Commands stay thin (`cmd/`); HTTP/parsing logic lives in `internal/`.
- Every command that returns data supports `--json` (stable, versioned schema — bump `output.SchemaVersion` deliberately on a breaking change, never silently).
- Never accept a secret (token, note-unlock password) as a literal flag value — env var, `--*-stdin`, or an interactive masked prompt only. See `internal/auth/store.go` and `cmd/note_unlock.go` for the existing pattern.

## Domain conventions (must stay consistent)

- All `/v1` requests carry a bearer credential (PAT or OAuth device-grant token) — never a Firebase ID token. See `gcp-note-taking-backend` AGENTS.md for how these are verified server-side.
- `/v1` errors are RFC 7807 Problem Details; `/api/oauth/*` errors are the older `{"detail": ...}` shape. `internal/client/client.go`'s `Do` vs `PostPublic` split reflects this — don't merge them without preserving both parsers.
- Exit codes are part of the contract for scripts (`internal/output/exitcodes.go`) — don't introduce a new failure path without mapping it to one of the existing codes or adding a new one deliberately.

## CI gates

- `gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green, cross-compile matrix (linux/darwin amd64+arm64, windows/amd64) builds clean.

## Before you open a PR

- `gofmt -l . && go vet ./... && go build ./...` all pass.
- If you touched `internal/client/types.go`, confirm it still matches the backend's current response shape (check `gcp-note-taking-backend/app/models/`).
- `go test ./...` passes. Unit tests cover pure logic (`internal/output`, `cmd`'s body-text helpers) -- there's no mocked HTTP layer, so also smoke-test the affected command against a running local backend (`docker-compose.yml` at the repo root of `gcp-note-taking-app`) before committing.

---

## Branching Strategy (identical across infra, backend, frontend & cli)

`main` is always deployable/tag-able. Never push work-in-progress to `main`.

### Branches
- Always branch from the latest `main`.
- One short-lived branch per unit of work, named `type/short-kebab-slug`:
  - `feat/…` new feature · `fix/…` bug fix · `chore/…` tooling/deps/config
  - `docs/…` docs only · `refactor/…` behavior-preserving · `test/…` tests only · `ci/…` pipeline · `perf/…` performance
  - e.g. `feat/note-search`, `fix/device-grant-slow-down`.

### Commits — Conventional Commits
- Format: `type(optional-scope): summary` — imperative, ≤ 72-char summary.
- Types: `feat, fix, chore, docs, refactor, test, ci, perf, build`.
- One logical change per commit; don't mix unrelated work.

### Pull Requests
- Open a PR into `main`; keep it focused and reviewable.
- **CI must be green before merge.** Never merge red CI.
- Prefer **squash merge**; the squash title follows Conventional Commits.

### Releases
- Tag `vX.Y.Z` on `main` to trigger `.github/workflows/release.yml` (GoReleaser → GitHub Releases). The CLI's version is independent from the API's own v1/v2+ versioning (see API Access page) and from the `--json` output schema version.

### Never commit
Secrets, `.env`, OAuth client secrets, API tokens, real credential-store contents.

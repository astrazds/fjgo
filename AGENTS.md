# AGENTS.md

Guidance for agents working in this repository.

## Project Shape

`fjgo` is a small Go CLI, installable Codex skill, and API client for the Forgejo API.

Keep the project boring:

- stdlib first
- no CLI framework until `flag` is clearly painful
- keep generated API code small, deterministic, and stdlib-only
- add hand-written command aliases only when they remove real repetition

## Commands

Run these before handing work back:

```sh
./scripts/verify.sh
```

Useful smoke check:

```sh
./fjgo --version
./fjgo skill install --dir "$(mktemp -d)/fjgo"
./fjgo doctor kavemand/.forgejo --json
./fjgo version
./fjgo api inspect createCurrentUserRepo
./fjgo api inspect repoSearch
./fjgo alias inspect repo pulls get
./fjgo api --json inspect repoSearch
./fjgo release upload kavemand/.forgejo 1 ./go.mod --yes --dry-run
./fjgo auth status
```

Authenticated commands use:

```sh
export FJGO_BASE_URL=https://forgejo.example.com/api/v1
export FJGO_TOKEN=...
```

The API base defaults to `https://v15.next.forgejo.org/api/v1`; override with
`FJGO_BASE_URL` or `-base-url`.
Ambient `FJGO_TOKEN` is ignored for the built-in public demo default unless the
base URL is explicitly configured.

## Code Layout

- `cmd/fjgo`: CLI parsing and command dispatch
- `internal/forgejo`: HTTP client, generated API methods, generated models
- `internal/fjgoskill`: embedded Codex skill and installer helper
- `tools/genapi`: stdlib Swagger generator for `endpoints_gen.go` and
  `models_gen.go`
- `scripts/generate.sh`: pinned/configurable Swagger generation wrapper
- `scripts/release.sh`: cross-platform archive builder
- `scripts/smoke-auth.sh`: optional authenticated smoke against a disposable repo
- `scripts/verify.sh`: full local verification gate
- `.forgejo/workflows/verify.yml`: push/PR verification and tag release archive
  upload
- `swagger.v1.json`: pinned Swagger input for reproducible generation

Keep command-specific parsing in `cmd/fjgo`. Keep HTTP details, multipart
upload helpers, and JSON types in `internal/forgejo`. The generic
`fjgo api list/inspect/call/upload` commands are the full-coverage CLI surface;
add nicer aliases only when they remove real repetition.

## Implementation Rules

- Preserve context-aware HTTP calls.
- Keep request timeouts.
- Keep token auth as `Authorization: token <token>` unless Forgejo changes.
- Return useful API errors with status code and response body.
- Add one focused test for new non-trivial client behavior.
- Keep generated methods typed: path parameters as strings, body schemas as
  generated model parameters, query data through `RequestOptions`, documented
  success responses as return values.
- Keep generated `Operation` metadata useful for agents: path params, query
  params, form params, upload status, body type, return type, and summaries.
- Keep `--yes` on mutating commands, and keep `--dry-run` / `--print-request`
  token-safe.
- Keep JSON output for inspect/list surfaces deterministic and parseable.
- Keep `doctor --json` redacted: token presence is OK, token values and private
  user fields are not.
- Keep `-R` / `--repo-from-remote` scoped to parsing Forgejo git remotes; do not
  guess repo context from unrelated files.
- Keep the embedded skill concise, with workflow detail in `references/`.
- Prefer a raw escape hatch like `get` over prematurely wrapping the whole API.
- Regenerate API methods with `go generate ./internal/forgejo`; do not edit
  `endpoints_gen.go` or `models_gen.go` by hand.
- Use `SPEC=/path/to/swagger.v1.json go generate ./internal/forgejo` when
  intentionally generating from a non-pinned spec.
- Keep `scripts/verify.sh` aligned with the live Swagger counts when the
  upstream API changes.

## v1 Scope

The v1 surface is:

- `version`, `me`, and raw `get`
- embedded skill install via `skill install` / `install --skills`
- redacted field diagnostics via `doctor --json`
- `auth status` / `whoami` token-safe diagnostics
- useful aliases: `repo get`, `repo topics`, `repo avatar`, `repo issue close`,
  `repo issue comment`, `release list`, `release upload`
- generic `api list/inspect/call/upload` coverage for the Swagger operation
  surface, including multipart release/issue/comment attachment uploads
- generated `alias list/inspect/collisions` and `model inspect`
- `--json` for agent-parseable inspect/list surfaces
- `--dry-run` / `--print-request` for token-safe request previews
- `-R` / `--repo-from-remote` for owner/repo resolution from git remotes
- generated typed client methods and model types for the full Swagger surface
- optional authenticated smoke via `scripts/smoke-auth.sh`
- release archives via `scripts/release.sh`
- repeatable verification via `scripts/verify.sh`

Prefer the generic API surface unless a named alias clearly reduces repeated
real-world usage.

# AGENTS.md

Guidance for agents working in this repository.

## Project Shape

`fjgo` is a small Go CLI plus API client for the Forgejo API at
`https://repos.astrazds.net/api/swagger#/`.

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
./fjgo version
./fjgo api inspect createCurrentUserRepo
```

Authenticated commands use:

```sh
export FJGO_TOKEN=...
```

The API base defaults to `https://repos.astrazds.net/api/v1`; override with
`FJGO_BASE_URL` or `-base-url`.

## Code Layout

- `cmd/fjgo`: CLI parsing and command dispatch
- `internal/forgejo`: HTTP client, generated API methods, generated models
- `tools/genapi`: stdlib Swagger generator for `endpoints_gen.go` and
  `models_gen.go`
- `scripts/generate.sh`: pinned/configurable Swagger generation wrapper
- `scripts/release.sh`: cross-platform archive builder
- `scripts/verify.sh`: full local verification gate
- `.forgejo/workflows/verify.yml`: push/PR verification
- `.forgejo/workflows/release.yml`: tag release archive upload
- `swagger.v1.json`: pinned Swagger input for reproducible generation

Keep command-specific parsing in `cmd/fjgo`. Keep HTTP details and JSON types in
`internal/forgejo`. The generic `fjgo api list/inspect/call` commands are the
full-coverage CLI surface; add nicer aliases only when they remove real
repetition.

## Implementation Rules

- Preserve context-aware HTTP calls.
- Keep request timeouts.
- Keep token auth as `Authorization: token <token>` unless Forgejo changes.
- Return useful API errors with status code and response body.
- Add one focused test for new non-trivial client behavior.
- Keep generated methods typed: path parameters as strings, body schemas as
  generated model parameters, query data through `RequestOptions`, documented
  success responses as return values.
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
- useful aliases: `repo get`, `repo topics`, `release list`
- generic `api list/inspect/call` coverage for every Swagger operation
- generated typed client methods and model types for the full Swagger surface
- release archives via `scripts/release.sh`
- repeatable verification via `scripts/verify.sh`

Prefer the generic API surface unless a named alias clearly reduces repeated
real-world usage.

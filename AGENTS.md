# AGENTS.md

Guidance for agents working in this repository.

## Project Shape

`fjgo` is a small Go AXI-style CLI, optional agent session hook, installable
Codex skill, and API client for the Forgejo API.

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
./fjgo
./fjgo setup hooks --check
./fjgo skill install --dir "$(mktemp -d)/fjgo"
./fjgo doctor kavemand/.forgejo --json
./fjgo version
./fjgo api inspect createCurrentUserRepo
./fjgo api inspect repoSearch
./fjgo api raw GET /version
./fjgo alias inspect repo pulls get
./fjgo api --json inspect repoSearch
./fjgo issue list kavemand/.forgejo --state open
./fjgo pr list kavemand/.forgejo --state open
./fjgo run list kavemand/.forgejo
./fjgo workflow list kavemand/.forgejo
./fjgo search issues fjgo --repo kavemand/.forgejo
./fjgo label list kavemand/.forgejo
printf secret | ./fjgo secret set kavemand/.forgejo VERIFY_SECRET --yes --dry-run
./fjgo variable set kavemand/.forgejo VERIFY_MODE --body release --yes --dry-run
./fjgo --repo kavemand/.forgejo repo branches list
./fjgo --repo kavemand/.forgejo repo collaborators list
./fjgo --repo kavemand/.forgejo repo branch-protection list
./fjgo repo create fjgo-test --private --yes --dry-run
./fjgo release create kavemand/.forgejo v0.0.0-test --yes --dry-run
./fjgo release latest kavemand/.forgejo
./fjgo release assets list kavemand/.forgejo 1
./fjgo release upload kavemand/.forgejo 1 ./go.mod --yes --dry-run
./fjgo issue dependencies add kavemand/.forgejo 1 2 --yes --dry-run
./fjgo issue time add kavemand/.forgejo 1 --seconds 60 --yes --dry-run
./fjgo pr update kavemand/.forgejo 1 --style rebase --yes --dry-run
./fjgo skill status
./fjgo skill generate --check
./fjgo update --check
./fjgo auth status
```

Authenticated commands use:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=...
```

The API base defaults to `https://v15.next.forgejo.org/api/v1`; override with
`FJGO_HOST`, `--host`, or explicit `-base-url`. `FJGO_HOST` and `--host`
derive `https://host/api/v1`. Repo context can be supplied explicitly
with root or command-local `--repo OWNER/REPO`, `FJGO_REPO=OWNER/REPO`,
positional repo args on repo-scoped commands, or `-R` / `--repo-from-remote`
for Forgejo git remotes.
Ambient `FJGO_TOKEN` is ignored for the built-in public demo default unless the
base URL is explicitly configured.

## Code Layout

- `cmd/fjgo`: CLI parsing and command dispatch
- `cmd/fjgo/toon.go`: stdlib TOON output encoder and truncation helpers
- `cmd/fjgo/axi_shared.go`: shared AXI field/body/count/log helpers
- `cmd/fjgo/workflow_commands.go`: curated issue and pull request commands
- `cmd/fjgo/advanced_workflow_commands.go`: issue pins/dependencies/reactions/time and PR review/update/diff helpers
- `cmd/fjgo/repo_lifecycle_commands.go`: curated repo lifecycle wrappers
- `cmd/fjgo/release_commands.go`: expanded release wrappers
- `cmd/fjgo/action_commands.go`: curated Actions run and workflow commands
- `cmd/fjgo/search_config_commands.go`: search and label commands
- `cmd/fjgo/secrets_variables_commands.go`: repo Actions secret/variable commands
- `cmd/fjgo/hooks.go`: explicit AXI session hook setup/capture helpers
- `internal/forgejo`: HTTP client, generated API methods, generated models
- `internal/fjgoskill`: embedded Codex skill and installer helper
- `tools/genapi`: stdlib Swagger generator for `endpoints_gen.go` and
  `models_gen.go`
- `scripts/generate.sh`: pinned/configurable Swagger generation wrapper
- `scripts/release.sh`: cross-platform archive builder
- `scripts/smoke-auth.sh`: optional authenticated smoke against a temporary repo
- `scripts/verify.sh`: full local verification gate
- `docs/agent-setup-prompt.md`: copy/paste prompt for setting up a fresh coding
  agent with `fjgo`
- `.forgejo/workflows/verify.yml`: push/PR verification and tag release archive
  upload
- `CHANGELOG.md`: release notes for tagged versions
- `swagger.v1.json`: pinned Swagger input for reproducible generation

Keep command-specific parsing in `cmd/fjgo`. Keep HTTP details, multipart
upload helpers, and JSON types in `internal/forgejo`. The generic
`fjgo api list/inspect/call/upload/raw` commands are the full-coverage CLI
surface; add nicer aliases only when they remove real repetition.

## Implementation Rules

- Preserve context-aware HTTP calls.
- Do not add hand-written command code for endpoints or functions absent from
  the pinned Forgejo Swagger. If Forgejo does not expose a documented API
  operation, omit the curated command instead of adding an unsupported stub.
- Keep request timeouts.
- Keep remote response reads bounded.
- Keep token auth as `Authorization: token <token>` unless Forgejo changes.
- Return useful API errors with status code and response body.
- Add one focused test for new non-trivial client behavior.
- Keep generated methods typed: path parameters as strings, body schemas as
  generated model parameters, query data through `RequestOptions`, documented
  success responses as return values.
- Keep generated `Operation` metadata useful for agents: path params, query
  params, form params, upload status, body type, return type, and summaries.
- Keep stdout TOON by default. Convert to TOON at the output boundary; keep
  internal logic on JSON/typed Go values.
- Keep default list schemas compact and add `--fields` where agents need
  explicit expansion.
- Keep detail/raw outputs truncated by default with `--full` as the escape
  hatch when truncation occurs.
- Keep definitive empty states and contextual help hints in TOON output.
- Keep structured errors on stdout. Use exit code 2 for usage errors and 1 for
  other failures.
- Keep `--yes` on mutating commands, and keep `--dry-run` / `--print-request`
  token-safe.
- Keep explicit `--json` output for inspect/list/raw surfaces deterministic and
  parseable.
- Keep `doctor --json` redacted: token presence is OK, token values and private
  user fields are not.
- Keep API error diagnostics token-safe, including server-reflected token text.
- Keep root `--json` failures parseable on stdout for coding agents.
- Keep explicit repo context predictable: root or command-local `--repo` /
  `FJGO_REPO` win when set; `-R` / `--repo-from-remote` stay scoped to parsing
  Forgejo git remotes; do not guess repo context from unrelated files.
- Keep `FJGO_HOST` / `--host` as the environment and CLI convenience derivation
  for `https://host/api/v1`; use explicit `-base-url` for non-standard API paths.
- Keep `setup hooks` explicit, idempotent, path-repairing, and scoped to the
  managed Claude Code, Codex, and OpenCode hook/plugin files.
- Keep the embedded skill concise, generated from `internal/fjgoskill`, with
  workflow detail in `references/`.
- Prefer raw escape hatches like `get` and `api raw` over prematurely wrapping
  the whole API.
- Regenerate API methods with `go generate ./internal/forgejo`; do not edit
  `endpoints_gen.go` or `models_gen.go` by hand.
- Use `SPEC=/path/to/swagger.v1.json go generate ./internal/forgejo` when
  intentionally generating from a non-pinned spec.
- Keep remote Swagger fetches timed out and size-limited.
- Keep `scripts/verify.sh` aligned with the live Swagger counts when the
  upstream API changes.

## v1 Scope

The v1 surface is:

- `version`, `me`, raw `get`, and `api raw`
- AXI home view via `fjgo` / `fjgo -R origin`
- ambient hook setup via `setup hooks`
- embedded skill install via `skill install` / `install --skills`
- redacted field diagnostics via `doctor --json`
- structured stdout errors and root JSON errors via `--json`
- `auth status` / `whoami` token-safe diagnostics
- curated workflow commands: `issue`, `pr`, `run`, `workflow`, `search`,
  `repo`, `label`, `secret`, `variable`, and `release`
- curated repo lifecycle commands: list, create, get, edit, fork, branches,
  collaborators, branch protection, topics, and avatar
- expanded release commands: list, view, latest, create, edit, delete, upload,
  asset list, and asset delete
- advanced issue/PR commands for issue pins/dependencies/blocks/reactions/
  deadlines/tracked time and PR edit/close/reopen/comment/reviews/review
  requests/review comments/update/diff/patch
- workflow/run lifecycle commands for workflow view and run watch, backed by
  repository contents and Actions run operations
- generic `api list/inspect/call/upload/raw` coverage for the Swagger operation
  surface, including multipart release/issue/comment attachment uploads
- generated `alias list/inspect/collisions` and `model inspect`
- TOON defaults plus `--json` for agent-parseable inspect/list/raw surfaces
- `--dry-run` / `--print-request` for token-safe request previews
- root and command-local `--repo`; `FJGO_HOST` / `--host` base URL derivation
- `-R` / `--repo-from-remote` for owner/repo resolution from git remotes
- generated typed client methods and model types for the full Swagger surface
- optional authenticated smoke via `scripts/smoke-auth.sh`
- v1 field-validation packet via `docs/alpha.md`
- agent setup prompt via `docs/agent-setup-prompt.md`
- release archives via `scripts/release.sh`
- repeatable verification via `scripts/verify.sh`

Prefer the generic API surface unless a named alias clearly reduces repeated
real-world usage.

## Agent skills

### Self-improvement

When the `fjgo` CLI itself fails unexpectedly during repository work, do not
silently work around it. Capture a minimal reproducible command, diagnose far
enough to provide useful evidence, and create a Forgejo issue labelled
`needs-triage`. Continue the original task through a safe escape hatch when one
exists, and keep the bug fix separate unless the user asks to implement it.

### Issue tracker

Issues are tracked in this repository's Forgejo Issues using `fjgo`. See `docs/agents/issue-tracker.md`.

### Triage labels

The tracker uses the five default canonical triage labels. See `docs/agents/triage-labels.md`.

### Domain docs

Domain documentation uses a single-context layout. See `docs/agents/domain.md`.

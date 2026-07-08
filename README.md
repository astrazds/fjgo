# fjgo

Agent-first Go CLI, API client, optional session hook, and installable skill for
Forgejo repos. `fjgo` follows the [AXI](https://axi.md/) pattern: compact TOON
stdout by default, contextual next-step hints, structured stdout errors, no
interactive prompts, token-safe dry runs, and explicit opt-in ambient context.

The API root defaults to the public Forgejo demo at
`https://v15.next.forgejo.org/api/v1`; override it with
`FJGO_HOST`, `--host`, `FJGO_BASE_URL`, or `-base-url`. `FJGO_HOST` and
`--host` derive `https://host/api/v1`.

## Agent Setup

Build or install the binary, then choose either ambient hooks, the bundled
skill, or both:

```sh
go build ./cmd/fjgo
./fjgo setup hooks --check
./fjgo setup hooks
./fjgo skill install --force
```

`fjgo setup hooks` installs or repairs managed SessionStart hooks for Claude
Code and Codex, enables Codex hooks in `~/.codex/config.toml`, and installs an
OpenCode ambient-context plugin. The managed hook runs `fjgo -R origin`, so each
agent session starts with compact Forgejo repo context for the current
directory. Re-running the setup is idempotent and repairs moved executable
paths. The installable skill is the lower-overhead on-demand path for agents
that support skills or for projects where per-session hooks are not wanted.

For a fresh coding agent, paste
[`docs/agent-setup-prompt.md`](docs/agent-setup-prompt.md) into the agent while
it is inside a Forgejo-backed checkout. The prompt installs `fjgo`, refreshes
the skill, checks repo/auth context, and prints user-facing next commands.

Inside a Forgejo-backed checkout, start every agent workflow with:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
./fjgo -R origin
./fjgo -R origin doctor
./fjgo -R origin issue list --state open
./fjgo -R origin pr list
./fjgo -R origin run list
./fjgo -R origin issue create --title "Test" --body "Body" --dry-run --yes
```

When there is no checkout remote, or when the task should target a different
repo, pass explicit context instead:

```sh
export FJGO_REPO=owner/repo
./fjgo repo get
./fjgo --repo owner/repo issue list --state open
./fjgo issue list --repo owner/repo --state open
```

Default stdout is TOON. Use `--json` only on explicit JSON-capable surfaces when
a script needs raw JSON, for example `fjgo doctor --json` or
`fjgo api --json call getVersion`. `doctor`, `auth status`, `--dry-run`,
`--print-request`, and structured errors are token-safe. They report token
presence and redacted user context, never the token value. API error bodies are
also scrubbed before they are shown, so a server cannot reflect the configured
token back into diagnostics.

## Status

Current app version: `v0.15.0`.

`v0.15.0` is the AXI expansion release. It adds command-local repo/host
context, generated skill drift checks, ambient hook setup, richer issue/PR,
repo, release, search, label, secret, variable, Actions, and workflow aliases,
structured token-safe errors, TOON-first output, guarded self-update checks,
and the rule that curated commands must be backed by pinned Forgejo Swagger
operations.

See [`CHANGELOG.md`](CHANGELOG.md) for release notes.

`fjgo` covers the full live Forgejo Swagger surface:

- embedded Codex skill install via `fjgo skill install` or
  `fjgo install --skills`
- opt-in AXI session hooks via `fjgo setup hooks`
- content-first home dashboard via `fjgo` or `fjgo -R origin`
- TOON stdout by default with `--json` escape hatches on inspect/list/raw
  surfaces
- redacted agent diagnostics via `fjgo doctor --json`
- structured stdout errors with exit code 2 for usage errors
- `491` generated endpoint methods
- `244` generated model types
- `491` CLI operations via `api list`, `api inspect`, and `api call`
- `419` generated convenience aliases via `alias list` and `alias inspect`
- explicit repo context through root or command-local `--repo OWNER/REPO`,
  `FJGO_REPO`, positional repo args, or scoped Forgejo git remote parsing with
  `-R`
- host context through `FJGO_HOST` or root/command-local `--host`
- curated AXI workflow commands for issues, pull requests, Actions runs,
  workflow dispatch, search, labels, repo lifecycle, releases, repo secrets,
  and repo variables
- curated repo lifecycle surfaces for list/create/edit/fork, branches,
  collaborators, and branch protection
- expanded releases: view/latest/edit/delete, asset list/delete, uploads, and
  `--body-file`/`--notes-file` notes
- advanced issue and PR surfaces for pins, dependencies, reactions, deadlines,
  tracked time, PR edit/close/reopen/comment, review requests/comments, PR
  update, diff, and patch
- workflow/run lifecycle surfaces for workflow view and run watch, backed by
  repository contents and Actions run operations
- raw API escape hatch via `fjgo api raw <METHOD> <path>`
- generated embedded skill check via `fjgo skill generate --check`
- guarded update check via `fjgo update --check`
- visible generated alias collisions via `alias collisions`
- generated query/form parameter inspection for operations
- typed body parameters for operations with Swagger body schemas
- generated model field inspection via `model inspect`
- explicit JSON output for inspect/list/raw surfaces used by scripts
- multipart release/issue/comment attachment upload support through `api upload`
- typed return values for operations with documented success response schemas
- optional authenticated field smoke via `scripts/smoke-auth.sh`
- alpha field-test packet in `docs/alpha.md`
- copy/paste setup prompt in `docs/agent-setup-prompt.md`
- Forgejo Actions verification and tag-release workflows

Run the full local gate with:

```sh
./scripts/verify.sh
```

The verifier regenerates API code, formats, tests, builds, runs live smoke
checks, verifies docs and embedded skill surfaces, checks coverage counts,
builds a release archive, verifies its embedded version metadata, and removes
`dist/`.

## Install

From source:

```sh
go install repos.astrazds.net/astrazds/fjgo/cmd/fjgo@latest
```

From a release archive:

```sh
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.15.0/fjgo_v0.15.0_linux_amd64.tar.gz
tar -xzf fjgo_v0.15.0_linux_amd64.tar.gz
install -Dm755 fjgo_v0.15.0_linux_amd64/fjgo ~/.local/bin/fjgo
```

Check for a newer release without changing files:

```sh
fjgo update --check
fjgo update --dry-run
```

`fjgo update --yes` replaces the current executable with the matching release
archive for the local OS and architecture.

## Quick Start

```sh
go build ./cmd/fjgo

./fjgo --version
./fjgo
./fjgo setup hooks --check
./fjgo skill install
./fjgo version
./fjgo -R origin doctor
./fjgo get /version
./fjgo api list repo
./fjgo api inspect createCurrentUserRepo
./fjgo api inspect repoSearch
./fjgo api call repoGet owner=kavemand repo=.forgejo
./fjgo api raw GET /repos/kavemand/.forgejo
./fjgo api --json inspect repoSearch
./fjgo api --json call repoGet owner=kavemand repo=.forgejo
./fjgo alias list
./fjgo alias inspect repo issues get
./fjgo alias collisions
./fjgo model inspect CreateRepoOption
./fjgo --repo kavemand/.forgejo repo get
./fjgo repo get --repo kavemand/.forgejo --fields full_name,default_branch,open_issues
./fjgo repo get kavemand/.forgejo
./fjgo repo list --org kavemand --fields name,private,archived
./fjgo repo create demo --private --dry-run --yes
./fjgo --repo kavemand/.forgejo repo branches list
./fjgo --repo kavemand/.forgejo repo collaborators add USER --permission write --dry-run --yes
./fjgo --repo kavemand/.forgejo repo branch-protection create --name main --required-approvals 1 --dry-run --yes
./fjgo -R origin repo get
./fjgo repo topics kavemand/.forgejo
./fjgo repo avatar kavemand/.forgejo assets/icon.png --dry-run --yes
./fjgo issue list kavemand/.forgejo --state open
./fjgo issue view kavemand/.forgejo 1 --full
./fjgo issue create kavemand/.forgejo --title "Bug" --body-file issue.md --dry-run --yes
./fjgo --repo kavemand/.forgejo issue dependencies add 42 7 --dry-run --yes
./fjgo --repo kavemand/.forgejo issue reactions add 42 +1 --dry-run --yes
./fjgo --repo kavemand/.forgejo issue deadline set 42 2026-08-01 --dry-run --yes
./fjgo --repo kavemand/.forgejo issue time add 42 --seconds 900 --dry-run --yes
./fjgo pr list kavemand/.forgejo --state open
./fjgo pr view kavemand/.forgejo 1 --reviews
./fjgo pr close kavemand/.forgejo 1 --dry-run --yes
./fjgo pr checks kavemand/.forgejo 1
./fjgo --repo kavemand/.forgejo pr review-requests add 1 --reviewer USER --dry-run --yes
./fjgo --repo kavemand/.forgejo pr diff 1
./fjgo run list kavemand/.forgejo
./fjgo run watch kavemand/.forgejo 1 --timeout 2m
./fjgo workflow list kavemand/.forgejo
./fjgo workflow view kavemand/.forgejo verify.yml --full
./fjgo workflow run kavemand/.forgejo verify.yml --ref main --input smoke=true --dry-run --yes
./fjgo search issues "bug" --repo kavemand/.forgejo
./fjgo label list kavemand/.forgejo
echo -n "$DEPLOY_TOKEN" | ./fjgo secret set kavemand/.forgejo DEPLOY_TOKEN --dry-run --yes
./fjgo variable set kavemand/.forgejo BUILD_MODE --body release --dry-run --yes
./fjgo release list kavemand/.forgejo
./fjgo release view kavemand/.forgejo v1.0.0
./fjgo release latest kavemand/.forgejo
./fjgo release create kavemand/.forgejo v1.0.0 --body-file notes.md --dry-run --yes
./fjgo release create kavemand/.forgejo v1.0.0 --notes-file notes.md --dry-run --yes
./fjgo release edit kavemand/.forgejo 123 --prerelease false --dry-run --yes
./fjgo release assets list kavemand/.forgejo 123
./fjgo release upload kavemand/.forgejo 123 dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
./fjgo auth status
./fjgo skill generate --check
./fjgo update --check
```

Authentication uses Forgejo's token auth header:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

Set `FJGO_HOST` or `FJGO_BASE_URL` with `FJGO_TOKEN` for private or non-demo instances. Ambient
`FJGO_TOKEN` is ignored for the built-in public demo default unless the base URL
is explicitly configured, which avoids sending a private token to the demo by
accident.

Use a read-only token for authenticated reads such as `fjgo me`. Repo write
operations, including topic or avatar updates, need repository write access.
Release creation/upload needs release/package permission for the target repo,
depending on the Forgejo instance policy.

## CLI

`fjgo` with no arguments prints an AXI home view: executable path, one-line
description, optional repository context, compact open issue/pull samples when
`--repo`, `FJGO_REPO`, or `-R origin` supplies context, and next commands.

`fjgo setup hooks [--check]` installs or checks managed ambient context hooks
for Claude Code, Codex, and OpenCode. It is explicit, idempotent, and
directory-scoped through the hook command `fjgo -R origin`.

`fjgo doctor [owner/repo] [--json]` prints a redacted field-feedback bundle for
agents: binary version, runtime, executable path, base URL, token/auth status,
optional git remote context, repository summary, latest/recent releases, bundled
skill install status, and useful next commands.

`fjgo skill install [--dir path] [--force]` installs the bundled Codex skill.
The default target is `.agents/skills/fjgo`. Use `fjgo skill status [--dir path]`
or `fjgo install --skills --check` to check whether the installed skill matches
the bundled copy. `fjgo skill generate --check` verifies that the embedded
`SKILL.md` still matches the generated source of truth.
`fjgo install --skills` is the same installer for agents that look for an
install command.

Failures are structured on stdout by default. Use root `--json` before a
command only when a caller specifically needs JSON errors:

```sh
fjgo --json api call repoGet owner=missing
```

`fjgo version` calls the Forgejo server `/version` endpoint and prints TOON by
default.

Curated workflow commands are the preferred surface for common agent work:

```sh
fjgo -R origin issue list --state open --fields number,title,state,author
fjgo -R origin issue view 42 --comments --full
fjgo -R origin issue create --title "Bug" --body-file issue.md --dry-run --yes
fjgo -R origin issue close 42 --dry-run --yes
fjgo -R origin issue pin 42 --dry-run --yes
fjgo -R origin issue dependencies add 42 7 --dry-run --yes
fjgo -R origin issue reactions add 42 +1 --dry-run --yes
fjgo -R origin issue deadline set 42 2026-08-01 --dry-run --yes
fjgo -R origin issue time add 42 --seconds 900 --dry-run --yes

fjgo -R origin pr list --state open
fjgo -R origin pr view 12 --full
fjgo -R origin pr view 12 --reviews
fjgo -R origin pr edit 12 --title "Updated title" --dry-run --yes
fjgo -R origin pr close 12 --dry-run --yes
fjgo -R origin pr comment 12 --body "Review note." --dry-run --yes
fjgo -R origin pr files 12
fjgo -R origin pr commits 12
fjgo -R origin pr checks 12
fjgo -R origin pr reviews 12
fjgo -R origin pr review-requests add 12 --reviewer USER --dry-run --yes
fjgo -R origin pr diff 12
fjgo -R origin pr update 12 --style rebase --dry-run --yes
fjgo -R origin pr merge 12 --method squash --dry-run --yes

fjgo -R origin run list --status failure
fjgo -R origin run view 123 --log-failed
fjgo -R origin run watch 123 --timeout 2m
fjgo -R origin workflow list
fjgo -R origin workflow view verify.yml --full
fjgo -R origin workflow run verify.yml --ref main --input smoke=true --dry-run --yes

fjgo -R origin search issues "login" --state open
fjgo search repos "forgejo cli" --limit 20
fjgo -R origin label list
fjgo -R origin label create --name bug --color ff0000 --dry-run --yes
echo -n "$TOKEN" | fjgo -R origin secret set DEPLOY_TOKEN --dry-run --yes
fjgo -R origin variable set BUILD_MODE --body release --dry-run --yes
```

These commands keep default schemas small, validate `--fields`, include count
metadata when Forgejo exposes it, truncate long text with `--full` escape
hatches, and keep mutations behind `--yes` plus token-safe dry runs. Use the
generated `api` and `alias` surfaces when a Forgejo operation has not earned a
hand-written workflow command.

Curated commands are backed by operations in the pinned Forgejo Swagger. If a
GitHub-shaped workflow has no documented Forgejo API operation, fjgo leaves it
out of the command surface.

`fjgo api list [filter]` lists every generated Swagger operation:

```sh
fjgo api list release
```

`fjgo api inspect <operationId>` shows method, path, summary, path parameters,
query parameters, typed body model and fields, multipart form parameters, and
typed return model:

```sh
fjgo api inspect createCurrentUserRepo
fjgo api inspect repoSearch
fjgo api --json inspect repoSearch
```

`fjgo api call <operationId>` executes any operation by Swagger `operationId`.
Default output is TOON with long string fields truncated and a `--full` hint
when truncation happens. Use `fjgo api --json call ...` for raw JSON:

```sh
fjgo api call getVersion
fjgo api call repoSearch q=fjgo limit=10
fjgo api call repoGet owner=kavemand repo=.forgejo
fjgo api call createCurrentUserRepo --yes --dry-run -body '{"name":"demo","private":true}'
fjgo api call createCurrentUserRepo --yes -body '{"name":"demo","private":true}'
fjgo api raw GET /repos/OWNER/REPO
fjgo api raw PATCH /repos/OWNER/REPO --dry-run --yes -body '{"description":"updated"}'
```

For `api call`, `name=value` arguments matching path parameters fill the path;
the rest become query parameters. JSON bodies can be inline, `@file`, or `-`
for stdin. Operations with a documented body schema fail locally when `-body`
is omitted. Mutating operations require `--yes`. Use `--dry-run` or
`--print-request` to print the request without performing network I/O.
Use `fjgo api raw <METHOD> <path>` when an API path is easier to express
directly than by operation ID. Raw mutations still require `--yes`.

Multipart/form-data operations use `api upload`:

```sh
fjgo api upload repoCreateReleaseAttachment owner=OWNER repo=REPO id=123 name=fjgo.tar.gz attachment=@dist/fjgo.tar.gz --yes
fjgo api upload issueCreateIssueAttachment owner=OWNER repo=REPO index=7 attachment=@screenshot.png --yes
```

`fjgo alias list` shows generated convenience commands for clear Swagger path
shapes. `fjgo alias inspect <command...>` shows the mapped operation, required
positional args, method, path, query params, body fields, form fields, return
type, upload status, and whether `--yes` is required. Aliases use positional
path args plus `name=value` query args, with `-body` matching `api call`.
Generated aliases for mutating operations require `--yes`.
`fjgo alias collisions` lists generated aliases that were skipped because
another operation already claimed the same command.

Inspect/list commands support `--json` for scripts that need raw JSON:

```sh
fjgo api --json list repo
fjgo alias --json inspect repo issues create
fjgo alias --json collisions
fjgo model --json inspect CreateIssueOption
```

`fjgo model inspect <Model>` prints generated JSON fields and Go types for a
Swagger model:

```sh
fjgo model inspect CreateRepoOption
```

Common aliases:

```sh
fjgo repo get kavemand/.forgejo
fjgo --repo kavemand/.forgejo repo get
fjgo repo list --org kavemand
fjgo repo create demo --private --dry-run --yes
fjgo --repo kavemand/.forgejo repo edit --description "Forgejo CLI" --dry-run --yes
fjgo --repo kavemand/.forgejo repo fork --name fjgo-fork --dry-run --yes
fjgo --repo kavemand/.forgejo repo branches list
fjgo --repo kavemand/.forgejo repo collaborators list
fjgo --repo kavemand/.forgejo repo branch-protection list
fjgo repo topics kavemand/.forgejo
fjgo repo topics kavemand/.forgejo --set forgejo,go,cli --dry-run --yes
fjgo repo avatar kavemand/.forgejo assets/icon.png --dry-run --yes
fjgo release list kavemand/.forgejo
fjgo release view kavemand/.forgejo v1.0.0
fjgo release latest kavemand/.forgejo
fjgo release create kavemand/.forgejo v1.0.0 --body-file notes.md --dry-run --yes
fjgo release create kavemand/.forgejo v1.0.0 --notes-file notes.md --dry-run --yes
fjgo release edit kavemand/.forgejo 123 --prerelease false --dry-run --yes
fjgo release delete kavemand/.forgejo v1.0.0 --dry-run --yes
fjgo release assets list kavemand/.forgejo 123
fjgo release assets delete kavemand/.forgejo 123 456 --dry-run --yes
fjgo release upload kavemand/.forgejo 123 dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
```

When running inside a checkout, `-R <remote>` or `--repo-from-remote <remote>`
resolves `owner/repo` from common Forgejo HTTPS and SSH git remote forms:

```sh
fjgo -R origin repo get
fjgo -R origin repo issues list state=open
fjgo -R origin release list
fjgo -R origin release create v1.0.0 --body-file notes.md --dry-run --yes
fjgo -R origin release upload 123 dist/fjgo.tar.gz --yes
```

`fjgo auth status` prints the active base URL, whether a token is present, and
the authenticated user when the token works. It never prints the token value.
HTTP error text from the Forgejo server is still included when useful, but any
configured token value is replaced before the error reaches stdout diagnostics.

## Go Client

The generated API surface lives in:

- `internal/forgejo/endpoints_gen.go`: operations, paths, and operation lookup
- `internal/forgejo/models_gen.go`: Swagger `definitions` as Go types

Generated files are reproducible from the pinned `swagger.v1.json` file:

```sh
go generate ./internal/forgejo
```

Refresh from live Swagger explicitly:

```sh
curl -fsSL https://v15.next.forgejo.org/swagger.v1.json -o swagger.v1.json
go generate ./internal/forgejo
```

Or generate from another compatible spec:

```sh
SPEC=/path/to/swagger.v1.json go generate ./internal/forgejo
```

`SPEC` may also be an `http://` or `https://` URL. Remote spec fetches use a
30 second timeout and a 32 MiB response limit; normal verification uses the
pinned local `swagger.v1.json`.

Generated methods accept typed path parameters, typed body parameters when the
operation has a Swagger body schema, plus `forgejo.RequestOptions` for query
values. Methods with a documented success response return the generated
response type:

```go
repo, err := client.RepoGet(ctx, "kavemand", ".forgejo", forgejo.RequestOptions{})
created, err := client.CreateCurrentUserRepo(ctx, &forgejo.CreateRepoOption{
	Name:    "demo",
	Private: true,
}, forgejo.RequestOptions{})
```

Use `forgejo.RequestOptions.Query` for query parameters. Multipart operations
can be executed with `DoOperationMultipart`; the CLI uses that path for release,
issue, and comment attachment uploads.
Client response bodies are bounded to 32 MiB before decoding or reporting
errors, which keeps malicious or broken Forgejo-compatible servers from forcing
unbounded local memory use.

## Development

```sh
./scripts/verify.sh
```

Generated files are committed for consumers, but should only be edited through
`go generate ./internal/forgejo`.

Forgejo Actions runs the same verifier on pushes and pull requests via
`.forgejo/workflows/verify.yml`.

Optional authenticated field smoke against a disposable repo:

```sh
go build ./cmd/fjgo
FJGO_BASE_URL=https://forgejo.example.com/api/v1 FJGO_TEST_REPO=owner/repo FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

When `FJGO_BASE_URL`, `FJGO_TEST_REPO`, and `FJGO_TOKEN` are set,
`./scripts/verify.sh` runs the authenticated smoke too. The smoke creates a test
issue, comments on it, closes it, and exercises token-safe dry-run previews.

Alpha testers should use `docs/alpha.md` for the field-test checklist and
failure-report format.

## Release

Build release archives into `dist/`:

```sh
VERSION=v0.15.0 ./scripts/release.sh
```

Override targets when testing locally:

```sh
VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
```

The script embeds `version`, `commit`, and UTC build date into `fjgo --version`
and writes `dist/checksums.txt`.

Pushing a `v*` tag runs the verify workflow, then the release job builds
archives and uploads them to a Forgejo release using the Actions token.
Release notes live in [`CHANGELOG.md`](CHANGELOG.md).

Smoke check a published release archive:

```sh
VERSION=v0.15.0 ./scripts/smoke-release.sh
```

## License

MIT

# fjgo

Agent-first Go CLI, API client, and installable Codex skill for Forgejo repos.

The API root defaults to the public Forgejo demo at
`https://v15.next.forgejo.org/api/v1`; override it with
`FJGO_BASE_URL` or `-base-url`.

## Agent Setup

Build or install the binary, then install the bundled skill into the current
project:

```sh
go build ./cmd/fjgo
./fjgo skill install
```

Inside a Forgejo-backed checkout, start every agent workflow with:

```sh
export FJGO_BASE_URL=https://forgejo.example.com/api/v1
export FJGO_TOKEN=your_access_token
./fjgo -R origin doctor --json
./fjgo -R origin alias inspect repo issues create
./fjgo -R origin repo issues create --dry-run --yes -body '{"title":"Test","body":"Body"}'
```

`doctor --json`, `auth status`, `--dry-run`, and `--print-request` are
token-safe. They report whether a token is present, never the token value.

## Status

Current app version: `v0.11.0`.

`fjgo` covers the full live Forgejo Swagger surface:

- embedded Codex skill install via `fjgo skill install` or
  `fjgo install --skills`
- redacted agent diagnostics via `fjgo doctor --json`
- structured root error output via `fjgo --json ...`
- `491` generated endpoint methods
- `244` generated model types
- `491` CLI operations via `api list`, `api inspect`, and `api call`
- `419` generated convenience aliases via `alias list` and `alias inspect`
- visible generated alias collisions via `alias collisions`
- generated query/form parameter inspection for operations
- typed body parameters for operations with Swagger body schemas
- generated model field inspection via `model inspect`
- JSON output for inspect/list surfaces used by other agents
- multipart release/issue/comment attachment upload support through `api upload`
- typed return values for operations with documented success response schemas
- optional authenticated field smoke via `scripts/smoke-auth.sh`
- alpha field-test packet in `docs/alpha.md`
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
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.11.0/fjgo_v0.11.0_linux_amd64.tar.gz
tar -xzf fjgo_v0.11.0_linux_amd64.tar.gz
install -Dm755 fjgo_v0.11.0_linux_amd64/fjgo ~/.local/bin/fjgo
```

## Quick Start

```sh
go build ./cmd/fjgo

./fjgo --version
./fjgo skill install
./fjgo version
./fjgo -R origin doctor --json
./fjgo get /version
./fjgo api list repo
./fjgo api inspect createCurrentUserRepo
./fjgo api inspect repoSearch
./fjgo api call repoGet owner=kavemand repo=.forgejo
./fjgo api --json inspect repoSearch
./fjgo alias list
./fjgo alias inspect repo issues get
./fjgo alias collisions
./fjgo model inspect CreateRepoOption
./fjgo repo get kavemand/.forgejo
./fjgo -R origin repo get
./fjgo repo topics kavemand/.forgejo
./fjgo repo avatar kavemand/.forgejo assets/icon.png --dry-run --yes
./fjgo release list kavemand/.forgejo
./fjgo release upload kavemand/.forgejo 123 dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
./fjgo auth status
```

Authentication uses Forgejo's token auth header:

```sh
export FJGO_BASE_URL=https://forgejo.example.com/api/v1
export FJGO_TOKEN=your_access_token
```

Set `FJGO_BASE_URL` with `FJGO_TOKEN` for private or non-demo instances. Ambient
`FJGO_TOKEN` is ignored for the built-in public demo default unless the base URL
is explicitly configured, which avoids sending a private token to the demo by
accident.

Use a read-only token for authenticated reads such as `fjgo me`. Repo write
operations, including topic or avatar updates, need repository write access.
Release creation/upload needs release/package permission for the target repo,
depending on the Forgejo instance policy.

## CLI

`fjgo doctor [owner/repo] --json` prints a redacted field-feedback bundle for
agents: binary version, runtime, executable path, base URL, token/auth status,
optional git remote context, repository summary, latest/recent releases, bundled
skill install status, and useful next commands.

`fjgo skill install [--dir path] [--force]` installs the bundled Codex skill.
The default target is `.agents/skills/fjgo`. Use `fjgo skill status [--dir path]`
or `fjgo install --skills --check` to check whether the skill is installed.
`fjgo install --skills` is the same installer for agents that look for an
install command.

Use root `--json` before a command to make failures parseable:

```sh
fjgo --json api call repoGet owner=missing
```

`fjgo version` calls the Forgejo server `/version` endpoint.

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

`fjgo api call <operationId>` executes any operation by Swagger `operationId`:

```sh
fjgo api call getVersion
fjgo api call repoSearch q=fjgo limit=10
fjgo api call repoGet owner=kavemand repo=.forgejo
fjgo api call createCurrentUserRepo --yes --dry-run -body '{"name":"demo","private":true}'
fjgo api call createCurrentUserRepo --yes -body '{"name":"demo","private":true}'
```

For `api call`, `name=value` arguments matching path parameters fill the path;
the rest become query parameters. JSON bodies can be inline, `@file`, or `-`
for stdin. Operations with a documented body schema fail locally when `-body`
is omitted. Mutating operations require `--yes`. Use `--dry-run` or
`--print-request` to print the request without performing network I/O.

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

Inspect/list commands support `--json` for agent parsing:

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
fjgo repo topics kavemand/.forgejo
fjgo repo topics kavemand/.forgejo --set forgejo,go,cli --dry-run --yes
fjgo repo avatar kavemand/.forgejo assets/icon.png --dry-run --yes
fjgo release list kavemand/.forgejo
fjgo release create kavemand/.forgejo v1.0.0 body="Release notes." --dry-run --yes
fjgo release upload kavemand/.forgejo 123 dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
```

When running inside a checkout, `-R <remote>` or `--repo-from-remote <remote>`
resolves `owner/repo` from common Forgejo HTTPS and SSH git remote forms:

```sh
fjgo -R origin repo get
fjgo -R origin repo issues list state=open
fjgo -R origin release list
fjgo -R origin release create v1.0.0 body="Release notes." --dry-run --yes
fjgo -R origin release upload 123 dist/fjgo.tar.gz --yes
```

`fjgo auth status` prints the active base URL, whether a token is present, and
the authenticated user when the token works. It never prints the token value.

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
VERSION=v0.11.0 ./scripts/release.sh
```

Override targets when testing locally:

```sh
VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
```

The script embeds `version`, `commit`, and UTC build date into `fjgo --version`
and writes `dist/checksums.txt`.

Pushing a `v*` tag runs the verify workflow, then the release job builds
archives and uploads them to a Forgejo release using the Actions token.

Smoke check a published release archive:

```sh
VERSION=v0.11.0 ./scripts/smoke-release.sh
```

## License

MIT

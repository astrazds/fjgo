# fjgo

Small Go CLI and client for the Forgejo API at `https://repos.astrazds.net/api/swagger#/`.

The API root defaults to `https://repos.astrazds.net/api/v1`; override it with
`FJGO_BASE_URL` or `-base-url`.

## Status

Current app version: `v0.8.0`.

`fjgo` covers the full live Forgejo Swagger surface:

- `491` generated endpoint methods
- `244` generated model types
- `491` CLI operations via `api list`, `api inspect`, and `api call`
- `417` generated convenience aliases via `alias list` and `alias inspect`
- typed body parameters for operations with Swagger body schemas
- typed return values for operations with documented success response schemas
- Forgejo Actions verification and tag-release workflows

Run the full local gate with:

```sh
./scripts/verify.sh
```

The verifier regenerates API code, formats, tests, builds, runs live smoke
checks, checks coverage counts, builds a release archive, verifies its embedded
version metadata, and removes `dist/`.

## Install

From source:

```sh
go install repos.astrazds.net/astrazds/fjgo/cmd/fjgo@latest
```

From a release archive:

```sh
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.8.0/fjgo_v0.8.0_linux_amd64.tar.gz
tar -xzf fjgo_v0.8.0_linux_amd64.tar.gz
install -Dm755 fjgo_v0.8.0_linux_amd64/fjgo ~/.local/bin/fjgo
```

## Quick Start

```sh
go build ./cmd/fjgo

./fjgo --version
./fjgo version
FJGO_TOKEN=... ./fjgo me
./fjgo get /version
./fjgo api list repo
./fjgo api inspect createCurrentUserRepo
./fjgo api call repoGet owner=astrazds repo=fjgo
./fjgo alias list
./fjgo alias inspect repo issues get
./fjgo repo get astrazds/fjgo
./fjgo repo topics astrazds/fjgo
./fjgo repo avatar astrazds/fjgo assets/icon.png
./fjgo release list astrazds/fjgo
```

Authentication uses Forgejo's token auth header:

```sh
export FJGO_TOKEN=your_access_token
```

Use a read-only token for authenticated reads such as `fjgo me`. Repo write
operations, including topic or avatar updates, need repository write access.
Release creation/upload needs release/package permission for the target repo,
depending on the Forgejo instance policy.

## CLI

`fjgo version` calls the Forgejo server `/version` endpoint.

`fjgo api list [filter]` lists every generated Swagger operation:

```sh
fjgo api list release
```

`fjgo api inspect <operationId>` shows method, path, summary, path parameters,
typed body model, and typed return model:

```sh
fjgo api inspect createCurrentUserRepo
```

`fjgo api call <operationId>` executes any operation by Swagger `operationId`:

```sh
fjgo api call getVersion
fjgo api call repoSearch q=fjgo limit=10
fjgo api call repoGet owner=astrazds repo=fjgo
fjgo api call createCurrentUserRepo -body '{"name":"demo","private":true}'
```

For `api call`, `name=value` arguments matching path parameters fill the path;
the rest become query parameters. JSON bodies can be inline, `@file`, or `-`
for stdin.

`fjgo alias list` shows generated convenience commands for clear Swagger path
shapes. `fjgo alias inspect <command...>` shows the mapped operation, required
positional args, method, path, body type, and whether `--yes` is required.
Aliases use positional path args plus `name=value` query args, with `-body`
matching `api call`.

Common aliases:

```sh
fjgo repo get astrazds/fjgo
fjgo repo topics astrazds/fjgo
fjgo repo topics astrazds/fjgo --set forgejo,go,cli
fjgo repo avatar astrazds/fjgo assets/icon.png
fjgo release list astrazds/fjgo
```

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
curl -fsSL https://repos.astrazds.net/swagger.v1.json -o swagger.v1.json
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
repo, err := client.RepoGet(ctx, "astrazds", "fjgo", forgejo.RequestOptions{})
created, err := client.CreateCurrentUserRepo(ctx, &forgejo.CreateRepoOption{
	Name:    "demo",
	Private: true,
}, forgejo.RequestOptions{})
```

Use `forgejo.RequestOptions.Query` for query parameters.

## Development

```sh
./scripts/verify.sh
```

Generated files are committed for consumers, but should only be edited through
`go generate ./internal/forgejo`.

Forgejo Actions runs the same verifier on pushes and pull requests via
`.forgejo/workflows/verify.yml`.

## Release

Build release archives into `dist/`:

```sh
VERSION=v0.8.0 ./scripts/release.sh
```

Override targets when testing locally:

```sh
VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
```

The script embeds `version`, `commit`, and UTC build date into `fjgo --version`
and writes `dist/checksums.txt`.

Pushing a `v*` tag runs `.forgejo/workflows/release.yml`, builds archives, and
uploads them to a Forgejo release using the Actions token.

Smoke check a published release archive:

```sh
VERSION=v0.8.0 ./scripts/smoke-release.sh
```

## License

MIT

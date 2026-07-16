# Developing fjgo

This guide is for contributors and release maintainers.

## Build and verify

Build the CLI:

```sh
go build ./cmd/fjgo
./fjgo --version
```

Run the complete local verification gate before handing work back:

```sh
./scripts/verify.sh
```

The verifier regenerates and formats code, runs Go and Node tests, builds the
CLI, checks the embedded and public skills, runs public API smoke tests, audits
Swagger coverage, checks the npm package, and builds a test release archive.

Forgejo Actions runs the same script on pushes and pull requests through
`.forgejo/workflows/verify.yml`.

## Agent-job benchmark tracer

Run the deterministic black-box tracer from the repository root:

```sh
go run ./cmd/fjgo-benchmark
```

The command builds `fjgo` once, runs the operation-discovery scenario against
an offline local Forgejo fixture, and writes versioned result JSON to stdout.
Use `-fjgo ./fjgo` to select an existing binary instead.

To evaluate another safe command sequence against the same outcome oracle, pass
a bounded JSON array of argument arrays with `-commands-file`. Use
`{fixture_base_url}` where the sequence needs the local fixture URL:

```json
[
  ["-base-url", "{fixture_base_url}/api/v1", "api", "--json", "raw", "GET", "/repos/search", "q=benchmark-target"]
]
```

## Generated Forgejo API client

Generated API code lives in:

- `internal/forgejo/endpoints_gen.go`: operations and lookup metadata.
- `internal/forgejo/models_gen.go`: Go types from Swagger definitions.

Regenerate both files from the committed `swagger.v1.json`:

```sh
go generate ./internal/forgejo
```

Generate from another local or remote specification only when intentionally
updating the API surface:

```sh
SPEC=/path/to/swagger.v1.json go generate ./internal/forgejo
```

Do not edit generated files by hand. The generator rejects Swagger features it
cannot represent faithfully, and verification compares generated operations
and models with the pinned specification.

Generated client methods accept a context, typed path and body arguments, and
`forgejo.RequestOptions` for query values and advanced request behavior:

```go
repo, err := client.RepoGet(ctx, "owner", "repo", forgejo.RequestOptions{})
created, err := client.CreateCurrentUserRepo(ctx, &forgejo.CreateRepoOption{
	Name:    "demo",
	Private: true,
}, forgejo.RequestOptions{})
```

Response reads are limited to 32 MiB. Multipart uploads stream files from disk.
Raw and streaming client methods are available for text, binary, and file
responses.

## Authenticated smoke test

The optional authenticated smoke test creates a temporary private repository,
tests write operations, and deletes the repository when it finishes:

```sh
go build ./cmd/fjgo
FJGO_HOST=forgejo.example.com \
FJGO_TOKEN=your_access_token \
./scripts/smoke-auth.sh
```

Set `FJGO_TEST_ORG` to create the repository in an organization or
`FJGO_TEST_REPO_NAME` to choose its name. Set `FJGO_SMOKE_AUTH=1` to include
this test in `./scripts/verify.sh`.

Use [alpha.md](alpha.md) for the live field-validation checklist and failure
report format.

## Build a release

Build all default release archives and checksums:

```sh
VERSION=v1.0.0 ./scripts/release.sh
```

Build one target while testing:

```sh
VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
```

Smoke-test a published release:

```sh
VERSION=v1.0.0 ./scripts/smoke-release.sh
```

Pushing a `v*` tag runs verification, builds archives, and creates a Forgejo
release. Release notes belong in `CHANGELOG.md`.

## Publish the npm launcher

The version in `package.json` must match the release tag without its leading
`v`. For example, tag `v1.1.0` uses npm version `1.1.0`.

Tag releases publish automatically when the Forgejo Actions secret `NPM_TOKEN`
is configured. To publish manually after the matching Forgejo release exists:

```sh
npm test
npm publish
```

The npm package contains a small zero-dependency Node launcher. It downloads
the platform release archive, verifies it against `checksums.txt`, extracts the
`fjgo` binary, and stores it in the user's cache.

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

## Agent-job benchmark

Run the complete deterministic black-box benchmark from the repository root:

```sh
go run ./cmd/fjgo-benchmark
```

The command builds `fjgo` once, runs all 47 scenarios against isolated offline
Forgejo fixtures, and writes versioned deterministic result JSON to stdout. The
catalog covers operation and model discovery, repository and host context,
compact inspection and output recovery, structured errors and capability
recovery, and mutation and credential safety. Use `-fjgo ./fjgo` to select an
existing binary instead.
Each result retains token-safe normalized CLI arguments, exit and byte counts,
structured recovery evidence where available, normalized Forgejo requests, and
manual-correction accounting without retaining unrestricted process output.

The authoritative current-product baseline is committed as
`internal/benchmark/baseline.json`. Regenerate its JSON, concise Markdown
summary, and candidate-gap report together:

```sh
go run ./cmd/fjgo-benchmark -write-baseline internal/benchmark/baseline.json
```

The generated `baseline.gaps.md` groups observed failures and friction by
frequency, safety impact, agent-job impact, and bounded evidence. It is a
review input only: generation does not create tracker issues, and endpoint
count alone does not justify a curated command. Check all three committed
artifacts without rewriting them:

```sh
go run ./cmd/fjgo-benchmark -check-baseline internal/benchmark/baseline.json
```

`./scripts/verify.sh` runs this full offline check. Public-demo smoke and the
optional authenticated smoke remain separate validation layers and are not
part of benchmark scoring.

Select a smaller diagnostic run by repeating `-scenario` or `-category`:

```sh
go run ./cmd/fjgo-benchmark -category mutation-safety
go run ./cmd/fjgo-benchmark -scenario mutation.dry-run
```

Compare the live product with the committed baseline and emit scenario-level
JSON deltas:

```sh
go run ./cmd/fjgo-benchmark -compare-baseline internal/benchmark/baseline.json
```

To evaluate alternative autonomous or corrected sequences across the catalog,
pass `-scenario-runs-file` with an object keyed by stable scenario ID:

```json
{
  "repository-context.root-flag": {
    "commands": [
      ["-base-url", "{fixture_base_url}/api/v1", "api", "--json", "raw", "GET", "/repos/benchmark/target"]
    ],
    "manual_corrections": 0
  }
}
```

An alternative sequence is scored by the scenario's external completion oracle,
not by matching the default command. A positive `manual_corrections` count is
recorded as `manually_corrected` and cannot pass as autonomous completion.

For Codex, Claude Code, OpenCode, or another external host, use the
[portable agent-job benchmark packet](benchmark-agent-packet.md). The packet
defines host-neutral outcomes, context, mutation and credential boundaries,
bounded evidence, and the import record. Importing a record does not launch
fjgo or embed, authenticate, or control an agent runtime:

```sh
go run ./cmd/fjgo-benchmark -import-host-run host-run.json
```

To evaluate another safe command sequence against the original tracer outcome
oracle, pass a bounded JSON array of argument arrays with `-commands-file`. Use
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
VERSION=v1.3.0 ./scripts/release.sh
```

Build one target while testing:

```sh
VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
```

Smoke-test a published release:

```sh
VERSION=v1.3.0 ./scripts/smoke-release.sh
```

Pushing a `v*` tag runs verification, builds archives, and creates a Forgejo
release. Release notes belong in `CHANGELOG.md`.

## Publish the npm launcher

The version in `package.json` must match the release tag without its leading
`v`. For example, tag `v1.3.0` uses npm version `1.3.0`.

Tag releases publish automatically when the Forgejo Actions secret `NPM_TOKEN`
is configured. To publish manually after the matching Forgejo release exists:

```sh
npm test
npm publish
```

The npm package contains a small zero-dependency Node launcher. It downloads
the platform release archive, verifies it against `checksums.txt`, extracts the
`fjgo` binary, and stores it in the user's cache.

# Changelog

## Unreleased

- Expanded the deterministic black-box benchmark from one tracer into a
  46-scenario offline catalog covering discovery and context, compact
  inspection, structured recovery, capability behavior, mutations, and
  credential safety.
- Added token-safe normalized CLI/request evidence, alternative scenario-run
  records, explicit manual-correction accounting, baseline comparison, and a
  bounded portable agent-host result importer.
- Captured the authoritative current-product baseline with generated summary
  and candidate-gap report, and made the full deterministic check part of
  repository verification.

## v1.1.0 - 2026-07-16

- Added a Forgejo-hosted Agent Skills distribution installable with
  `npx skills add https://repos.astrazds.net/astrazds/fjgo.git --skill fjgo -g`.
- Added a zero-dependency npm launcher so the skill can run the matching,
  checksum-verified native CLI release through `npx -y fjgo` on demand.
- Extended tag releases to publish the launcher when an `NPM_TOKEN` Actions
  secret is configured.
- Added the first deterministic black-box agent-job benchmark tracer with an
  external Forgejo fixture oracle, isolated subprocess execution, bounded
  token-safe evidence, alternative safe command sequences, and versioned JSON
  results.
- Fixed curated issue dependency and blocking mutations to send complete
  same-repository `IssueMeta` bodies in live requests and token-safe previews.
- Prevented nonexistent issue-label filters from silently broadening curated
  issue lists, with deterministic structured errors in TOON and JSON modes.

## v1.0.0 - 2026-07-10

- Promoted `fjgo` to its feature-complete v1 contract: curated AXI workflows
  backed by an exact, audited generic surface for every pinned/live Forgejo v15
  Swagger operation.
- Completed TOON specification v3.3 output behavior, compact schemas, bounded
  truncation, contextual help, definitive empty states, and parseable
  structured errors for coding agents.
- Added token or Basic authentication, optional TOTP and sudo impersonation,
  with credential-safe status, request previews, and API diagnostics.
- Made generated typed methods faithfully handle JSON, text, and binary/file
  request and response bodies, preserve Swagger media types, and expose optional
  response status/header metadata.
- Added `--include-response` to buffered generic calls with sensitive response
  header redaction, while keeping `--raw` and `--output` as streaming paths.
- Made every explicit JSON response boundary parseable for JSON, text, binary,
  and empty `204` successes across generic and curated commands.
- Honored Swagger optional body parameters and numeric minimum constraints, and
  exposed documented response codes and headers through operation inspection.
- Preserved operation tags, deprecation flags and replacement descriptions,
  model titles/descriptions/formats/examples, and unsigned 64-bit model fields.
- Added the release-attachment endpoint's documented raw
  `application/octet-stream` request mode alongside multipart uploads.
- Streamed multipart file bodies from disk instead of buffering whole assets in
  memory.
- Kept optional nil typed bodies absent and preserved explicit
  `RequestOptions.Body` overrides for optional scalar zero-value fidelity.
- Added `alias omissions` so every operation without a convenience alias has an
  explicit reason and generic escape hatch.
- Replaced count-only coverage confidence with exact operation/model bijection,
  alias-partition, and fail-fast unsupported-Swagger-feature checks.

## v0.16.0 - 2026-07-09

- Hardened AXI error handling so root/global unknown flags and nested unknown
  subcommands return structured stdout errors with inline valid flags or
  subcommands instead of generic parser output.
- Added focused subcommand help coverage across repo, release, skill, API,
  issue, PR, run, workflow, search, label, secret, and variable surfaces.
- Added cheap aggregate counts to the no-args home view for open issues and
  pull requests when Forgejo exposes total-count headers.
- Expanded session-end hook capture from a tab-separated timestamp into
  structured JSONL with cwd, platform, branch, HEAD, repo, and dirty-file count.
- Made tests hermetic against ambient `FJGO_HOST`, `FJGO_TOKEN`, and
  `FJGO_REPO`, and fixed explicit `-base-url` precedence so a configured
  shell cannot redirect httptest-backed tests to a live Forgejo host.
- Updated authenticated smoke coverage to create a unique temporary repository
  and delete it on exit or failure.

## v0.15.0 - 2026-07-08

- Reworked `fjgo` around AXI conventions: TOON-first output, structured stdout
  errors, compact default schemas, truncation with `--full`, contextual help,
  and token-safe dry-run/request previews.
- Added content-first home/dashboard output, explicit ambient hook setup for
  Claude Code, Codex, and OpenCode, and generated embedded skill drift checks.
- Added root and command-local repo/host context through `--repo`, `FJGO_REPO`,
  `--host`, and `FJGO_HOST`, with explicit `-base-url` for custom API paths.
- Expanded curated Forgejo API-backed commands for repositories, issues, pull
  requests, labels, search, Actions runs, workflow dispatch/content, releases,
  release assets, secrets, variables, and guarded self-update checks.
- Added raw API request support through `fjgo api raw`, richer `--fields`
  handling, structured Forgejo error translation, and generated operation/model
  inspection checks for agents.
- Removed curated command stubs for GitHub-shaped workflows that are not
  exposed by the pinned Forgejo Swagger. Curated aliases now only wrap
  documented Forgejo API operations.
- Updated docs, alpha field-test packet, agent setup prompt, verification, and
  authenticated smoke coverage for the expanded CLI surface.

# Changelog

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

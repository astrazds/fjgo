# Changelog

## v0.15.0 - 2026-07-08

- Reworked `fjgo` around AXI conventions: TOON-first output, structured stdout
  errors, compact default schemas, truncation with `--full`, contextual help,
  and token-safe dry-run/request previews.
- Added content-first home/dashboard output, explicit ambient hook setup for
  Claude Code, Codex, and OpenCode, and generated embedded skill drift checks.
- Added root and command-local repo/host context through `--repo`, `FJGO_REPO`,
  `--host`, and `FJGO_HOST`, while preserving `FJGO_BASE_URL` for custom API
  paths.
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

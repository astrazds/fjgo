---
name: fjgo
description: Forgejo repository operations through the fjgo CLI for coding agents. Use when Codex needs to inspect, triage, modify, release, or report on Forgejo repos; work with Forgejo issues, pulls, releases, repository metadata, files, labels, topics, or attachments; verify auth/repo context; or produce token-safe dry-run plans before mutating Forgejo state.
---

# fjgo

Use `fjgo` for Forgejo-specific work and normal `git` for local commit, branch,
diff, and remote operations.

## First Command

Set `FJGO_BASE_URL` to the same Forgejo host as the checkout remote, then run
this before acting on a repo:

```sh
export FJGO_BASE_URL=https://forgejo.example.com/api/v1
fjgo -R origin doctor --json
```

If there is no git remote, pass `owner/repo` to repo-scoped commands directly.
For private instances or write tasks, rely on `FJGO_TOKEN` from the environment.
Never print or ask to expose the token. `doctor`, `auth status`, dry runs, and
request previews do not print token values.

## Safe Defaults

- Use `--dry-run` or `--print-request` before mutating commands when planning.
- Use `--yes` only after the target repo, operation, body, and path are clear.
- Prefer `alias inspect` and `api inspect` before guessing arguments or bodies.
- Prefer generated aliases for common workflows; fall back to `api call` or
  `api upload` when no short alias exists.
- Use `model inspect <Model>` to build JSON bodies from required fields.

## References

Read [references/workflows.md](references/workflows.md) for issue, pull request,
release, repository metadata, file/content, field feedback, and auth-smoke
recipes.

For first-time setup of another agent, use the repository prompt at
`docs/agent-setup-prompt.md`.

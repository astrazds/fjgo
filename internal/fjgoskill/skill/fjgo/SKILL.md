---
name: fjgo
description: Forgejo repository operations through the fjgo CLI for coding agents. Use when Codex needs to inspect, triage, modify, release, or report on Forgejo repos; work with Forgejo issues, pulls, releases, repository metadata, files, labels, topics, or attachments; verify auth/repo context; or produce token-safe dry-run plans before mutating Forgejo state.
---

# fjgo

Use `fjgo` for Forgejo-specific work and normal `git` for local commit, branch,
diff, and remote operations. `fjgo` is AXI-shaped: stdout is compact TOON by
default, errors are structured on stdout, mutating commands require `--yes`, and
dry runs/request previews are token-safe.

## First Command

Set `FJGO_HOST` to the same Forgejo host as the checkout remote,
then run this before acting on a repo:

```sh
export FJGO_HOST=forgejo.example.com
fjgo -R origin
fjgo -R origin doctor
fjgo -R origin issue list --state open
fjgo -R origin pr list
fjgo -R origin run list
```

Use `-R origin` only when that git remote is a Forgejo remote on `FJGO_HOST`. GitHub and GitLab remotes are not Forgejo remotes. For those checkouts, set `FJGO_HOST` and pass `--repo OWNER/REPO`.

If there is no git remote, pass `owner/repo` to repo-scoped commands directly,
use root or command-local `--repo OWNER/REPO`, or export `FJGO_REPO=OWNER/REPO`.
For private instances or write tasks, rely on `FJGO_TOKEN` from the environment.
Never print or ask to expose the token. `doctor`, `auth status`, dry runs, and
request previews do not print token values. API error diagnostics are also
token-safe, including server-reflected token text.

Use `--json` only when a command explicitly supports it and raw JSON is needed,
for example `fjgo doctor --json` or `fjgo api --json call getVersion`.

## Setup Paths

Recommend ambient hooks first when the user wants persistent Forgejo context:

```sh
fjgo setup hooks --check
fjgo setup hooks
```

This installs managed SessionStart hooks for Claude Code and Codex plus an
OpenCode plugin. Restart the agent session after installing hooks. The skill is
the secondary on-demand path and can be installed with:

```sh
fjgo skill install --force
```

## Safe Defaults

- Use `--dry-run` or `--print-request` before mutating commands when planning.
- Use `--yes` only after the target repo, operation, body, and path are clear.
- Prefer curated workflow commands first: `issue`, `pr`, `run`, `workflow`,
  `search`, `repo`, `label`, `secret`, `variable`, and `release`.
- Use command-local `--repo OWNER/REPO` and `--host HOST` when it keeps the command
  self-contained for another agent or transcript.
- Use repo lifecycle commands for list/create/edit/fork, branches,
  collaborators, and branch protection before falling back to generated aliases.
- Use release commands for view/latest/edit/delete, asset list/delete, upload,
  and `--notes-file` notes.
- Use issue/PR advanced commands for pins, dependencies, reactions, deadlines,
  tracked time, PR edit/close/reopen/comment, review requests/comments,
  PR update, diff, and patch.
- Use workflow/run commands for workflow list/view/run and run list/view/watch.
- Use `alias inspect` and `api inspect` before guessing generated arguments or
  bodies.
- Use `alias omissions` to see why an operation has no generated alias and the
  exact generic command that replaces it.
- Fall back to generated aliases, `api call`, `api upload`, or `api raw` when no curated
  command exists.
- Use `--include-response` on buffered generic calls when status or response
  headers are needed; use `--raw` or `--output` for binary payloads.
- Use `model inspect <Model>` to build JSON bodies from required fields.
- Use `--full` when TOON output reports truncated long text.
- Use `--fields` on list/detail workflow commands to request only needed
  fields; unknown fields fail with the valid field list.

<!-- fjgo:static-guidance:start -->
Use fjgo for Forgejo-specific API work and normal git for local branch, commit, diff, and remote operations.

First commands:
  fjgo --repo OWNER/REPO
  fjgo --repo OWNER/REPO doctor
  fjgo --repo OWNER/REPO repo get
  fjgo --repo OWNER/REPO issue list --state open
  fjgo --repo OWNER/REPO pr list
  fjgo --repo OWNER/REPO run list

Safe mutation pattern:
  inspect the alias or operation first
  run with --dry-run --yes
  rerun without --dry-run only after target repo, path, and body are correct

Useful surfaces:
  fjgo setup hooks
  fjgo skill install --force
  fjgo auth status
  fjgo api raw GET /repos/OWNER/REPO
  fjgo issue view OWNER/REPO 1 --full
  fjgo pr view OWNER/REPO 1 --reviews
  fjgo pr checks OWNER/REPO 1
  fjgo search issues "bug" --repo OWNER/REPO
  fjgo release list OWNER/REPO
  fjgo release assets list OWNER/REPO 1
  fjgo update --check
  fjgo model inspect CreateIssueOption
<!-- fjgo:static-guidance:end -->

## References

Read [references/workflows.md](references/workflows.md) for issue, pull request,
release, repository metadata, file/content, field feedback, and auth-smoke
recipes.

For first-time setup of another agent, use the repository prompt at
`docs/agent-setup-prompt.md`.

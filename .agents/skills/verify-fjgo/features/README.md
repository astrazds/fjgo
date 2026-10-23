# fjgo verification map

This directory is the maintained source for verifying the user-facing behavior of the fjgo CLI. Read the index before driving the binary, then use the matching feature file as the recipe.

## Baseline preconditions

- Build `./fjgo` from this repository with the Launch steps in `../SKILL.md`.
- Run the skill doctor and require `version: v1.4.3` before any feature command.
- Clear `FJGO_TOKEN`, `FJGO_USERNAME`, `FJGO_PASSWORD`, `FJGO_OTP`, `FJGO_SUDO`, and `FJGO_REPO` on every command.
- For Forgejo reads, set `FJGO_HOST=v15.next.forgejo.org` and use `kavemand/.forgejo` when the recipe names that repo.
- Never pass `-R origin` in this clone. `origin` is GitHub.
- Never drive a binary that this verification run did not build.

## Driving conventions

- Start every recipe from the baseline state unless its preconditions say otherwise.
- Run each user command in its own tmux session named `fjgo-verify-<feature-id>-<short-id>`.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Record stdout, stderr, the exit code, and a tmux transcript under `.agents/skills/verify-fjgo/evidence/<feature-id>/`.
- Do not remove proof artifacts during cleanup.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the final screen.
- CLI proof includes the command, stdout, stderr, and exit code.
- A read proof includes the field the user asked for, such as a version line, a repository full name, or an issue count line.
- Record the feature ID and entry point used with every artifact.
- Report an unreachable path with the attempted command and the unmet precondition.
- Do not report a skipped entry point as verified through a different path.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with tmux` starts with `Preconditions:` and uses labeled bullets that pair each user action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Features

- [Server version](./server-version.md) reads the Forgejo server version from the configured host.
- [Doctor](./doctor.md) prints a redacted JSON report for a public repository.
- [API inspect](./api-inspect.md) shows one generated operation without contacting a host.
- [Repository get](./repo-get.md) reads one public repository summary.
- [Issue list](./issue-list.md) lists open issues for a public repository, including the empty result.

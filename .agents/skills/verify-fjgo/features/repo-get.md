# Repository get

Repository get prints a short summary of one Forgejo repository, including its full name, privacy, and default branch.

## Sub-features

- `repo-get-public` reads `kavemand/.forgejo` without a token.
- `repo-get-missing-arg` returns a usage error when no repository is given.

## How to get to it (user POV)

- Run `fjgo repo get OWNER/REPO`.
- Run `fjgo --repo OWNER/REPO repo get`.
- Run `fjgo --host <host> --repo OWNER/REPO repo get`.

## Driving it with tmux

Preconditions:

- `./fjgo` exists and the skill doctor recorded `version: v1.4.3`.
- `FJGO_TOKEN` is unset on the command.
- The session name is `fjgo-verify-repo-get-<short-id>` and its working directory is the repository root.

- **Read the public repository.** Inside the session, run `env -u FJGO_TOKEN -u FJGO_USERNAME -u FJGO_PASSWORD -u FJGO_OTP -u FJGO_SUDO -u FJGO_REPO FJGO_HOST=v15.next.forgejo.org ./fjgo repo get kavemand/.forgejo > .agents/skills/verify-fjgo/evidence/repo-get/stdout.txt 2> .agents/skills/verify-fjgo/evidence/repo-get/stderr.txt; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/repo-get/exit.txt`. Exit code `0`. Stdout contains `full_name: kavemand/.forgejo`, `private: false`, and `default_branch: master`. Stderr is empty.
- **Missing repository.** Run `env -u FJGO_TOKEN -u FJGO_REPO -u FJGO_HOST ./fjgo repo get > .agents/skills/verify-fjgo/evidence/repo-get/usage.txt 2> .agents/skills/verify-fjgo/evidence/repo-get/usage.err; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/repo-get/usage.exit`. The exit code is not `0`. The error names `owner/repo`.
- **Proof.** Save the tmux pane to `.agents/skills/verify-fjgo/evidence/repo-get/transcript.txt`. `stdout.txt` still shows `full_name: kavemand/.forgejo`.

## Gotchas

- The public smoke repository uses `master` as its default branch. Do not require `main`.
- `fjgo repo get` with no argument does not fall back to this clone's `origin` remote.
- Do not use `-R origin` in this clone.

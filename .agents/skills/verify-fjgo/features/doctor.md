# Doctor

Doctor prints a redacted report of the fjgo binary, the configured Forgejo host, whether a token is present, and the named repository. It does not print token values.

## Sub-features

- `doctor-json` writes deterministic JSON for one public repository.
- `doctor-auth-absent` reports that no token is present when the environment has none.

## How to get to it (user POV)

- Run `fjgo doctor OWNER/REPO --json`.
- Run `fjgo doctor OWNER/REPO` for the TOON report.
- Run `fjgo --host <host> doctor OWNER/REPO --json`.

## Driving it with tmux

Preconditions:

- `./fjgo` exists and the skill doctor recorded `version: v1.4.3`.
- `FJGO_TOKEN` is unset on the command.
- The session name is `fjgo-verify-doctor-<short-id>` and its working directory is the repository root.

- **JSON report.** Ask for the public smoke repository. Inside the session, run `env -u FJGO_TOKEN -u FJGO_USERNAME -u FJGO_PASSWORD -u FJGO_OTP -u FJGO_SUDO -u FJGO_REPO FJGO_HOST=v15.next.forgejo.org ./fjgo doctor kavemand/.forgejo --json > .agents/skills/verify-fjgo/evidence/doctor/stdout.txt 2> .agents/skills/verify-fjgo/evidence/doctor/stderr.txt; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/doctor/exit.txt`. Exit code `0`. Stderr is empty.
- **Redacted auth.** Read `stdout.txt`. `token_present` is false. `auth.authenticated` is false. `base_url` is `https://v15.next.forgejo.org/api/v1`. `repo.owner` is `kavemand` and `repo.repo` is `.forgejo`. The file does not contain a token string.
- **Tool identity.** `tool.name` is `fjgo` and `tool.version` is `v1.4.3`.
- **Proof.** Save the tmux pane to `.agents/skills/verify-fjgo/evidence/doctor/transcript.txt`. The transcript shows the doctor command and `stdout.txt` still parses as JSON.

## Gotchas

- `fjgo doctor` without `owner/repo` or `--repo` returns a usage error. Pass `kavemand/.forgejo` on the command.
- `--json` changes the report to JSON. The TOON form is a different entry and is not a substitute for the JSON assertions above.
- The skill doctor in `../SKILL.md` is `./fjgo --version`. This feature is the product command `fjgo doctor`.
- Do not use `-R origin` in this clone. That remote is GitHub and the command will not resolve a Forgejo repository.

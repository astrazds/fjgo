# API inspect

API inspect prints one bundled Forgejo operation, including its method and path, without calling that operation.

## Sub-features

- `inspect-get-version` shows the `getVersion` operation.
- `inspect-unknown` fails when the operation id is not in the bundle.

## How to get to it (user POV)

- Run `fjgo api inspect <operationId>`.
- Run `fjgo api inspect <operationId> --json`.
- Run `fjgo api list <filter>` first when the operation id is unknown.

## Driving it with tmux

Preconditions:

- `./fjgo` exists and the skill doctor recorded `version: v1.4.3`.
- The session name is `fjgo-verify-api-inspect-<short-id>` and its working directory is the repository root.
- No `FJGO_HOST` is required.

- **Inspect getVersion.** Inside the session, run `env -u FJGO_HOST -u FJGO_TOKEN -u FJGO_USERNAME -u FJGO_PASSWORD -u FJGO_OTP -u FJGO_SUDO -u FJGO_REPO ./fjgo api inspect getVersion > .agents/skills/verify-fjgo/evidence/api-inspect/stdout.txt 2> .agents/skills/verify-fjgo/evidence/api-inspect/stderr.txt; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/api-inspect/exit.txt`. Exit code `0`. Stdout contains `id: getVersion`, `method: GET`, and `path: /version`. Stderr is empty.
- **Unknown id.** Run `env -u FJGO_HOST -u FJGO_TOKEN ./fjgo api inspect notARealOperation > .agents/skills/verify-fjgo/evidence/api-inspect/unknown.txt 2> .agents/skills/verify-fjgo/evidence/api-inspect/unknown.err; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/api-inspect/unknown.exit`. The exit code is not `0`.
- **Proof.** Save the tmux pane to `.agents/skills/verify-fjgo/evidence/api-inspect/transcript.txt`. `stdout.txt` still contains `path: /version`.

## Gotchas

- `fjgo api inspect` reads the bundled catalog. A network failure is not a failure of this feature.
- `fjgo api call getVersion` is a different entry point. It contacts the host. Do not report a call as an inspect proof.
- Passing extra words after the operation id is a usage error.

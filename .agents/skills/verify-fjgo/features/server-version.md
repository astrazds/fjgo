# Server version

Server version shows the Forgejo application version for the host the user configured. It does not print the fjgo binary version.

## Sub-features

- `version-read` prints the server version for an explicit host.
- `version-help` prints usage without contacting the host.

## How to get to it (user POV)

- Run `fjgo version` with `FJGO_HOST` set to the Forgejo host.
- Run `fjgo --host <host> version`.
- Run `fjgo version --help` to read usage.

## Driving it with tmux

Preconditions:

- `./fjgo` exists and the skill doctor recorded `version: v1.4.3`.
- `FJGO_TOKEN` is unset on the command.
- The session name is `fjgo-verify-server-version-<short-id>` and its working directory is the repository root.

- **Read the server version.** Ask fjgo for the version of `v15.next.forgejo.org`. Inside the session, run `env -u FJGO_TOKEN -u FJGO_USERNAME -u FJGO_PASSWORD -u FJGO_OTP -u FJGO_SUDO -u FJGO_REPO FJGO_HOST=v15.next.forgejo.org ./fjgo version > .agents/skills/verify-fjgo/evidence/server-version/stdout.txt 2> .agents/skills/verify-fjgo/evidence/server-version/stderr.txt; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/server-version/exit.txt`. Exit code `0`. Stdout contains a `version:` line whose value is not `v1.4.3`. Stderr is empty.
- **Confirm the binary version is a different command.** Run `./fjgo --version > .agents/skills/verify-fjgo/evidence/server-version/binary-version.txt`. That file contains `version: v1.4.3`, which must differ from the server version line.
- **Help entry.** Run `./fjgo version --help > .agents/skills/verify-fjgo/evidence/server-version/help.txt`. Exit code `0`. The file contains `usage: fjgo version` and does not contain a live server version such as `15.0`.
- **Proof.** Save the tmux pane with `tmux capture-pane -p -t "$SESSION" > .agents/skills/verify-fjgo/evidence/server-version/transcript.txt`. The transcript shows the version command. `stdout.txt` keeps the server version line. Neither file contains a token.

## Gotchas

- `fjgo --version` is the binary version. `fjgo version` is the server version. A proof that only runs `--version` does not cover this feature.
- TOON stdout may have no trailing newline. Write the exit code to its own file.
- Ambient `FJGO_TOKEN` is ignored on the default public API unless the host or base URL is set. Set `FJGO_HOST` anyway and clear the token so the transcript shows the intended host.
- Do not use `-R origin` in this clone.

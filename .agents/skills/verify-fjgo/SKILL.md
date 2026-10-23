---
name: verify-fjgo
description: Drive the fjgo CLI the way a user does, from a source build in this repo. Use to launch the binary, check it is worth driving, exercise a mapped Forgejo read, and keep the transcript after teardown.
---

# Verify fjgo

fjgo is a short-lived CLI. There is no server to keep alive. Launch means build `./fjgo` once from this checkout. Each drive is a new process in its own tmux session. The primary surface is the terminal. The npm launcher and the public `skills/fjgo` package are separate. This skill drives the binary built here.

This clone's `origin` remote is GitHub. Never pass `-R origin` or `--repo-from-remote` in this checkout. Point reads at a Forgejo host with `FJGO_HOST` and an `owner/repo` argument.

## Launch

From the repository root, put a Go toolchain that can build this module first on `PATH`. On this machine the mise shim reports that no version is set, so prepend `/home/astrazds/.local/share/mise/installs/go/1.27.1/bin`.

`scripts/verify.sh` sets `GOTMPDIR` to `${PWD}/.gocache/tmp` when it is unset, because `/tmp` on this host is not executable. Do the same before `go build` when `GOTMPDIR` is empty.

```sh
if [ -z "${GOTMPDIR:-}" ]; then
  GOTMPDIR="${PWD}/.gocache/tmp"
  export GOTMPDIR
fi
mkdir -p "$GOTMPDIR"
export TMPDIR="$GOTMPDIR"
go build -o ./fjgo ./cmd/fjgo
```

Ready means `./fjgo --version` exits 0 and stdout contains `version:` followed by the `version` constant in `cmd/fjgo/main.go`. For this tree that constant is `v1.4.3`. A source build without ldflags also prints `commit: none` and `date: unknown`. Stderr is empty. TOON records may omit a trailing newline, so do not require one.

`.gocache/` and the `./fjgo` binary are gitignored. Do not commit them.

## Doctor

Run this before any drive, and again whenever a command looks wrong. It answers whether this build is worth driving. It is not the product command `fjgo doctor`.

```sh
test -x ./fjgo
./fjgo --version > .agents/skills/verify-fjgo/evidence/doctor-version.txt
```

Pass only when the exit code is 0, stderr is empty, and the file contains `version: v1.4.3`. Fail the run when the binary is missing, the exit code is not 0, or the version line does not match `cmd/fjgo/main.go`. Do not continue into a feature recipe after a failed doctor.

## Drive

Read `features/README.md`, then the feature file. Start every recipe from a clean credential environment. fjgo does not store a token. Host and token come from the environment and flags of that process. Two drives can run side by side only when each one has its own tmux session and its own explicit environment. Do not drive a shell that already exported `FJGO_TOKEN`, `FJGO_USERNAME`, `FJGO_PASSWORD`, or `FJGO_OTP`. Clear those names on the command itself.

```sh
RUN_ID="<feature-id>-<short-id>"
SESSION="fjgo-verify-${RUN_ID}"
tmux has-session -t "$SESSION" 2>/dev/null && tmux kill-session -t "$SESSION"
tmux new-session -d -s "$SESSION" -c "$PWD"
```

Send the feature command with `tmux send-keys` and a trailing `C-m`. Keep the payload in single quotes so `$?` is the fjgo exit code inside the session. Capture the pane after the exit file appears.

```sh
tmux capture-pane -p -t "$SESSION" > .agents/skills/verify-fjgo/evidence/<feature-id>/transcript.txt
```

The public read host used by `scripts/verify.sh` is `v15.next.forgejo.org`. The public read repo is `kavemand/.forgejo`. Set `FJGO_HOST` to that host and leave `FJGO_TOKEN` unset for those reads. `fjgo api inspect` does not need a host.

## Evidence

Write proof under `.agents/skills/verify-fjgo/evidence/<feature-id>/`. That directory is the proof location. Cleanup must not delete it.

Each drive records the command, stdout, stderr, and exit code, plus the tmux transcript of the action. A later read of the same user-facing data is part of the proof when the feature file asks for it. Do not prove a write by calling an unexported function or a test-only endpoint.

`./fjgo --version` and `fjgo api inspect` stay on this machine. `fjgo version`, `fjgo doctor`, `fjgo repo get`, and `fjgo issue list` perform real HTTP reads against the host you set. There is no dry-run on those reads. A mutation preview is a different command and must include `--dry-run` and `--yes`. When you do run a preview, confirm the side effect by observing that no Forgejo object changed and that stdout is a request preview. Do not trust the flag name alone. These read recipes do not send a mutation.

Stdout must not contain a token, password, or OTP. Do not print `FJGO_TOKEN` while capturing evidence.

## Cleanup

Kill only the tmux session this run created.

```sh
tmux has-session -t "$SESSION" 2>/dev/null && tmux kill-session -t "$SESSION"
```

Do not kill processes by name. Do not delete `.agents/skills/verify-fjgo/evidence/`. You may delete `./fjgo` when this run built it. Leave `.gocache/` in place. It is gitignored scratch owned by the Go toolchain and by `scripts/verify.sh`.

After cleanup, `test -f` the evidence files from the drive. A missing transcript means the run failed.

## Helpers

This skill has no helper script. The commands above are the harness.

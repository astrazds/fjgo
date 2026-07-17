# fjgo Agent Setup Prompt

Copy this prompt into a coding agent while it is working inside a
Forgejo-backed checkout.

```text
Set up this checkout for Forgejo work with fjgo.

Rules:
- Do not ask me to paste a token into chat.
- Do not print token values.
- Treat `doctor`, `auth status`, dry runs, request previews, and structured
  errors as token-safe diagnostics.
- Expect TOON on stdout by default. Use `--json` only for commands that
  explicitly support it and when raw JSON is needed.
- Use normal git for local git work and fjgo for Forgejo API work.
- Use explicit repo context with root or command-local `--repo owner/repo` or
  `FJGO_REPO=owner/repo`
  when there is no suitable Forgejo git remote.
- Prefer dry runs before any Forgejo write.
- Keep changes minimal; do not add project files unless setup requires it.

Steps:
1. Check whether `fjgo` is available:
   - Run `command -v fjgo`.
   - If missing, install the current stable release for this machine:
     - Detect OS with `uname -s`, mapping Linux to `linux` and Darwin to `darwin`.
     - Detect arch with `uname -m`, mapping `x86_64` to `amd64` and `aarch64`/`arm64` to `arm64`.
     - Download `https://repos.astrazds.net/astrazds/fjgo/releases/download/v1.2.0/fjgo_v1.2.0_${os}_${arch}.tar.gz`.
     - Extract it and install `fjgo` to `~/.local/bin/fjgo`.
     - If the OS/arch is unsupported or `~/.local/bin` is not on PATH, stop and tell me the exact command to run.
2. Run `fjgo --version`.
3. Optionally check for a newer binary with `fjgo update --check`; do not run
   `fjgo update --yes` unless I explicitly ask for a local binary update.
4. Install or repair ambient Axi hooks with `fjgo setup hooks --check`, then
   `fjgo setup hooks`. Restarting the agent session is needed before hooks feed
   context automatically, so continue this setup manually too.
5. Install or refresh the bundled skill with `fjgo skill install --force`, then
   run `fjgo skill generate --check`.
6. Determine the Forgejo API base:
   - If `FJGO_HOST` is already set, use it; it derives `https://host/api/v1`.
   - Otherwise inspect `git remote get-url origin`.
   - For a remote like `https://host/owner/repo.git` or `git@host:owner/repo.git`, export `FJGO_HOST=host`.
   - If a non-standard API path is required, pass it explicitly as `-base-url`.
7. Determine repo context:
   - Prefer `git remote get-url origin` plus `fjgo -R origin` when it is a Forgejo remote on the configured base host.
   - If there is no suitable remote and `FJGO_REPO` is unset, ask me for the `owner/repo` value.
   - If I provide one, export it as `FJGO_REPO=owner/repo` and use commands without `-R origin`.
8. If the repo is private or a write task is requested and `FJGO_TOKEN` is not set, stop and tell me to set it locally:
   `export FJGO_TOKEN=...`
9. Run, using either `-R origin`, command-local `--repo owner/repo`, or the exported `FJGO_REPO` context:
   - `fjgo`
   - `fjgo doctor`
   - `fjgo doctor --json`
   - `fjgo skill status`
   - `fjgo issue list --state open`
   - `fjgo pr list`
   - `fjgo run list`
   - `fjgo repo get`
   - `fjgo release list`
10. If `skill status` reports `current: false`, rerun `fjgo skill install --force` and check again.
11. Finish by printing concise user instructions:
   - the detected `FJGO_HOST` or explicit `-base-url`
   - the detected repo context (`FJGO_REPO`, `--repo`, or `-R origin`)
   - whether a token is present, without printing it
   - the first commands to use:
     - `fjgo doctor --json`
     - `fjgo`
     - `fjgo issue list --state open`
     - `fjgo pr list --state open`
     - `fjgo run list`
     - `fjgo workflow view <workflow.yml> --full`
     - `fjgo api raw GET /repos/OWNER/REPO`
     - `fjgo repo branches list`
     - `fjgo release list`
     - `fjgo alias inspect <command...>`
     - `fjgo api inspect <operationId>`
   - for writes: inspect first, dry-run, then rerun with `--yes`.
```

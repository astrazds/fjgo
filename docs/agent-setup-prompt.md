# fjgo Agent Setup Prompt

Copy this prompt into a coding agent while it is working inside a
Forgejo-backed checkout.

```text
Set up this checkout for Forgejo work with fjgo.

Rules:
- Do not ask me to paste a token into chat.
- Do not print token values.
- Treat `doctor`, `auth status`, dry runs, request previews, and API errors as
  token-safe diagnostics.
- Use normal git for local git work and fjgo for Forgejo API work.
- Prefer dry runs before any Forgejo write.
- Keep changes minimal; do not add project files unless setup requires it.

Steps:
1. Check whether `fjgo` is available:
   - Run `command -v fjgo`.
   - If missing, install the current alpha release for this machine:
     - Detect OS with `uname -s`, mapping Linux to `linux` and Darwin to `darwin`.
     - Detect arch with `uname -m`, mapping `x86_64` to `amd64` and `aarch64`/`arm64` to `arm64`.
     - Download `https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.14.0/fjgo_v0.14.0_${os}_${arch}.tar.gz`.
     - Extract it and install `fjgo` to `~/.local/bin/fjgo`.
     - If the OS/arch is unsupported or `~/.local/bin` is not on PATH, stop and tell me the exact command to run.
2. Run `fjgo --version`.
3. Install or refresh the bundled skill with `fjgo skill install --force`.
4. Determine the Forgejo API base:
   - If `FJGO_BASE_URL` is already set, use it.
   - Otherwise inspect `git remote get-url origin`.
   - For a remote like `https://host/owner/repo.git` or `git@host:owner/repo.git`, use `https://host/api/v1`.
   - Export it for this session as `FJGO_BASE_URL`.
5. If the repo is private or a write task is requested and `FJGO_TOKEN` is not set, stop and tell me to set it locally:
   `export FJGO_TOKEN=...`
6. Run:
   - `fjgo -R origin doctor --json`
   - `fjgo skill status`
   - `fjgo alias inspect repo issues list`
   - `fjgo -R origin repo get`
7. If `skill status` reports `current: false`, rerun `fjgo skill install --force` and check again.
8. Finish by printing concise user instructions:
   - the detected `FJGO_BASE_URL`
   - whether a token is present, without printing it
   - the first commands to use:
     - `fjgo -R origin doctor --json`
     - `fjgo -R origin repo issues list state=open`
     - `fjgo -R origin repo pulls list state=open`
     - `fjgo alias inspect <command...>`
     - `fjgo api inspect <operationId>`
   - for writes: inspect first, dry-run, then rerun with `--yes`.
```

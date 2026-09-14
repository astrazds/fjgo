# Contributing to fjgo

Thanks for taking an interest in fjgo. Small, focused changes are easiest
to review.

Open issues and pull requests on GitHub at https://github.com/astrazds/fjgo.
This clone's `origin` is GitHub. Use `gh` for tracker work here. Use `fjgo` only against a Forgejo host.

## Before opening a pull request

1. Open an issue for behavior changes or substantial new work so the scope can
   be agreed first.
2. Keep fjgo an agent-first Forgejo CLI. Do not add a GUI, daemon, hosted
   control plane, or a second API client.
3. Do not add hand-written commands for endpoints absent from the pinned
   Forgejo Swagger. Prefer `api inspect` / `api call` / `api raw` until a
   curated alias removes real repetition.
4. Preserve AXI defaults: TOON on stdout, structured errors, `--yes` on
   mutations, token-safe dry runs, and no interactive prompts.
5. Do not add telemetry, credential storage, or logging of tokens, passwords,
   or one-time codes.
6. Do not commit generated release archives, `dist/`, `node_modules/`, local
   agent files, or editor state.
7. Run the complete local check:

   ```sh
   npm ci
   ./scripts/verify.sh
   go run ./cmd/fjgo-benchmark
   ```

   See [docs/development.md](docs/development.md) for generation, smoke, and
   release commands.

## Pull requests

Explain the problem, the chosen approach, and how you verified it. Add or
update a focused test for new non-trivial client behavior. By contributing,
you agree that your contribution is licensed under the repository's MIT
license.

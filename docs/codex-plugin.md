# Codex plugin

The fjgo repository root is a Codex plugin package. It distributes the existing
public `fjgo` Agent Skill with install-surface metadata, project branding, and
starter prompts. The plugin remains a thin packaging layer over the released
CLI rather than a second Forgejo client.

## Install from a marketplace

Once a configured marketplace exposes the package, install it with:

```sh
codex plugin add fjgo@MARKETPLACE
```

Start a new Codex thread after installation so the bundled skill appears in
the available skill list. Marketplace publication is separate from fjgo
releases; until a marketplace lists the plugin, install the standalone Agent
Skill as described in the [README](../README.md#agent-skill).

## Test through a local marketplace

To evaluate a checkout before publishing it, place or copy the checkout at
`plugins/fjgo` under a temporary marketplace root. Add
`.agents/plugins/marketplace.json` under that root:

```json
{
  "name": "fjgo-local",
  "interface": {
    "displayName": "fjgo Local"
  },
  "plugins": [
    {
      "name": "fjgo",
      "source": {
        "source": "local",
        "path": "./plugins/fjgo"
      },
      "policy": {
        "installation": "AVAILABLE",
        "authentication": "ON_USE"
      },
      "category": "Developer Tools"
    }
  ]
}
```

Register the marketplace root and install the plugin:

```sh
codex plugin marketplace add /absolute/path/to/marketplace-root
codex plugin add fjgo@fjgo-local
```

These commands update the current user's Codex marketplace and plugin
configuration. Use a disposable marketplace root for development, and remove
the marketplace afterward if it should not remain configured.

## Runtime and authentication

The bundled skill invokes `npx -y @astrazds/fjgo`. The npm package is scoped;
the command name remains `fjgo`. The launcher downloads the matching native
release, verifies its checksum, and reuses the cached executable on later
runs. Runtime requirements remain Node.js 20 or newer on Linux or macOS with
an x64 or arm64 CPU.

Authentication remains local environment configuration:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

The plugin does not provide OAuth, store Forgejo credentials, or ask users to
paste tokens into prompts. Public operations can work without a token when the
Forgejo server allows them. Private and mutating operations use `FJGO_TOKEN`
with the same token-safe diagnostics, dry runs, and explicit `--yes`
authorization as the standalone CLI.

Ambient session hooks also remain optional. Install them only when persistent
Forgejo context is wanted:

```sh
npx -y @astrazds/fjgo setup hooks --check
npx -y @astrazds/fjgo setup hooks
```

## Maintain the package

Install the pinned development tooling and run the complete gate:

```sh
npm ci
./scripts/verify.sh
go run ./cmd/fjgo-benchmark
```

The Node test suite stages the repository as a local marketplace plugin, asks
the pinned Codex CLI to install it in an isolated configuration, and confirms
that Codex discovers the `fjgo` skill. The plugin-creation reference validator
is an additional compatibility check when that system skill is available.

The plugin manifest, `@astrazds/fjgo` npm package, and `v*` release tag share
one version. Verification and release archive creation reject version drift
before a release is published.

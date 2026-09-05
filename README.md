# fjgo

`fjgo` helps coding agents work with repositories hosted on
[Forgejo](https://forgejo.org/).

It can read and update issues, pull requests, releases, Actions runs,
repository settings, labels, secrets, and more. It also gives agents a safe
way to use the full Forgejo API when there is no shorter command.

## Why use it?

An agent can call the Forgejo API with `curl`, but it has to remember URLs,
JSON shapes, and authentication rules. That creates extra work and makes
mistakes more likely.

`fjgo` gives the agent:

- short commands for common Forgejo jobs;
- compact output that uses fewer tokens;
- clear errors and help;
- safe request previews before making changes;
- automatic protection against printing passwords or API tokens;
- access to every operation in the bundled Forgejo API specification.

Use normal `git` commands for commits, branches, and local files. Use `fjgo`
for tasks that need the Forgejo server.

## Install

### Agent Skill

Install the Agent Skill globally:

```sh
npx skills add https://repos.astrazds.net/astrazds/fjgo.git --skill fjgo -g
```

That is the full setup. You do not need to clone this repository or run
`npm install`.

The skill teaches your agent to run the CLI through `npx -y fjgo`. The first
run downloads the matching `fjgo` release and saves it in a local cache. Later
runs reuse that copy.

Requirements: Node.js 20 or newer, on Linux or macOS with an x64 or arm64 CPU.

Without Node.js, install a native release archive:

```sh
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v1.4.0/fjgo_v1.4.0_linux_amd64.tar.gz
tar -xzf fjgo_v1.4.0_linux_amd64.tar.gz
install -Dm755 fjgo_v1.4.0_linux_amd64/fjgo ~/.local/bin/fjgo
```

Use `darwin` instead of `linux`, and `arm64` instead of `amd64`, when that
matches the machine. Then run `fjgo` directly instead of `npx -y fjgo`.

### Optional ambient hooks

The skill and the native binary are enough for on-demand use. If you want
Forgejo context injected at the start of every agent session, install the
optional hooks after the CLI is on PATH:

```sh
npx -y fjgo setup hooks --check
npx -y fjgo setup hooks
```

You only need the skill or the hooks. Installing both is fine; the hooks add
live session context, and the skill remains available on demand.

### Codex plugin

This repository is also a validated Codex plugin package. A marketplace can
point at the repository root to distribute the existing `fjgo` skill with
plugin presentation metadata and starter prompts. Until a marketplace lists
it, the Agent Skill command above remains the shortest public installation
path.

The plugin still runs `npx -y fjgo`; it does not bundle another API client,
install ambient hooks automatically, provide OAuth, or store credentials.
See [Codex plugin](docs/codex-plugin.md) for marketplace installation, local
testing, authentication, and maintenance details.

## Connect to Forgejo

Set your Forgejo host and access token:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

Create the token in your Forgejo account settings. Give it only the permissions
needed for your task. Do not paste the token into an agent prompt or commit it
to a file.

## First use

Run these commands inside a repository that has a Forgejo Git remote named
`origin`:

```sh
npx -y fjgo -R origin
npx -y fjgo -R origin doctor
npx -y fjgo -R origin issue list --state open
npx -y fjgo -R origin pr list --state open
npx -y fjgo -R origin run list
```

`-R origin` reads the repository owner and name from the Git remote. You can
also choose a repository directly:

```sh
npx -y fjgo --repo OWNER/REPO repo get
npx -y fjgo issue list OWNER/REPO --state open
```

## Make a change safely

Commands that change Forgejo require `--yes`. Preview the request with
`--dry-run` first:

```sh
npx -y fjgo -R origin issue create \
  --title "Fix the login page" \
  --body "The login button is not working." \
  --dry-run --yes
```

If the preview is correct, remove `--dry-run`:

```sh
npx -y fjgo -R origin issue create \
  --title "Fix the login page" \
  --body "The login button is not working." \
  --yes
```

More examples:

```sh
npx -y fjgo -R origin issue view 42 --comments --full
npx -y fjgo -R origin pr checks 12
npx -y fjgo -R origin release list
npx -y fjgo -R origin workflow list
npx -y fjgo -R origin search issues "login" --state open
```

Every command has focused help:

```sh
npx -y fjgo issue --help
npx -y fjgo issue create --help
```

## Learn more

- [Wiki manual](https://repos.astrazds.net/astrazds/fjgo/wiki): concise operator
  guidance published through Forgejo from the reviewed sources in `docs/wiki`.
- [CLI reference](docs/cli-reference.md): authentication, repository selection,
  output, command groups, and the full API escape hatch.
- [Agent setup prompt](docs/agent-setup-prompt.md): a ready-to-paste setup prompt
  for another coding agent.
- [Codex plugin](docs/codex-plugin.md): plugin packaging, marketplace
  installation, authentication boundaries, and validation.
- [Development guide](docs/development.md): build, test, generate code, and make
  releases, including the deterministic offline agent-job benchmark and its
  scenario-run record format. CI runs through `.forgejo/workflows/verify.yml`.
- [AXI compliance](docs/axi-compliance.md): the agent-friendly interface rules
  followed by `fjgo`.
- [Field validation](docs/alpha.md): the v1.4 live-testing checklist.
- [Changelog](CHANGELOG.md): release history.

## License

MIT

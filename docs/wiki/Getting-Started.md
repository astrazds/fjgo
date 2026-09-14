# Getting started

## Choose an installation path

The Agent Skill is the shortest public installation path:

```sh
npx skills add https://github.com/astrazds/fjgo.git --skill fjgo -g
```

The skill teaches an agent to run the released CLI through
`npx -y @astrazds/fjgo`. Node.js 20 or newer is required, on Linux or macOS
with an x64 or arm64 CPU. The npm package is scoped because unscoped `fjgo` is
blocked on the public registry. The installed command name remains `fjgo`.

Without Node.js, install a native release archive:

```sh
curl -LO https://github.com/astrazds/fjgo/releases/download/v1.4.2/fjgo_v1.4.2_linux_amd64.tar.gz
tar -xzf fjgo_v1.4.2_linux_amd64.tar.gz
install -Dm755 fjgo_v1.4.2_linux_amd64/fjgo ~/.local/bin/fjgo
```

Use `darwin` instead of `linux`, and `arm64` instead of `amd64`, when that
matches the machine. Then run `fjgo` directly.

The repository root is also a validated Codex plugin package. A configured
marketplace can expose it as `fjgo`, adding plugin metadata and starter prompts
while reusing the same skill and `npx -y @astrazds/fjgo` runtime. Marketplace
publication is separate from fjgo releases. The plugin does not provide OAuth
or store Forgejo credentials.

## Optional ambient hooks

The skill and the native binary are enough for on-demand use. If you want
Forgejo context injected at the start of every agent session:

```sh
npx -y @astrazds/fjgo setup hooks --check
npx -y @astrazds/fjgo setup hooks
```

You only need the skill or the hooks.

## Connect to Forgejo

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

Create the token in your Forgejo account settings and grant only the
permissions needed for the task. Never paste a real token into an agent prompt
or commit it to a file.

`fjgo` keeps authentication values out of its normal output. Request previews
and diagnostics report whether authentication is present without printing the
token, secret-setting previews redact secret data, and structured API errors
scrub credentials reflected by a server. You should still treat command output
as operational data and review it before sharing.

## Check repository context

Run these commands inside a checkout whose Forgejo remote is named `origin`:

```sh
npx -y @astrazds/fjgo -R origin
npx -y @astrazds/fjgo -R origin doctor --json
npx -y @astrazds/fjgo -R origin issue list --state open
```

`-R origin` resolves the owner and repository from that Git remote. You can
instead pass `--repo OWNER/REPO` explicitly.

## Preview a change

Mutating commands require `--yes`. Use `--dry-run` first so the request can be
checked without sending it:

```sh
npx -y @astrazds/fjgo -R origin issue create \
  --title "Document the next task" \
  --body "Describe the work and acceptance criteria." \
  --dry-run --yes
```

If the preview is correct, remove `--dry-run` to apply it:

```sh
npx -y @astrazds/fjgo -R origin issue create \
  --title "Document the next task" \
  --body "Describe the work and acceptance criteria." \
  --yes
```

See the [CLI reference](https://github.com/astrazds/fjgo/blob/main/docs/cli-reference.md)
for authentication, output formats, command groups, and the generic API escape
hatch.

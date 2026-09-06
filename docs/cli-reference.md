# fjgo CLI reference

This page covers configuration and advanced use. Run the CLI through
`npx -y @astrazds/fjgo` or a native `fjgo` binary. The public npm package is
scoped; the command name remains `fjgo`. For the exact arguments and flags
supported by a command, run:

```sh
npx -y @astrazds/fjgo COMMAND --help
```

## Server and authentication

For a private Forgejo server, set:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

`FJGO_HOST` becomes `https://HOST/api/v1`. Use `--host HOST` for one command,
or `-base-url URL` when the server has a different API path.

Without a configured host, `fjgo` uses the public Forgejo demo API at
`https://v15.next.forgejo.org/api/v1`. It does not send an ambient token to
that demo server.

Basic authentication, one-time passwords, and administrator impersonation are
also supported:

```sh
export FJGO_USERNAME=alice
export FJGO_PASSWORD=your_password
export FJGO_OTP=123456
export FJGO_SUDO=bob
```

The matching flags are `--username`, `--password`, `--otp`, and `--sudo`.
Use only the authentication settings needed by your Forgejo server.

Check the active server and authentication without printing secrets:

```sh
npx -y @astrazds/fjgo auth status
npx -y @astrazds/fjgo doctor --json
```

Tokens, passwords, and one-time passwords are removed from diagnostics, request
previews, and API error messages.

## Choose a repository

There are four clear ways to choose a repository:

```sh
# Read OWNER/REPO from a Forgejo Git remote.
npx -y @astrazds/fjgo -R origin repo get

# Set it for one command.
npx -y @astrazds/fjgo --repo OWNER/REPO issue list
npx -y @astrazds/fjgo issue list --repo OWNER/REPO

# Use the command's positional repository argument.
npx -y @astrazds/fjgo issue list OWNER/REPO

# Set it for the current shell.
export FJGO_REPO=OWNER/REPO
npx -y @astrazds/fjgo repo get
```

An explicit `--repo` or `FJGO_REPO` wins over remote detection. `fjgo` does not
guess repository context from unrelated files.

## Output and safety

Normal output uses TOON, a compact format designed for agents. Keep it as the
default unless a script needs JSON:

```sh
npx -y @astrazds/fjgo doctor --json
npx -y @astrazds/fjgo api --json inspect repoSearch
```

Long text is shortened by default. Add `--full` when the output says it was
truncated. List and detail commands that support `--fields` can return only the
fields you need.

Commands that change server state require `--yes`. Preview them first with
`--dry-run` or `--print-request`:

```sh
npx -y @astrazds/fjgo -R origin repo edit \
  --description "Updated description" \
  --dry-run --yes
```

Errors are structured on standard output. Exit code `2` means the command was
used incorrectly; exit code `1` means another error occurred.

## Timeouts

The root `-timeout DURATION` flag bounds each HTTP request and defaults to 15
seconds. It does not limit the total lifetime of a command that makes multiple
requests.

Long-running commands can have a separate operation timeout. For example,
`run watch` polls for up to 10 minutes by default, while every poll remains
bounded by the root HTTP timeout:

```sh
npx -y @astrazds/fjgo -timeout 15s -R origin run watch 123 --timeout 2m
```

Use the root flag for slow individual requests and the command-local flag for
the overall polling window.

## Main command groups

Common work is organized by topic:

- `repo`: repository details, creation, settings, branches, collaborators,
  branch protection, topics, and avatars.
- `issue`: list, view, create, edit, comment, close, pin, dependencies,
  reactions, deadlines, and tracked time.
- `pr`: list, view, edit, comment, review, update, merge, diff, and checks.
- `run` and `workflow`: inspect Forgejo Actions and start workflows.
- `release`: create releases and manage release assets.
- `search`: search issues or repositories.
- `label`, `secret`, and `variable`: manage repository metadata and Actions
  configuration.
- `auth` and `doctor`: check server, repository, and authentication context.

Examples:

```sh
npx -y @astrazds/fjgo -R origin repo branches list
npx -y @astrazds/fjgo -R origin issue view 42 --comments --full
npx -y @astrazds/fjgo -R origin pr view 12 --reviews
npx -y @astrazds/fjgo -R origin run view 123 --log-failed
npx -y @astrazds/fjgo -R origin workflow run verify.yml --ref main --dry-run --yes
npx -y @astrazds/fjgo -R origin release latest
```

## Full API access

The short commands cover common jobs. The `api` command covers every operation
in the bundled Forgejo Swagger specification.

Find and inspect an operation before calling it:

```sh
npx -y @astrazds/fjgo api list release
npx -y @astrazds/fjgo api inspect repoSearch
npx -y @astrazds/fjgo model inspect CreateRepoOption
```

Call an operation by its Swagger operation ID:

```sh
npx -y @astrazds/fjgo api call repoSearch q=fjgo limit=10
npx -y @astrazds/fjgo api call repoGet owner=OWNER repo=REPO
npx -y @astrazds/fjgo api call createCurrentUserRepo \
  -body '{"name":"demo","private":true}' \
  --dry-run --yes
```

Use `api raw` when a direct method and path are simpler:

```sh
npx -y @astrazds/fjgo api raw GET /repos/OWNER/REPO
npx -y @astrazds/fjgo api raw PATCH /repos/OWNER/REPO \
  -body '{"description":"updated"}' \
  --dry-run --yes
```

Use `api upload` for multipart file uploads. Use `--output PATH` for binary
downloads. Run each subcommand with `--help` for its body, query, upload, and
response options.

## Generated aliases

`fjgo` creates readable aliases for API paths that map cleanly to commands:

```sh
npx -y @astrazds/fjgo alias list
npx -y @astrazds/fjgo alias inspect repo issues get
npx -y @astrazds/fjgo alias collisions
npx -y @astrazds/fjgo alias omissions
```

`alias omissions` explains why an API operation has no alias and shows the
`api call` or `api upload` command to use instead.

## Optional session hooks

The Agent Skill loads only when needed. If you want every agent session to
start with compact Forgejo context, install the optional hooks:

```sh
npx -y @astrazds/fjgo setup hooks --check
npx -y @astrazds/fjgo setup hooks
```

This configures managed integrations for Claude Code, Codex, and OpenCode.
Restart the agent after installation.

# fjgo Alpha Field Test

Use this packet to test `fjgo` as a coding-agent-first Forgejo CLI plus skill.

## Install

```sh
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.12.0/fjgo_v0.12.0_linux_amd64.tar.gz
tar -xzf fjgo_v0.12.0_linux_amd64.tar.gz
install -Dm755 fjgo_v0.12.0_linux_amd64/fjgo ~/.local/bin/fjgo
fjgo skill install --force
```

For private or non-demo Forgejo instances:

```sh
export FJGO_BASE_URL=https://forgejo.example.com/api/v1
export FJGO_TOKEN=your_access_token
```

## First Check

Run this inside a Forgejo-backed checkout:

```sh
fjgo -R origin doctor --json
fjgo skill status
```

If `skill status` reports missing files, reinstall with:

```sh
fjgo skill install --force
```

## Field Tasks

1. Inspect repo context and open work:

```sh
fjgo -R origin doctor --json
fjgo -R origin repo issues list state=open
fjgo -R origin repo pulls list state=open
```

2. Create, comment on, and close a disposable issue:

```sh
fjgo -R origin repo issues create --dry-run --yes -body '{"title":"fjgo alpha test","body":"Created during fjgo alpha testing."}'
fjgo -R origin repo issues create --yes -body '{"title":"fjgo alpha test","body":"Created during fjgo alpha testing."}'
fjgo -R origin repo issue comment ISSUE_NUMBER --dry-run --yes -body '{"body":"Alpha test comment."}'
fjgo -R origin repo issue comment ISSUE_NUMBER --yes -body '{"body":"Alpha test comment."}'
fjgo -R origin repo issue close ISSUE_NUMBER --dry-run --yes
fjgo -R origin repo issue close ISSUE_NUMBER --yes
```

3. Inspect pull request state:

```sh
fjgo -R origin repo pulls list state=open
fjgo -R origin repo pulls get PR_NUMBER
fjgo -R origin repo pulls files get PR_NUMBER
```

4. Preview repo metadata changes without mutating:

```sh
fjgo -R origin repo topics
fjgo -R origin repo topics --set forgejo,go,cli --dry-run --yes
fjgo -R origin repo avatar assets/icon.png --dry-run --yes
```

5. Validate release workflows:

```sh
fjgo -R origin release list
fjgo -R origin release create v0.0.0-alpha-test --dry-run --yes body="Alpha test release preview."
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
```

## Optional Write Smoke

Use only with a disposable repo:

```sh
FJGO_BASE_URL=https://forgejo.example.com/api/v1 FJGO_TEST_REPO=owner/repo FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

The smoke creates an issue, comments on it, closes it, and exercises token-safe
dry-run previews.

## Failure Report

Paste these into the feedback issue:

```sh
fjgo --version
fjgo -R origin doctor --json
fjgo -R origin auth status
```

Also include:

- exact command that failed
- full stdout/stderr
- expected result
- whether the command was run by a human or a coding agent

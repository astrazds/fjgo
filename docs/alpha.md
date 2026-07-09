# fjgo Alpha Field Test

Use this packet to test `fjgo` as an AXI-style Forgejo CLI with TOON stdout,
managed agent session hooks, and an installable skill.

For a fresh agent, paste `docs/agent-setup-prompt.md` into the agent while it is
inside a Forgejo-backed checkout.

## Install

```sh
curl -LO https://repos.astrazds.net/astrazds/fjgo/releases/download/v0.16.0/fjgo_v0.16.0_linux_amd64.tar.gz
tar -xzf fjgo_v0.16.0_linux_amd64.tar.gz
install -Dm755 fjgo_v0.16.0_linux_amd64/fjgo ~/.local/bin/fjgo
fjgo setup hooks --check
fjgo setup hooks
fjgo skill install --force
fjgo skill generate --check
```

Set the API base to the same Forgejo host as the checkout remote before using
`-R origin`. Add a token for private repos or write tasks:

```sh
export FJGO_HOST=forgejo.example.com
export FJGO_TOKEN=your_access_token
```

## First Check

Run this inside a Forgejo-backed checkout after setting `FJGO_HOST`:

```sh
fjgo -R origin doctor --json
fjgo -R origin
fjgo skill status
fjgo update --check
```

If there is no checkout remote, use explicit repo context instead:

```sh
export FJGO_REPO=owner/repo
fjgo doctor --json
fjgo repo get
```

If `skill status` reports missing or outdated files, reinstall with:

```sh
fjgo skill install --force
```

## Field Tasks

1. Inspect repo context and open work:

```sh
fjgo -R origin doctor --json
fjgo -R origin doctor
fjgo -R origin issue list --state open
fjgo -R origin pr list --state open
fjgo -R origin run list
```

2. Create, comment on, and close a disposable issue:

```sh
fjgo -R origin issue create --title "fjgo alpha test" --body "Created during fjgo alpha testing." --dry-run --yes
fjgo -R origin issue create --title "fjgo alpha test" --body "Created during fjgo alpha testing." --yes
fjgo -R origin issue comment ISSUE_NUMBER --body "Alpha test comment." --dry-run --yes
fjgo -R origin issue comment ISSUE_NUMBER --body "Alpha test comment." --yes
fjgo -R origin issue close ISSUE_NUMBER --dry-run --yes
fjgo -R origin issue close ISSUE_NUMBER --yes
```

3. Inspect pull request state:

```sh
fjgo -R origin pr list --state open
fjgo -R origin pr view PR_NUMBER --reviews
fjgo -R origin pr edit PR_NUMBER --title "Updated title" --dry-run --yes
fjgo -R origin pr close PR_NUMBER --dry-run --yes
fjgo -R origin pr comment PR_NUMBER --body "Alpha PR note." --dry-run --yes
fjgo -R origin pr files PR_NUMBER
fjgo -R origin pr checks PR_NUMBER
```

4. Preview repo metadata changes without mutating:

```sh
fjgo -R origin repo topics
fjgo -R origin repo topics --set forgejo,go,cli --dry-run --yes
fjgo -R origin repo avatar assets/icon.png --dry-run --yes
fjgo -R origin repo branches list
fjgo -R origin repo collaborators list
fjgo -R origin repo branch-protection list
fjgo -R origin repo edit --description "fjgo alpha preview" --dry-run --yes
fjgo repo create fjgo-alpha-preview --private --dry-run --yes
```

5. Validate release workflows:

```sh
fjgo -R origin release list
fjgo -R origin release latest
fjgo -R origin release create v0.0.0-alpha-test --body "Alpha test release preview." --dry-run --yes
fjgo -R origin release create v0.0.0-alpha-test --notes-file notes.md --dry-run --yes
fjgo -R origin release edit RELEASE_ID --body-file notes.md --dry-run --yes
fjgo -R origin release assets list RELEASE_ID
fjgo -R origin release assets delete RELEASE_ID ASSET_ID --dry-run --yes
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
```

6. Preview advanced issue and PR workflows:

```sh
fjgo -R origin issue pinned
fjgo -R origin issue pin ISSUE_NUMBER --dry-run --yes
fjgo -R origin issue dependencies list ISSUE_NUMBER
fjgo -R origin issue dependencies add ISSUE_NUMBER OTHER_ISSUE --dry-run --yes
fjgo -R origin issue reactions add ISSUE_NUMBER +1 --dry-run --yes
fjgo -R origin issue deadline set ISSUE_NUMBER 2026-08-01 --dry-run --yes
fjgo -R origin issue time add ISSUE_NUMBER --seconds 900 --dry-run --yes
fjgo -R origin pr reviews PR_NUMBER
fjgo -R origin pr review-requests add PR_NUMBER --reviewer USER --dry-run --yes
fjgo -R origin pr diff PR_NUMBER
fjgo -R origin pr update PR_NUMBER --style rebase --dry-run --yes
```

7. Inspect Actions and workflow files:

```sh
fjgo -R origin run list --status failure
fjgo -R origin run view RUN_ID --log-failed
fjgo -R origin run watch RUN_ID --timeout 2m
fjgo -R origin workflow list
fjgo -R origin workflow view verify.yml --full
fjgo -R origin workflow run verify.yml --ref main --dry-run --yes
```

8. Exercise raw API fallback only when no curated command or generated operation
fits:

```sh
fjgo api raw GET /repos/OWNER/REPO
fjgo api raw PATCH /repos/OWNER/REPO --dry-run --yes -body '{"description":"updated"}'
```

## Optional Write Smoke

This creates a unique private repo, runs the write smoke inside it, and deletes
the repo on exit or failure:

```sh
FJGO_HOST=forgejo.example.com FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

Set `FJGO_TEST_ORG=org` to create the temporary repo in an organization. Set
`FJGO_SMOKE_AUTH=1` to include this smoke in `./scripts/verify.sh`.

## Failure Report

Paste these into the feedback issue:

```sh
fjgo --version
fjgo -R origin doctor --json
fjgo -R origin auth status
```

Also include:

- exact command that failed
- full stdout/stderr; stdout is structured TOON unless `--json` was used
- expected result
- whether the command was run by a human or a coding agent

Diagnostics are intended to be shareable: token presence may be shown, but token
values are redacted from `doctor`, `auth status`, dry runs, request previews,
structured errors, root `--json` errors, and Forgejo API error text.

# fjgo Workflows

## Orientation

```sh
export FJGO_HOST=forgejo.example.com
fjgo -R origin
fjgo --repo OWNER/REPO
fjgo issue list --repo OWNER/REPO --state open
fjgo -R origin doctor
fjgo -R origin doctor --json
fjgo auth status
fjgo skill status
fjgo -R origin repo get
fjgo -R origin issue list --state open
fjgo -R origin pr list
fjgo -R origin run list
fjgo search issues "bug" --repo OWNER/REPO
fjgo api raw GET /repos/OWNER/REPO
fjgo model inspect CreateIssueOption
```

Use `doctor --json` output as the field-feedback artifact when a workflow needs
raw JSON. Normal command output is TOON. Diagnostics are redacted: token value is
never printed, including when a Forgejo API error body reflects the configured
token.

Use `--repo OWNER/REPO` or `FJGO_REPO=OWNER/REPO` when there is no suitable
Forgejo git remote. Do not infer repo context from unrelated local files.

## Ambient Setup

```sh
fjgo setup hooks --check
fjgo setup hooks
fjgo skill install --force
```

Hooks are the ambient path for Claude Code, Codex, and OpenCode. The skill is
the on-demand path for agents that support skills or projects that do not want
per-session context.

## Triage Issue

Inspect the issue, related comments, labels, and current repo state before
changing anything:

```sh
fjgo -R origin issue list --state open
fjgo -R origin issue view ISSUE_NUMBER --comments --full
fjgo -R origin label list
```

Comment or close only after a dry run:

```sh
fjgo -R origin issue comment ISSUE_NUMBER --body "Triage note." --dry-run --yes
fjgo -R origin issue close ISSUE_NUMBER --dry-run --yes
fjgo -R origin issue pin ISSUE_NUMBER --dry-run --yes
fjgo -R origin issue reactions add ISSUE_NUMBER +1 --dry-run --yes
fjgo -R origin issue deadline set ISSUE_NUMBER 2026-08-01 --dry-run --yes
```

Inspect or update related issue workflow state:

```sh
fjgo -R origin issue dependencies list ISSUE_NUMBER
fjgo -R origin issue dependencies add ISSUE_NUMBER BLOCKING_ISSUE --dry-run --yes
fjgo -R origin issue time list ISSUE_NUMBER
fjgo -R origin issue time add ISSUE_NUMBER --seconds 900 --dry-run --yes
```

## Open PR Summary

List open pull requests, inspect the target PR, and fetch changed files:

```sh
fjgo -R origin pr list --state open
fjgo -R origin pr view PR_NUMBER --full
fjgo -R origin pr view PR_NUMBER --reviews
fjgo -R origin pr edit PR_NUMBER --title "Updated title" --dry-run --yes
fjgo -R origin pr close PR_NUMBER --dry-run --yes
fjgo -R origin pr comment PR_NUMBER --body "Review note." --dry-run --yes
fjgo -R origin pr files PR_NUMBER
fjgo -R origin pr commits PR_NUMBER
fjgo -R origin pr checks PR_NUMBER
fjgo -R origin pr reviews PR_NUMBER
fjgo -R origin pr diff PR_NUMBER
fjgo -R origin pr patch PR_NUMBER
fjgo -R origin pr update PR_NUMBER --style rebase --dry-run --yes
fjgo -R origin pr review-requests add PR_NUMBER --reviewer USER --dry-run --yes
```

## Publish Release

Create and upload release assets through the task alias. Always dry-run first:

```sh
fjgo -R origin release list
fjgo -R origin release latest
fjgo -R origin release view v1.3.0
fjgo -R origin release create v1.3.0 --body-file notes.md --dry-run --yes
fjgo -R origin release create v1.3.0 --notes-file notes.md --dry-run --yes
fjgo -R origin release create v1.3.0 --body-file notes.md --yes
fjgo -R origin release edit RELEASE_ID --body-file notes.md --dry-run --yes
fjgo -R origin release assets list RELEASE_ID
fjgo -R origin release assets delete RELEASE_ID ASSET_ID --dry-run --yes
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --yes
```

## Update Repo Metadata

Preview metadata changes before mutating:

```sh
fjgo -R origin repo get
fjgo -R origin repo branches list
fjgo -R origin repo collaborators list
fjgo -R origin repo branch-protection list
fjgo -R origin repo edit --description "Updated description" --dry-run --yes
fjgo repo create demo --private --dry-run --yes
fjgo -R origin repo fork --name demo-fork --dry-run --yes
fjgo -R origin repo topics
fjgo -R origin repo topics --set forgejo,go,cli --dry-run --yes
fjgo -R origin repo avatar assets/icon.png --dry-run --yes
```

Use generated aliases or `api call` for less common repository edits:

```sh
fjgo alias inspect repo edit
fjgo model inspect EditRepoOption
```

## Inspect Failing Action

```sh
fjgo -R origin run list --status failure
fjgo -R origin run view RUN_ID
fjgo -R origin run view RUN_ID --log-failed
fjgo -R origin run watch RUN_ID --timeout 2m
fjgo -R origin workflow list
fjgo -R origin workflow view verify.yml --full
fjgo -R origin workflow run verify.yml --ref main --dry-run --yes
```

The root `-timeout` bounds each HTTP request (15 seconds by default), while
`run watch --timeout` controls the overall polling window. A watch can therefore
run longer than the HTTP timeout without leaving any individual poll unbounded.

## Read And Write Repo Contents

Read contents through generated content aliases:

```sh
fjgo alias list contents
fjgo alias inspect repo contents get
fjgo -R origin repo contents get README.md ref=main
```

For write bodies, inspect the operation and model:

```sh
fjgo api inspect repoCreateFile
fjgo api inspect repoUpdateFile
fjgo model inspect CreateFileOptions
fjgo model inspect UpdateFileOptions
fjgo -R origin repo contents update README.md --dry-run --yes -body '{"message":"update README","content":"BASE64_CONTENT","branch":"main","sha":"CURRENT_FILE_SHA"}'
```

## Create Issue

```sh
fjgo -R origin issue create --title "Title" --body "Body" --dry-run --yes
fjgo -R origin issue create --title "Title" --body "Body" --yes
```

## Labels, Secrets, And Variables

```sh
fjgo -R origin label list
fjgo -R origin label create --name bug --color ff0000 --dry-run --yes
echo -n "$TOKEN" | fjgo -R origin secret set DEPLOY_TOKEN --dry-run --yes
fjgo -R origin variable set BUILD_MODE --body release --dry-run --yes
```

## Optional Auth Smoke

When field-testing authenticated writes, the smoke creates a unique private repo,
runs inside it, and deletes the repo on exit or failure:

```sh
FJGO_HOST=forgejo.example.com FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

Set `FJGO_TEST_ORG=org` to create the temporary repo in an organization. Set
`FJGO_SMOKE_AUTH=1` to include this smoke in `./scripts/verify.sh`.

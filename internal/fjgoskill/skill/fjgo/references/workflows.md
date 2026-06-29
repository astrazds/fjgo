# fjgo Workflows

## Orientation

```sh
fjgo -R origin doctor --json
fjgo auth status
fjgo skill status
fjgo -R origin repo get
fjgo alias inspect repo issues list
fjgo api inspect issueCreateIssue
fjgo model inspect CreateIssueOption
```

Use `doctor --json` output as the field-feedback artifact when a workflow fails.
It is redacted: token value is never printed.

## Triage Issue

Inspect the issue, related comments, labels, and current repo state before
changing anything:

```sh
fjgo -R origin repo issues list state=open type=issues
fjgo -R origin repo issues get ISSUE_NUMBER
fjgo -R origin repo issues timeline get ISSUE_NUMBER
fjgo -R origin repo labels list
```

Comment or close only after a dry run:

```sh
fjgo -R origin repo issue comment ISSUE_NUMBER --dry-run --yes -body '{"body":"Triage note."}'
fjgo -R origin repo issue close ISSUE_NUMBER --dry-run --yes
```

## Open PR Summary

List open pull requests, inspect the target PR, and fetch changed files:

```sh
fjgo -R origin repo pulls list state=open
fjgo -R origin repo pulls get PR_NUMBER
fjgo -R origin repo pulls files get PR_NUMBER
fjgo -R origin repo pulls PR_NUMBER commits get
```

## Publish Release

Create and upload release assets through the task alias. Always dry-run first:

```sh
fjgo -R origin release list
fjgo -R origin release create v1.0.0 body="Release notes." --dry-run --yes
fjgo -R origin release create v1.0.0 body="Release notes." --yes
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
fjgo -R origin release upload RELEASE_ID dist/fjgo.tar.gz name=fjgo.tar.gz --yes
```

## Update Repo Metadata

Preview metadata changes before mutating:

```sh
fjgo -R origin repo get
fjgo -R origin repo topics
fjgo -R origin repo topics --set forgejo,go,cli --dry-run --yes
fjgo -R origin repo avatar assets/icon.png --dry-run --yes
```

Use generic aliases for less common repository edits:

```sh
fjgo alias inspect repo edit
fjgo model inspect EditRepoOption
```

## Inspect Failing Action

```sh
fjgo alias list actions
fjgo alias inspect repo actions runs list
fjgo -R origin repo actions runs list
fjgo -R origin repo actions runs get RUN_ID
fjgo -R origin repo actions tasks list
```

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
fjgo -R origin alias inspect repo issues create
fjgo -R origin repo issues create --dry-run --yes -body '{"title":"Title","body":"Body"}'
fjgo -R origin repo issues create --yes -body '{"title":"Title","body":"Body"}'
```

## Optional Auth Smoke

When field-testing with a disposable repo:

```sh
FJGO_BASE_URL=https://forgejo.example.com/api/v1 FJGO_TEST_REPO=owner/repo FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

The smoke creates a test issue, comments on it, closes it, and exercises
token-safe dry-run previews for release upload, topics, and avatar update.

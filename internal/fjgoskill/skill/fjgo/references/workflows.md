# fjgo Workflows

## Orientation

```sh
fjgo -R origin doctor --json
fjgo auth status
fjgo -R origin repo get
fjgo alias inspect repo issues list
fjgo api inspect issueCreateIssue
fjgo model inspect CreateIssueOption
```

Use `doctor --json` output as the field-feedback artifact when a workflow fails.
It is redacted: token value is never printed.

## Issues

List open issues:

```sh
fjgo -R origin repo issues list state=open type=issues
```

Create an issue:

```sh
fjgo -R origin alias inspect repo issues create
fjgo -R origin repo issues create --dry-run --yes -body '{"title":"Title","body":"Body"}'
fjgo -R origin repo issues create --yes -body '{"title":"Title","body":"Body"}'
```

Comment and close:

```sh
fjgo -R origin repo issue comment 123 --dry-run --yes -body '{"body":"Comment"}'
fjgo -R origin repo issue comment 123 --yes -body '{"body":"Comment"}'
fjgo -R origin repo issue close 123 --dry-run --yes
fjgo -R origin repo issue close 123 --yes
```

## Pull Requests

List and inspect pull requests:

```sh
fjgo -R origin repo pulls list state=open
fjgo -R origin repo pulls get 7
fjgo -R origin repo pulls files get 7
```

Inspect before PR mutations:

```sh
fjgo alias inspect repo pulls create
fjgo model inspect CreatePullRequestOption
fjgo -R origin repo pulls create --dry-run --yes -body '{"head":"branch","base":"main","title":"Title"}'
```

## Releases And Attachments

```sh
fjgo -R origin release list
fjgo -R origin api inspect repoCreateRelease
fjgo -R origin api call repoCreateRelease --dry-run --yes owner=OWNER repo=REPO -body '{"tag_name":"v1.0.0","name":"v1.0.0"}'
fjgo -R origin release upload 123 dist/fjgo.tar.gz name=fjgo.tar.gz --dry-run --yes
```

## Repository Metadata

```sh
fjgo -R origin repo get
fjgo -R origin repo topics
fjgo -R origin repo topics --set forgejo,go,cli --dry-run --yes
fjgo -R origin repo avatar assets/icon.png --dry-run --yes
```

## Files And Contents

Use the generated content aliases first:

```sh
fjgo alias list contents
fjgo alias inspect repo contents get
fjgo -R origin repo contents get README.md ref=main
```

For write bodies, inspect the operation and model:

```sh
fjgo api inspect repoCreateFile
fjgo model inspect CreateFileOptions
```

## Optional Auth Smoke

When field-testing with a disposable repo:

```sh
FJGO_BASE_URL=https://forgejo.example.com/api/v1 FJGO_TEST_REPO=owner/repo FJGO_TOKEN=... ./scripts/smoke-auth.sh
```

The smoke creates a test issue, comments on it, closes it, and exercises
token-safe dry-run previews for release upload, topics, and avatar update.

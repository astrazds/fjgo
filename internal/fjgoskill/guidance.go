package fjgoskill

import (
	"fmt"
	"strings"
)

const (
	guidanceStart = "<!-- fjgo:static-guidance:start -->"
	guidanceEnd   = "<!-- fjgo:static-guidance:end -->"
)

func StaticGuidance(commandPrefix string) string {
	return `Use fjgo for Forgejo-specific API work and normal git for local branch, commit, diff, and remote operations.

First commands:
  ` + commandPrefix + ` --repo OWNER/REPO
  ` + commandPrefix + ` --repo OWNER/REPO doctor
  ` + commandPrefix + ` --repo OWNER/REPO repo get
  ` + commandPrefix + ` --repo OWNER/REPO issue list --state open
  ` + commandPrefix + ` --repo OWNER/REPO pr list
  ` + commandPrefix + ` --repo OWNER/REPO run list

Safe mutation pattern:
  inspect the alias or operation first
  run with --dry-run --yes
  rerun without --dry-run only after target repo, path, and body are correct

Useful surfaces:
  ` + commandPrefix + ` setup hooks
  ` + commandPrefix + ` skill install --force
  ` + commandPrefix + ` auth status
  ` + commandPrefix + ` api raw GET /repos/OWNER/REPO
  ` + commandPrefix + ` issue view OWNER/REPO 1 --full
  ` + commandPrefix + ` pr view OWNER/REPO 1 --reviews
  ` + commandPrefix + ` pr checks OWNER/REPO 1
  ` + commandPrefix + ` search issues "bug" --repo OWNER/REPO
  ` + commandPrefix + ` release list OWNER/REPO
  ` + commandPrefix + ` release assets list OWNER/REPO 1
  ` + commandPrefix + ` update --check
  ` + commandPrefix + ` model inspect CreateIssueOption`
}

func StaticGuidanceBlock(commandPrefix string) string {
	return guidanceStart + "\n" + StaticGuidance(commandPrefix) + "\n" + guidanceEnd
}

func GeneratedSkillMarkdown(commandPrefix string) string {
	return strings.TrimSpace(`---
name: fjgo
description: Forgejo repository operations through the fjgo CLI for coding agents. Use when Codex needs to inspect, triage, modify, release, or report on Forgejo repos; work with Forgejo issues, pulls, releases, repository metadata, files, labels, topics, or attachments; verify auth/repo context; or produce token-safe dry-run plans before mutating Forgejo state.
---

# fjgo

Use `+"`"+commandPrefix+"`"+` for Forgejo-specific work and normal `+"`git`"+` for local commit, branch,
diff, and remote operations. `+"`"+commandPrefix+"`"+` is AXI-shaped: stdout is compact TOON by
default, errors are structured on stdout, mutating commands require `+"`--yes`"+`, and
dry runs/request previews are token-safe.

## First Command

Set `+"`FJGO_HOST`"+` to the same Forgejo host as the checkout remote,
then run this before acting on a repo:

`+"```sh"+`
export FJGO_HOST=forgejo.example.com
`+commandPrefix+` -R origin
`+commandPrefix+` -R origin doctor
`+commandPrefix+` -R origin issue list --state open
`+commandPrefix+` -R origin pr list
`+commandPrefix+` -R origin run list
`+"```"+`

If there is no git remote, pass `+"`owner/repo`"+` to repo-scoped commands directly,
use root or command-local `+"`--repo OWNER/REPO`"+`, or export `+"`FJGO_REPO=OWNER/REPO`"+`.
For private instances or write tasks, rely on `+"`FJGO_TOKEN`"+` from the environment.
Never print or ask to expose the token. `+"`doctor`"+`, `+"`auth status`"+`, dry runs, and
request previews do not print token values. API error diagnostics are also
token-safe, including server-reflected token text.

Use `+"`--json`"+` only when a command explicitly supports it and raw JSON is needed,
for example `+"`"+commandPrefix+" doctor --json`"+` or `+"`"+commandPrefix+" api --json call getVersion`"+`.

## Setup Paths

Recommend ambient hooks first when the user wants persistent Forgejo context:

`+"```sh"+`
`+commandPrefix+` setup hooks --check
`+commandPrefix+` setup hooks
`+"```"+`

This installs managed SessionStart hooks for Claude Code and Codex plus an
OpenCode plugin. Restart the agent session after installing hooks. The skill is
the secondary on-demand path and can be installed with:

`+"```sh"+`
`+commandPrefix+` skill install --force
`+"```"+`

## Safe Defaults

- Use `+"`--dry-run`"+` or `+"`--print-request`"+` before mutating commands when planning.
- Use `+"`--yes`"+` only after the target repo, operation, body, and path are clear.
- Prefer curated workflow commands first: `+"`issue`"+`, `+"`pr`"+`, `+"`run`"+`, `+"`workflow`"+`,
  `+"`search`"+`, `+"`repo`"+`, `+"`label`"+`, `+"`secret`"+`, `+"`variable`"+`, and `+"`release`"+`.
- Use command-local `+"`--repo OWNER/REPO`"+` and `+"`--host HOST`"+` when it keeps the command
  self-contained for another agent or transcript.
- Use repo lifecycle commands for list/create/edit/fork, branches,
  collaborators, and branch protection before falling back to generated aliases.
- Use release commands for view/latest/edit/delete, asset list/delete, upload,
  and `+"`--notes-file`"+` notes.
- Use issue/PR advanced commands for pins, dependencies, reactions, deadlines,
  tracked time, PR edit/close/reopen/comment, review requests/comments,
  PR update, diff, and patch.
- Use workflow/run commands for workflow list/view/run and run list/view/watch.
- Use `+"`alias inspect`"+` and `+"`api inspect`"+` before guessing generated arguments or
  bodies.
- Use `+"`alias omissions`"+` to see why an operation has no generated alias and the
  exact generic command that replaces it.
- Fall back to generated aliases, `+"`api call`"+`, `+"`api upload`"+`, or `+"`api raw`"+` when no curated
  command exists.
- Use `+"`--include-response`"+` on buffered generic calls when status or response
  headers are needed; use `+"`--raw`"+` or `+"`--output`"+` for binary payloads.
- Use `+"`model inspect <Model>`"+` to build JSON bodies from required fields.
- Use `+"`--full`"+` when TOON output reports truncated long text.
- Use `+"`--fields`"+` on list/detail workflow commands to request only needed
  fields; unknown fields fail with the valid field list.

`+StaticGuidanceBlock(commandPrefix)+`

## References

Read [references/workflows.md](references/workflows.md) for issue, pull request,
release, repository metadata, file/content, field feedback, and auth-smoke
recipes.

For first-time setup of another agent, use the repository prompt at
`+"`docs/agent-setup-prompt.md`"+`.
`) + "\n"
}

func GeneratedNpxSkillMarkdown() string {
	markdown := GeneratedSkillMarkdown("npx -y @astrazds/fjgo")
	setupStart := strings.Index(markdown, "\n## Setup Paths\n")
	safeDefaults := strings.Index(markdown, "\n## Safe Defaults\n")
	if setupStart >= 0 && safeDefaults > setupStart {
		setup := `
## Optional Ambient Hooks

The installed skill is the complete on-demand setup. Only when the user asks
for persistent Forgejo context, offer the optional ambient hooks:

` + "```sh" + `
npx -y @astrazds/fjgo setup hooks --check
npx -y @astrazds/fjgo setup hooks
` + "```" + `

This installs managed SessionStart hooks for Claude Code and Codex plus an
OpenCode plugin. Restart the agent session after installing hooks.
`
		markdown = markdown[:setupStart] + setup + markdown[safeDefaults:]
	}
	markdown = strings.ReplaceAll(markdown, "  npx -y @astrazds/fjgo skill install --force\n", "")
	if index := strings.Index(markdown, "\n## References\n"); index >= 0 {
		markdown = markdown[:index]
	}
	return strings.TrimSpace(markdown) + "\n"
}

func EmbeddedGuidanceCurrent() (bool, string, error) {
	b, err := embedded.ReadFile("skill/fjgo/SKILL.md")
	if err != nil {
		return false, "", err
	}
	got, err := extractGuidanceBlock(string(b))
	if err != nil {
		return false, "", err
	}
	want := StaticGuidanceBlock("fjgo")
	return got == want, want, nil
}

func EmbeddedSkillCurrent() (bool, string, error) {
	b, err := embedded.ReadFile("skill/fjgo/SKILL.md")
	if err != nil {
		return false, "", err
	}
	want := GeneratedSkillMarkdown("fjgo")
	return string(b) == want, want, nil
}

func extractGuidanceBlock(text string) (string, error) {
	start := strings.Index(text, guidanceStart)
	if start < 0 {
		return "", fmt.Errorf("missing %s", guidanceStart)
	}
	afterStart := start + len(guidanceStart)
	endRel := strings.Index(text[afterStart:], guidanceEnd)
	if endRel < 0 {
		return "", fmt.Errorf("missing %s", guidanceEnd)
	}
	end := afterStart + endRel + len(guidanceEnd)
	return strings.TrimSpace(text[start:end]), nil
}

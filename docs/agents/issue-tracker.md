# Issue tracker: Forgejo

Issues and PRDs for this repo live as Forgejo issues at `repos.astrazds.net/astrazds/fjgo`. Use this repository's `fjgo` CLI for tracker operations.

## Conventions

Run commands inside this clone and use `-R origin` to derive the repository from its Forgejo remote.

- **Create an issue**: `fjgo -R origin issue create --title "..." --body-file issue.md --yes`
- **Read an issue**: `fjgo -R origin issue view <number> --comments --full`
- **List issues**: `fjgo -R origin issue list --state open`, with `--labels`, `--assignee`, or `--json` when needed
- **Comment on an issue**: `fjgo -R origin issue comment <number> --body-file comment.md --yes`
- **Edit an issue**: `fjgo -R origin issue edit <number> ... --yes`
- **Close an issue**: `fjgo -R origin issue close <number> --yes`
- **Reopen an issue**: `fjgo -R origin issue reopen <number> --yes`
- **Preview mutations**: add `--dry-run --yes` or `--print-request --yes`

Use `fjgo api inspect <operation>` and `fjgo api call <operation>` when a tracker operation lacks a curated command. Relevant label operations include `issueAddLabel`, `issueRemoveLabel`, `issueReplaceLabels`, and `issueCreateLabel`.

## Pull requests as a triage surface

**PRs as a request surface: no.**

Pull requests are not included in the issue triage queue unless this flag is changed to `yes`.

## When a skill says “publish to the issue tracker”

Create a Forgejo issue with `fjgo -R origin issue create`.

## When a skill says “fetch the relevant ticket”

Run `fjgo -R origin issue view <number> --comments --full`.

## Wayfinding operations

The **map** is one Forgejo issue with linked child issues as tickets.

- **Map**: an issue labelled `wayfinder:map`, holding Notes, Decisions-so-far, and Fog
- **Child ticket**: an issue linked from the map and labelled `wayfinder:<type>` (`research`, `prototype`, `grilling`, or `task`)
- **Blocking**: use Forgejo issue dependencies through `fjgo issue dependencies` and `fjgo issue blocks`
- **Frontier**: inspect the map's open child issues and select the first unblocked, unclaimed issue in map order
- **Claim**: assign the issue to the driving developer before beginning work
- **Resolve**: comment with the answer, close the child issue, and add a context pointer to the map's Decisions-so-far

# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues at `github.com/astrazds/fjgo`. Use `gh` for tracker operations.

This project's git remote is GitHub. Do not use `fjgo -R origin` against this clone. `fjgo` remains the Forgejo API client for Forgejo remotes and hosts.

## Conventions

Run commands inside this clone. `gh` uses `github.com/astrazds/fjgo` from the `origin` remote.

- **Create an issue**: `gh issue create --title "..." --body-file issue.md`
- **Read an issue**: `gh issue view <number> --comments`
- **List issues**: `gh issue list --state open`, with `--label`, `--assignee`, or `--json` when needed
- **Comment on an issue**: `gh issue comment <number> --body-file comment.md`
- **Edit an issue**: `gh issue edit <number> ...`
- **Close an issue**: `gh issue close <number>`
- **Reopen an issue**: `gh issue reopen <number>`
- **Preview mutations**: `gh` has no `--dry-run`. Draft the body locally, then create or comment.

Use `gh api` when a tracker operation lacks a curated command. Label operations include `gh issue edit <number> --add-label` and `gh label create`.

## Pull requests as a triage surface

**PRs as a request surface: no.**

Pull requests are not included in the issue triage queue unless this flag is changed to `yes`.

## When a skill says “publish to the issue tracker”

Create a GitHub issue with `gh issue create`.

## When a skill says “fetch the relevant ticket”

Run `gh issue view <number> --comments`.

## Wayfinding operations

The **map** is one GitHub issue with linked child issues as tickets.

- **Map**: an issue labelled `wayfinder:map`, holding Notes, Decisions-so-far, and Fog
- **Child ticket**: an issue linked from the map and labelled `wayfinder:<type>` (`research`, `prototype`, `grilling`, or `task`)
- **Blocking**: mention the blocking issue in the child body (`Blocked by #N`) and keep that link current
- **Frontier**: inspect the map's open child issues and select the first unblocked, unclaimed issue in map order
- **Claim**: assign the issue to the driving developer before beginning work
- **Resolve**: comment with the answer, close the child issue, and add a context pointer to the map's Decisions-so-far

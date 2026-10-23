# Issue list

Issue list prints the open issues for one Forgejo repository. A repository with no open issues still returns a count line and help commands.

## Sub-features

- `issue-list-open` lists open issues for `kavemand/.forgejo`.
- `issue-list-empty` treats zero open issues as a successful empty state.

## How to get to it (user POV)

- Run `fjgo issue list OWNER/REPO --state open`.
- Run `fjgo --repo OWNER/REPO issue list --state open`.
- Run `fjgo issue list OWNER/REPO --state closed` for the closed list.

## Driving it with tmux

Preconditions:

- `./fjgo` exists and the skill doctor recorded `version: v1.4.3`.
- `FJGO_TOKEN` is unset on the command.
- The session name is `fjgo-verify-issue-list-<short-id>` and its working directory is the repository root.
- This recipe does not require a seeded issue. The public repository may have zero open issues.

- **List open issues.** Inside the session, run `env -u FJGO_TOKEN -u FJGO_USERNAME -u FJGO_PASSWORD -u FJGO_OTP -u FJGO_SUDO -u FJGO_REPO FJGO_HOST=v15.next.forgejo.org ./fjgo issue list kavemand/.forgejo --state open > .agents/skills/verify-fjgo/evidence/issue-list/stdout.txt 2> .agents/skills/verify-fjgo/evidence/issue-list/stderr.txt; printf '%s\n' "$?" > .agents/skills/verify-fjgo/evidence/issue-list/exit.txt`. Exit code `0`. Stdout contains `kavemand/.forgejo`. Stderr is empty.
- **Empty state.** When the repository has no open issues, stdout contains `0 issues found for kavemand/.forgejo` and a help line that mentions `issue list --state closed`. That empty line is the proof. Do not create an issue to force a row.
- **Proof.** Save the tmux pane to `.agents/skills/verify-fjgo/evidence/issue-list/transcript.txt`. `stdout.txt` still names `kavemand/.forgejo`.

## Gotchas

- Zero open issues is success. A recipe that requires a positive count fails for lack of a seeded issue, and that failure belongs in this section rather than a weaker assertion.
- Creating an issue would be a mutation. This feature does not send `--yes` and does not drop `--dry-run`.
- Do not use `-R origin` in this clone.

package main

import (
	"context"
	"io"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var issueListFlags = newFlagSet(
	[]string{"--json", "--fields", "--state", "--labels", "--assignee", "--author", "--q", "--sort", "--limit", "--page"},
	[]string{"--fields", "--state", "--labels", "--assignee", "--author", "--q", "--sort", "--limit", "--page"},
)

var issueViewFlags = newFlagSet(
	[]string{"--json", "--fields", "--comments", "--full"},
	[]string{"--fields"},
)

var issueCreateFlags = newFlagSet(
	[]string{"--title", "--body", "--body-file", "-body", "--assignee", "--label-id", "--milestone", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--title", "--body", "--body-file", "-body", "--assignee", "--label-id", "--milestone"},
)

var issueEditFlags = newFlagSet(
	[]string{"--title", "--body", "--body-file", "-body", "--state", "--assignee", "--milestone", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--title", "--body", "--body-file", "-body", "--state", "--assignee", "--milestone"},
)

var issueCommentFlags = newFlagSet(
	[]string{"--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--body", "--body-file", "-body"},
)

var issueMutationFlags = newFlagSet(
	[]string{"--yes", "--dry-run", "--print-request", "--json"},
	nil,
)

var issuePinnedFlags = newFlagSet(
	[]string{"--json", "--fields"},
	[]string{"--fields"},
)

var issueDependencyListFlags = newFlagSet(
	[]string{"--json", "--fields", "--limit", "--page"},
	[]string{"--fields", "--limit", "--page"},
)

var issueReactionListFlags = newFlagSet(
	[]string{"--json", "--limit", "--page"},
	[]string{"--limit", "--page"},
)

var issueTimeListFlags = newFlagSet(
	[]string{"--json", "--limit", "--page", "--user", "--since", "--before"},
	[]string{"--limit", "--page", "--user", "--since", "--before"},
)

var issueTimeAddFlags = newFlagSet(
	[]string{"--seconds", "--user", "--created", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--seconds", "--user", "--created"},
)

var issueDependencyParentFlags = newFlagSet(
	[]string{"--limit", "--page", "--fields", "--json", "--yes", "--dry-run", "--print-request"},
	[]string{"--limit", "--page", "--fields"},
)

var issueReactionParentFlags = newFlagSet(
	[]string{"--limit", "--page", "--json", "--yes", "--dry-run", "--print-request"},
	[]string{"--limit", "--page"},
)

var issueTimeParentFlags = newFlagSet(
	[]string{"--seconds", "--limit", "--page", "--json", "--yes", "--dry-run", "--print-request"},
	[]string{"--seconds", "--limit", "--page"},
)

func issueGroup() commandGroup {
	dependencies := issueDependencyGroup("dependencies")
	blocks := issueDependencyGroup("blocks")
	reactions := issueReactionGroup()
	deadline := issueDeadlineGroup()
	tracked := issueTimeGroup()
	return commandGroup{
		name:     "issue",
		overview: issueHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: issueListHelp(), flags: issueListFlags, run: runLeaf(runIssueList)},
			{names: []string{"view"}, help: issueViewHelp(), flags: issueViewFlags, run: runLeaf(runIssueView)},
			{names: []string{"create"}, help: issueCreateHelp(), flags: issueCreateFlags, mutation: mutationRequired, run: runLeaf(runIssueCreate)},
			{names: []string{"edit"}, help: issueEditHelp(), flags: issueEditFlags, mutation: mutationRequired, run: runLeaf(runIssueEdit)},
			{names: []string{"close"}, help: issueStateHelp("close"), flags: issueMutationFlags, mutation: mutationRequired, run: issueStateLeaf("closed")},
			{names: []string{"reopen"}, help: issueStateHelp("reopen"), flags: issueMutationFlags, mutation: mutationRequired, run: issueStateLeaf("open")},
			{names: []string{"comment"}, help: issueCommentHelp(), flags: issueCommentFlags, mutation: mutationRequired, run: runLeaf(runIssueComment)},
			{names: []string{"pinned"}, help: issuePinnedHelp(), flags: issuePinnedFlags, run: runLeaf(runIssuePinned)},
			{names: []string{"pin"}, help: issuePinHelp("pin"), flags: issueMutationFlags, mutation: mutationRequired, run: runIssuePinLeaf},
			{names: []string{"unpin"}, help: issuePinHelp("unpin"), flags: issueMutationFlags, mutation: mutationRequired, run: runIssuePinLeaf},
			{names: []string{"move-pin"}, help: issuePinHelp("move-pin"), flags: issueMutationFlags, mutation: mutationRequired, run: runIssuePinLeaf},
			{
				names: []string{"dependencies", "deps"}, help: issueDependencyParentHelp("dependencies"), flags: issueDependencyParentFlags,
				children: &dependencies,
				run:      dispatchKind(issueDependencyGroup),
			},
			{
				names: []string{"blocks"}, help: issueDependencyParentHelp("blocks"), flags: issueMutationFlags,
				children: &blocks,
				run:      dispatchKind(issueDependencyGroup),
			},
			{
				names: []string{"reactions"}, help: issueReactionParentHelp(), flags: issueReactionParentFlags,
				children: &reactions,
				run:      dispatchFixed(issueReactionGroup),
			},
			{names: []string{"react"}, help: issueReactHelp(), flags: issueMutationFlags, mutation: mutationRequired, run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runIssueReactionMutate(ctx, client, cfg, args, stdout, true)
			}},
			{
				names: []string{"deadline"}, help: issueDeadlineParentHelp(), flags: issueMutationFlags,
				children: &deadline,
				run:      dispatchFixed(issueDeadlineGroup),
			},
			{
				names:    []string{"time", "times"},
				help:     issueTimeParentHelp("time"),
				flags:    issueTimeParentFlags,
				children: &tracked,
				run:      dispatchFixed(issueTimeGroup),
			},
		},
	}
}

func issueStateLeaf(state string) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return runIssueState(ctx, client, cfg, args, stdout, state)
	}
}

func runIssuePinLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runIssuePin(ctx, client, cfg, args, stdout, matched)
}

func dispatchKind(build func(kind string) commandGroup) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return dispatchGroup(build(matched), ctx, client, cfg, args, stdout)
	}
}

func dispatchFixed(build func() commandGroup) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return dispatchGroup(build(), ctx, client, cfg, args, stdout)
	}
}

func issueListHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage: "fjgo issue list [owner/repo] [flags]",
		Flags: []string{
			"--state <open|closed|all> (default open)",
			"--labels <a,b>, --assignee <user>, --author <user>, --q <text>, --sort <key>",
			"--limit <n> (default " + defaultListLimit + "), --page <n>",
			"--fields <a,b,c>, --json",
		},
		Examples: []string{
			"fjgo -R origin issue list --state open",
			"fjgo issue list OWNER/REPO --fields number,title,state,author",
		},
	}
}

func issueViewHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue view [owner/repo] <number> [flags]",
		Flags:    []string{"--comments", "--full", "--fields <a,b,c>, --json"},
		Examples: []string{"fjgo -R origin issue view 42", "fjgo issue view OWNER/REPO 42 --comments --full"},
	}
}

func issueCreateHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue create [owner/repo] --title <text> [flags] --yes",
		Flags:    []string{"--body <text>, --body-file <path>", "--assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue create --title \"Bug\" --body-file issue.md --dry-run --yes", "fjgo issue create OWNER/REPO --title \"Bug\" --yes"},
	}
}

func issueEditHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue edit [owner/repo] <number> [flags] --yes",
		Flags:    []string{"--title <text>, --body <text>, --body-file <path>", "--state <open|closed>, --assignee <user>, --milestone <id>", "--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue edit 42 --title \"New title\" --dry-run --yes", "fjgo issue edit OWNER/REPO 42 --body-file body.md --yes"},
	}
}

func issueStateHelp(command string) commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue " + command + " [owner/repo] <number> --yes",
		Flags:    []string{"--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue " + command + " 42 --dry-run --yes", "fjgo issue " + command + " OWNER/REPO 42 --yes"},
	}
}

func issueCommentHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue comment [owner/repo] <number> --body <text> --yes",
		Flags:    []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue comment 42 --body \"done\" --dry-run --yes", "fjgo issue comment OWNER/REPO 42 --body-file comment.md --yes"},
	}
}

func issuePinnedHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue pinned [owner/repo] [--fields <a,b,c>] [--json]",
		Examples: []string{"fjgo -R origin issue pinned", "fjgo issue pinned OWNER/REPO --fields number,title,state"},
	}
}

func issuePinHelp(command string) commandHelpSpec {
	switch command {
	case "unpin":
		return commandHelpSpec{
			Usage:    "fjgo issue unpin [owner/repo] <number> --yes",
			Flags:    []string{"--dry-run, --print-request, --json"},
			Examples: []string{"fjgo -R origin issue unpin 42 --dry-run --yes", "fjgo issue unpin OWNER/REPO 42 --yes"},
		}
	case "move-pin":
		return commandHelpSpec{
			Usage:    "fjgo issue move-pin [owner/repo] <number> <position> --yes",
			Flags:    []string{"--dry-run, --print-request, --json"},
			Examples: []string{"fjgo -R origin issue move-pin 42 1 --dry-run --yes", "fjgo issue move-pin OWNER/REPO 42 1 --yes"},
		}
	default:
		return commandHelpSpec{
			Usage:    "fjgo issue pin [owner/repo] <number> [position] --yes",
			Flags:    []string{"--dry-run, --print-request, --json"},
			Examples: []string{"fjgo -R origin issue pin 42 --dry-run --yes", "fjgo issue pin OWNER/REPO 42 1 --yes"},
		}
	}
}

func issueDependencyParentHelp(kind string) commandHelpSpec {
	if kind == "blocks" {
		return commandHelpSpec{
			Usage:    "fjgo issue blocks <add|remove> [owner/repo] <number> <other-number> --yes",
			Flags:    []string{"--dry-run, --print-request, --json"},
			Examples: []string{"fjgo -R origin issue blocks add 42 7 --dry-run --yes", "fjgo issue blocks remove OWNER/REPO 42 7 --yes"},
		}
	}
	return commandHelpSpec{
		Usage:    "fjgo issue dependencies <list|add|remove> [owner/repo] <number> [other-number] [flags]",
		Flags:    []string{"--limit <n> (default " + defaultListLimit + "), --page <n>, --fields <a,b,c>, --json", "--yes, --dry-run, --print-request"},
		Examples: []string{"fjgo -R origin issue dependencies list 42", "fjgo issue dependencies add OWNER/REPO 42 7 --dry-run --yes"},
	}
}

func issueReactionParentHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue reactions <list|add|remove> [owner/repo] <number> [reaction] [flags]",
		Flags:    []string{"--limit <n> (default " + defaultListLimit + "), --page <n>, --json", "--yes, --dry-run, --print-request"},
		Examples: []string{"fjgo -R origin issue reactions list 42", "fjgo issue reactions add OWNER/REPO 42 +1 --dry-run --yes"},
	}
}

func issueReactHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue react [owner/repo] <number> <reaction> --yes",
		Flags:    []string{"--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue react 42 +1 --dry-run --yes", "fjgo issue react OWNER/REPO 42 +1 --yes"},
	}
}

func issueDeadlineParentHelp() commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue deadline <set|clear> [owner/repo] <number> [date] --yes",
		Flags:    []string{"--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin issue deadline set 42 2026-08-01 --dry-run --yes", "fjgo issue deadline clear OWNER/REPO 42 --yes"},
	}
}

func issueTimeParentHelp(kind string) commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo issue " + kind + " <list|add|reset|delete> [owner/repo] <number> [flags]",
		Flags:    []string{"--seconds <n>", "--limit <n> (default " + defaultListLimit + "), --page <n>, --json", "--yes, --dry-run, --print-request"},
		Examples: []string{"fjgo -R origin issue " + kind + " list 42", "fjgo issue " + kind + " add OWNER/REPO 42 --seconds 900 --dry-run --yes"},
	}
}

func issueDependencyGroup(kind string) commandGroup {
	listHelp := commandHelpSpec{}
	if kind == "dependencies" {
		listHelp = commandHelpSpec{
			Usage:    "fjgo issue dependencies list [owner/repo] <number> [flags]",
			Flags:    []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"},
			Examples: []string{"fjgo -R origin issue dependencies list 42", "fjgo issue dependencies list OWNER/REPO 42 --fields number,title,state"},
		}
	}
	mutate := func(action string, add bool) commandLeaf {
		command := kind + " " + action
		return commandLeaf{
			names: []string{action},
			help: commandHelpSpec{
				Usage:    "fjgo issue " + command + " [owner/repo] <number> <other-number> --yes",
				Flags:    []string{"--dry-run, --print-request, --json"},
				Examples: []string{"fjgo -R origin issue " + command + " 42 7 --dry-run --yes", "fjgo issue " + command + " OWNER/REPO 42 7 --yes"},
			},
			flags:    issueMutationFlags,
			mutation: mutationRequired,
			run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runIssueDependencyMutate(ctx, client, cfg, args, stdout, kind, add)
			},
		}
	}
	remove := mutate("remove", false)
	remove.names = []string{"remove", "delete"}
	list := commandLeaf{
		names: []string{"list"},
		help:  listHelp,
		flags: issueDependencyListFlags,
		run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
			if kind != "dependencies" && kind != "deps" {
				return newUsageError("issue blocks does not support list", "Run `fjgo issue dependencies list <number>` to list issues that block a target")
			}
			return runIssueDependencyList(ctx, client, cfg, args, stdout)
		},
	}
	return commandGroup{
		name: "issue " + kind,
		overview: func() string {
			return "usage: fjgo issue " + kind + " <list|add|remove> [owner/repo] <number> [other-number] [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue " + kind + " list 42\n  fjgo --repo OWNER/REPO issue " + kind + " add 42 7 --dry-run --yes\n  fjgo --repo OWNER/REPO issue " + kind + " remove 42 7 --dry-run --yes"
		},
		leaves: []commandLeaf{list, mutate("add", true), remove},
	}
}

func issueReactionGroup() commandGroup {
	mutate := func(action string, add bool) commandLeaf {
		command := "reactions " + action
		return commandLeaf{
			names: []string{action},
			help: commandHelpSpec{
				Usage:    "fjgo issue " + command + " [owner/repo] <number> <reaction> --yes",
				Flags:    []string{"--dry-run, --print-request, --json"},
				Examples: []string{"fjgo -R origin issue " + command + " 42 +1 --dry-run --yes", "fjgo issue " + command + " OWNER/REPO 42 +1 --yes"},
			},
			flags:    issueMutationFlags,
			mutation: mutationRequired,
			run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runIssueReactionMutate(ctx, client, cfg, args, stdout, add)
			},
		}
	}
	remove := mutate("remove", false)
	remove.names = []string{"remove", "delete"}
	return commandGroup{
		name: "issue reactions",
		overview: func() string {
			return "usage: fjgo issue reactions <list|add|remove> [owner/repo] <number> [reaction] [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue reactions list 42\n  fjgo --repo OWNER/REPO issue reactions add 42 +1 --dry-run --yes\n  fjgo --repo OWNER/REPO issue reactions remove 42 +1 --dry-run --yes"
		},
		leaves: []commandLeaf{
			{
				names: []string{"list"},
				help: commandHelpSpec{
					Usage:    "fjgo issue reactions list [owner/repo] <number> [flags]",
					Flags:    []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--json"},
					Examples: []string{"fjgo -R origin issue reactions list 42", "fjgo issue reactions list OWNER/REPO 42 --limit 100"},
				},
				flags: issueReactionListFlags,
				run:   runLeaf(runIssueReactionList),
			},
			mutate("add", true),
			remove,
		},
	}
}

func issueDeadlineGroup() commandGroup {
	leaf := func(action string) commandLeaf {
		date := ""
		if action == "set" {
			date = " <YYYY-MM-DD>"
		}
		return commandLeaf{
			names: []string{action},
			help: commandHelpSpec{
				Usage:    "fjgo issue deadline " + action + " [owner/repo] <number>" + date + " --yes",
				Flags:    []string{"--dry-run, --print-request, --json"},
				Examples: []string{"fjgo -R origin issue deadline " + action + " 42" + date + " --dry-run --yes", "fjgo issue deadline " + action + " OWNER/REPO 42" + date + " --yes"},
			},
			flags:    issueMutationFlags,
			mutation: mutationRequired,
			run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runIssueDeadline(ctx, client, cfg, args, stdout, matched)
			},
		}
	}
	clear := leaf("clear")
	clear.names = []string{"clear", "delete"}
	return commandGroup{
		name: "issue deadline",
		overview: func() string {
			return "usage: fjgo issue deadline <set|clear> [owner/repo] <number> [date] --yes\nexamples:\n  fjgo --repo OWNER/REPO issue deadline set 42 2026-08-01 --dry-run --yes\n  fjgo --repo OWNER/REPO issue deadline clear 42 --dry-run --yes"
		},
		leaves: []commandLeaf{leaf("set"), clear},
	}
}

func issueTimeGroup() commandGroup {
	deleteLeaf := commandLeaf{
		names: []string{"delete", "remove"},
		help: commandHelpSpec{
			Usage:    "fjgo issue time delete [owner/repo] <number> <time-id> --yes",
			Flags:    []string{"--dry-run, --print-request, --json"},
			Examples: []string{"fjgo -R origin issue time delete 42 <time-id> --dry-run --yes", "fjgo issue time delete OWNER/REPO 42 <time-id> --yes"},
		},
		flags:    issueMutationFlags,
		mutation: mutationRequired,
		run:      runLeaf(runIssueTimeDelete),
	}
	return commandGroup{
		name: "issue time",
		overview: func() string {
			return "usage: fjgo issue time <list|add|reset|delete> [owner/repo] <number> [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue time list 42\n  fjgo --repo OWNER/REPO issue time add 42 --seconds 900 --dry-run --yes\n  fjgo --repo OWNER/REPO issue time reset 42 --dry-run --yes"
		},
		leaves: []commandLeaf{
			{
				names: []string{"list"},
				help: commandHelpSpec{
					Usage:    "fjgo issue time list [owner/repo] <number> [flags]",
					Flags:    []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--user <user>, --since <timestamp>, --before <timestamp>", "--json"},
					Examples: []string{"fjgo -R origin issue time list 42", "fjgo issue time list OWNER/REPO 42 --user alice"},
				},
				flags: issueTimeListFlags,
				run:   runLeaf(runIssueTimeList),
			},
			{
				names: []string{"add"},
				help: commandHelpSpec{
					Usage:    "fjgo issue time add [owner/repo] <number> --seconds <n> --yes",
					Flags:    []string{"--user <user>", "--dry-run, --print-request, --json"},
					Examples: []string{"fjgo -R origin issue time add 42 --seconds 900 --dry-run --yes", "fjgo issue time add OWNER/REPO 42 --seconds 900 --yes"},
				},
				flags:    issueTimeAddFlags,
				mutation: mutationRequired,
				run:      runLeaf(runIssueTimeAdd),
			},
			{
				names: []string{"reset"},
				help: commandHelpSpec{
					Usage:    "fjgo issue time reset [owner/repo] <number> --yes",
					Flags:    []string{"--dry-run, --print-request, --json"},
					Examples: []string{"fjgo -R origin issue time reset 42 --dry-run --yes", "fjgo issue time reset OWNER/REPO 42 --yes"},
				},
				flags:    issueMutationFlags,
				mutation: mutationRequired,
				run:      runLeaf(runIssueTimeReset),
			},
			deleteLeaf,
		},
	}
}

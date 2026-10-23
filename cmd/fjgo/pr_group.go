package main

import (
	"context"
	"io"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var prListFlags = newFlagSet(
	[]string{"--json", "--fields", "--state", "--author", "--sort", "--limit", "--page"},
	[]string{"--fields", "--state", "--author", "--sort", "--limit", "--page"},
)
var prViewFlags = newFlagSet([]string{"--json", "--fields", "--full", "--reviews"}, []string{"--fields"})
var prCreateFlags = newFlagSet(
	[]string{"--title", "--body", "--body-file", "-body", "--head", "--base", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--title", "--body", "--body-file", "-body", "--head", "--base"},
)
var prEditFlags = newFlagSet(
	[]string{"--title", "--body", "--body-file", "-body", "--state", "--base", "--assignee", "--label-id", "--milestone", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--title", "--body", "--body-file", "-body", "--state", "--base", "--assignee", "--label-id", "--milestone"},
)
var prCommentFlags = newFlagSet(
	[]string{"--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--body", "--body-file", "-body"},
)
var prFieldsFlags = newFlagSet([]string{"--json", "--fields"}, []string{"--fields"})
var prMergeFlags = newFlagSet(
	[]string{"--method", "--delete-branch", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--method"},
)
var prReviewFlags = newFlagSet(
	[]string{"--approve", "--request-changes", "--comment", "--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--body", "--body-file", "-body"},
)
var prReviewsFlags = newFlagSet([]string{"--json", "--limit", "--page"}, []string{"--limit", "--page"})
var prReviewRequestFlags = newFlagSet(
	[]string{"--reviewer", "--team", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--reviewer", "--team"},
)
var prReviewCommentFlags = newFlagSet(
	[]string{"--body", "--body-file", "-body", "--path", "--new-line", "--old-line", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--body", "--body-file", "-body", "--path", "--new-line", "--old-line"},
)
var prUpdateFlags = newFlagSet(
	[]string{"--style", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--style"},
)
var prDiffFlags = newFlagSet([]string{"--binary", "--full", "--json"}, nil)

func prGroup() commandGroup {
	requests := prReviewRequestGroup()
	return commandGroup{
		name:     "pr",
		overview: prHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: prSpec("list"), flags: prListFlags, run: runLeaf(runPRList)},
			{names: []string{"view"}, help: prSpec("view"), flags: prViewFlags, run: runLeaf(runPRView)},
			{names: []string{"create"}, help: prSpec("create"), flags: prCreateFlags, mutation: mutationRequired, run: runLeaf(runPRCreate)},
			{names: []string{"edit"}, help: prSpec("edit"), flags: prEditFlags, mutation: mutationRequired, run: runLeaf(runPREdit)},
			{names: []string{"close"}, help: prSpec("close"), flags: issueMutationFlags, mutation: mutationRequired, run: prStateLeaf("closed")},
			{names: []string{"reopen"}, help: prSpec("reopen"), flags: issueMutationFlags, mutation: mutationRequired, run: prStateLeaf("open")},
			{names: []string{"comment"}, help: prSpec("comment"), flags: prCommentFlags, mutation: mutationRequired, run: runLeaf(runPRComment)},
			{names: []string{"files"}, help: prSpec("files"), flags: prFieldsFlags, run: runLeaf(runPRFiles)},
			{names: []string{"commits"}, help: prSpec("commits"), flags: prFieldsFlags, run: runLeaf(runPRCommits)},
			{names: []string{"checks"}, help: prSpec("checks"), flags: prFieldsFlags, run: runLeaf(runPRChecks)},
			{names: []string{"merge"}, help: prSpec("merge"), flags: prMergeFlags, mutation: mutationRequired, run: runLeaf(runPRMerge)},
			{names: []string{"review"}, help: prSpec("review"), flags: prReviewFlags, mutation: mutationRequired, run: runLeaf(runPRReview)},
			{names: []string{"reviews"}, help: prSpec("reviews"), flags: prReviewsFlags, run: runLeaf(runPRReviews)},
			{
				names: []string{"review-requests"}, help: prSpec("review-requests"), flags: prReviewRequestFlags,
				children: &requests,
				run:      runReviewRequestEntry(true),
			},
			{names: []string{"request-review"}, help: prSpec("request-review"), flags: prReviewRequestFlags, mutation: mutationRequired, run: runReviewRequestEntry(true)},
			{names: []string{"unrequest-review"}, help: prSpec("unrequest-review"), flags: prReviewRequestFlags, mutation: mutationRequired, run: runReviewRequestEntry(false)},
			{names: []string{"review-comment"}, help: prSpec("review-comment"), flags: prReviewCommentFlags, mutation: mutationRequired, run: runLeaf(runPRReviewComment)},
			{names: []string{"update"}, help: prSpec("update"), flags: prUpdateFlags, mutation: mutationRequired, run: runLeaf(runPRUpdate)},
			{names: []string{"diff"}, help: prSpec("diff"), flags: prDiffFlags, run: prDiffLeaf("diff")},
			{names: []string{"patch"}, help: prSpec("patch"), flags: prDiffFlags, run: prDiffLeaf("patch")},
		},
	}
}

func prCommandHelps() map[string]commandHelpSpec {
	return prGroup().helpSpecs()
}

func prStateLeaf(state string) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return runPRState(ctx, client, cfg, args, stdout, state)
	}
}

func prDiffLeaf(kind string) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return runPRDiffPatch(ctx, client, cfg, args, stdout, kind)
	}
}

func runReviewRequestEntry(defaultAdd bool) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		if !defaultAdd {
			return runPRReviewRequests(ctx, client, cfg, args, stdout, false)
		}
		if len(args) > 0 && (args[0] == "remove" || args[0] == "delete") {
			return runPRReviewRequests(ctx, client, cfg, args[1:], stdout, false)
		}
		if len(args) > 0 && args[0] == "add" {
			return runPRReviewRequests(ctx, client, cfg, args[1:], stdout, true)
		}
		return runPRReviewRequests(ctx, client, cfg, args, stdout, true)
	}
}

func prReviewRequestGroup() commandGroup {
	remove := commandLeaf{
		names:    []string{"remove", "delete"},
		help:     prReviewRequestActionHelp("remove"),
		flags:    prReviewRequestFlags,
		mutation: mutationRequired,
	}
	return commandGroup{
		name: "pr review-requests",
		leaves: []commandLeaf{
			{names: []string{"add"}, help: prReviewRequestActionHelp("add"), flags: prReviewRequestFlags, mutation: mutationRequired},
			remove,
		},
	}
}

func prReviewRequestActionHelp(action string) commandHelpSpec {
	return commandHelpSpec{
		Usage:    "fjgo pr review-requests " + action + " [owner/repo] <number> (--reviewer <user>|--team <team>) --yes",
		Flags:    []string{"--reviewer <user> (repeatable), --team <team> (repeatable)", "--dry-run, --print-request, --json"},
		Examples: []string{"fjgo -R origin pr review-requests " + action + " 12 --reviewer alice --dry-run --yes", "fjgo pr review-requests " + action + " OWNER/REPO 12 --team qa --yes"},
	}
}

func prSpec(name string) commandHelpSpec {
	specs := map[string]commandHelpSpec{
		"list":             {Usage: "fjgo pr list [owner/repo] [flags]", Flags: []string{"--state <open|closed|all> (default open)", "--author <user>, --sort <key>, --limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin pr list --state open", "fjgo pr list OWNER/REPO --fields number,title,state,author"}},
		"view":             {Usage: "fjgo pr view [owner/repo] <number> [flags]", Flags: []string{"--reviews", "--full", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin pr view 12 --full", "fjgo pr view OWNER/REPO 12 --reviews"}},
		"create":           {Usage: "fjgo pr create [owner/repo] --title <text> --head <branch> [flags] --yes", Flags: []string{"--base <branch>, --body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr create --title \"Fix\" --head feature --dry-run --yes", "fjgo pr create OWNER/REPO --title \"Fix\" --head feature --yes"}},
		"edit":             {Usage: "fjgo pr edit [owner/repo] <number> [flags] --yes", Flags: []string{"--title <text>, --body <text>, --body-file <path>", "--base <branch>, --assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr edit 12 --title \"Fix\" --dry-run --yes", "fjgo pr edit OWNER/REPO 12 --body-file body.md --yes"}},
		"close":            {Usage: "fjgo pr close [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr close 12 --dry-run --yes", "fjgo pr close OWNER/REPO 12 --yes"}},
		"reopen":           {Usage: "fjgo pr reopen [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr reopen 12 --dry-run --yes", "fjgo pr reopen OWNER/REPO 12 --yes"}},
		"comment":          {Usage: "fjgo pr comment [owner/repo] <number> --body <text> --yes", Flags: []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr comment 12 --body \"done\" --dry-run --yes", "fjgo pr comment OWNER/REPO 12 --body-file comment.md --yes"}},
		"files":            {Usage: "fjgo pr files [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr files 12", "fjgo pr files OWNER/REPO 12 --fields filename,status"}},
		"commits":          {Usage: "fjgo pr commits [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr commits 12", "fjgo pr commits OWNER/REPO 12 --fields sha,message,author"}},
		"checks":           {Usage: "fjgo pr checks [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr checks 12", "fjgo pr checks OWNER/REPO 12 --fields context,state"}},
		"merge":            {Usage: "fjgo pr merge [owner/repo] <number> [flags] --yes", Flags: []string{"--method <merge|rebase|squash>, --delete-branch", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr merge 12 --method squash --dry-run --yes", "fjgo pr merge OWNER/REPO 12 --yes"}},
		"review":           {Usage: "fjgo pr review [owner/repo] <number> (--approve|--request-changes|--comment) --yes", Flags: []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr review 12 --approve --dry-run --yes", "fjgo pr review OWNER/REPO 12 --comment --body \"note\" --yes"}},
		"reviews":          {Usage: "fjgo pr reviews [owner/repo] <number> [--limit <n>] [--page <n>] [--json]", Examples: []string{"fjgo -R origin pr reviews 12", "fjgo pr reviews OWNER/REPO 12 --limit 20"}},
		"review-requests":  {Usage: "fjgo pr review-requests <add|remove> [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr review-requests add 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr review-requests remove 12 --reviewer alice --yes"}},
		"request-review":   {Usage: "fjgo pr request-review [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr request-review 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr request-review 12 --team qa --yes"}},
		"unrequest-review": {Usage: "fjgo pr unrequest-review [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr unrequest-review 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr unrequest-review 12 --team qa --yes"}},
		"review-comment":   {Usage: "fjgo pr review-comment [owner/repo] <number> <review-id> --path <file> --new-line <n> --body <text> --yes", Flags: []string{"--path <file>, --new-line <n>, --old-line <n>, --body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr review-comment 12 34 --path main.go --new-line 10 --body \"note\" --dry-run --yes", "fjgo --repo OWNER/REPO pr review-comment 12 34 --path main.go --new-line 10 --body-file note.md --yes"}},
		"update":           {Usage: "fjgo pr update [owner/repo] <number> [--style merge|rebase] --yes", Flags: []string{"--style <merge|rebase>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr update 12 --style rebase --dry-run --yes", "fjgo pr update OWNER/REPO 12 --yes"}},
		"diff":             {Usage: "fjgo pr diff [owner/repo] <number> [--binary] [--full] [--json]", Examples: []string{"fjgo -R origin pr diff 12", "fjgo pr diff OWNER/REPO 12 --full"}},
		"patch":            {Usage: "fjgo pr patch [owner/repo] <number> [--binary] [--full] [--json]", Examples: []string{"fjgo -R origin pr patch 12", "fjgo pr patch OWNER/REPO 12 --full"}},
	}
	return specs[name]
}

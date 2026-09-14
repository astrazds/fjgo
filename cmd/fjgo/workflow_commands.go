package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var issueFields = []string{"number", "title", "state", "author", "comments", "labels", "updated", "url", "body"}
var issueListDefaultFields = []string{"number", "title", "state", "author"}
var issueViewDefaultFields = []string{"number", "title", "state", "author", "comments", "body"}

var prFields = []string{"number", "title", "state", "author", "draft", "mergeable", "merged", "comments", "reviews", "additions", "deletions", "changed_files", "labels", "updated", "url", "body", "head", "base"}
var prListDefaultFields = []string{"number", "title", "state", "author", "draft"}
var prViewDefaultFields = []string{"number", "title", "state", "author", "draft", "mergeable", "merged", "body"}

func issueHelp() string {
	return `usage: fjgo issue <subcommand> [owner/repo] [flags]

subcommands:
  list [owner/repo]                  list repository issues
  view [owner/repo] <number>         show issue details
  create [owner/repo] --title <t>    create an issue
  edit [owner/repo] <number>         edit an issue
  close [owner/repo] <number>        close an issue idempotently
  reopen [owner/repo] <number>       reopen an issue idempotently
  comment [owner/repo] <number>      add an issue comment
  pinned [owner/repo]                list pinned issues
  pin|unpin [owner/repo] <number>    pin or unpin an issue
  dependencies <list|add|remove>     manage issue dependencies
  blocks <add|remove>                manage issue blocking links
  reactions <list|add|remove>        manage issue reactions
  deadline <set|clear>               set or clear issue deadline
  time <list|add|reset|delete>       manage tracked time

flags:
  --state <open|closed|all>, --labels <a,b>, --assignee <user>, --author <user>, --q <text>, --sort <key>, --limit <n>, --page <n>
  --fields <a,b,c>, --full, --json
  --title <text>, --body <text>, --body-file <path>, --assignee <user>, --label-id <id>, --milestone <id>
  --seconds <n>, --reviewer <user>, --team <team>
  --yes, --dry-run, --print-request

examples:
  fjgo -R origin issue list --state open
  fjgo issue view OWNER/REPO 42 --comments
  fjgo -R origin issue create --title "Bug" --body-file issue.md --dry-run --yes
  fjgo -R origin issue close 42 --yes
  fjgo --repo OWNER/REPO issue dependencies add 42 7 --dry-run --yes
  fjgo --repo OWNER/REPO issue time add 42 --seconds 900 --dry-run --yes`
}

func issueCommandHelps() map[string]commandHelpSpec {
	helps := map[string]commandHelpSpec{
		"list": {
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
		},
		"view": {
			Usage: "fjgo issue view [owner/repo] <number> [flags]",
			Flags: []string{"--comments", "--full", "--fields <a,b,c>, --json"},
			Examples: []string{
				"fjgo -R origin issue view 42",
				"fjgo issue view OWNER/REPO 42 --comments --full",
			},
		},
		"create":       {Usage: "fjgo issue create [owner/repo] --title <text> [flags] --yes", Flags: []string{"--body <text>, --body-file <path>", "--assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue create --title \"Bug\" --body-file issue.md --dry-run --yes", "fjgo issue create OWNER/REPO --title \"Bug\" --yes"}},
		"edit":         {Usage: "fjgo issue edit [owner/repo] <number> [flags] --yes", Flags: []string{"--title <text>, --body <text>, --body-file <path>", "--state <open|closed>, --assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue edit 42 --title \"New title\" --dry-run --yes", "fjgo issue edit OWNER/REPO 42 --body-file body.md --yes"}},
		"close":        {Usage: "fjgo issue close [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue close 42 --dry-run --yes", "fjgo issue close OWNER/REPO 42 --yes"}},
		"reopen":       {Usage: "fjgo issue reopen [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue reopen 42 --dry-run --yes", "fjgo issue reopen OWNER/REPO 42 --yes"}},
		"comment":      {Usage: "fjgo issue comment [owner/repo] <number> --body <text> --yes", Flags: []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue comment 42 --body \"done\" --dry-run --yes", "fjgo issue comment OWNER/REPO 42 --body-file comment.md --yes"}},
		"pinned":       {Usage: "fjgo issue pinned [owner/repo] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin issue pinned", "fjgo issue pinned OWNER/REPO --fields number,title,state"}},
		"pin":          {Usage: "fjgo issue pin [owner/repo] <number> [position] --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue pin 42 --dry-run --yes", "fjgo issue pin OWNER/REPO 42 1 --yes"}},
		"unpin":        {Usage: "fjgo issue unpin [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue unpin 42 --dry-run --yes", "fjgo issue unpin OWNER/REPO 42 --yes"}},
		"move-pin":     {Usage: "fjgo issue move-pin [owner/repo] <number> <position> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue move-pin 42 1 --dry-run --yes", "fjgo issue move-pin OWNER/REPO 42 1 --yes"}},
		"dependencies": {Usage: "fjgo issue dependencies <list|add|remove> [owner/repo] <number> [other-number] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>, --fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin issue dependencies list 42", "fjgo issue dependencies add OWNER/REPO 42 7 --dry-run --yes"}},
		"deps":         {Usage: "fjgo issue deps <list|add|remove> [owner/repo] <number> [other-number] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>, --fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin issue deps list 42", "fjgo issue deps add OWNER/REPO 42 7 --dry-run --yes"}},
		"blocks":       {Usage: "fjgo issue blocks <add|remove> [owner/repo] <number> <other-number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue blocks add 42 7 --dry-run --yes", "fjgo issue blocks remove OWNER/REPO 42 7 --yes"}},
		"reactions":    {Usage: "fjgo issue reactions <list|add|remove> [owner/repo] <number> [reaction] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin issue reactions list 42", "fjgo issue reactions add OWNER/REPO 42 +1 --dry-run --yes"}},
		"react":        {Usage: "fjgo issue react [owner/repo] <number> <reaction> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue react 42 +1 --dry-run --yes", "fjgo issue react OWNER/REPO 42 +1 --yes"}},
		"deadline":     {Usage: "fjgo issue deadline <set|clear> [owner/repo] <number> [date] --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue deadline set 42 2026-08-01 --dry-run --yes", "fjgo issue deadline clear OWNER/REPO 42 --yes"}},
		"time":         {Usage: "fjgo issue time <list|add|reset|delete> [owner/repo] <number> [flags]", Flags: []string{"--seconds <n>", "--limit <n> (default " + defaultListLimit + "), --page <n>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin issue time list 42", "fjgo issue time add OWNER/REPO 42 --seconds 900 --dry-run --yes"}},
		"times":        {Usage: "fjgo issue times <list|add|reset|delete> [owner/repo] <number> [flags]", Flags: []string{"--seconds <n>", "--limit <n> (default " + defaultListLimit + "), --page <n>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin issue times list 42", "fjgo issue times add OWNER/REPO 42 --seconds 900 --dry-run --yes"}},
	}
	for command, spec := range issueNestedCommandHelps() {
		helps[command] = spec
	}
	return helps
}

func issueNestedCommandHelps() map[string]commandHelpSpec {
	listDependencies := commandHelpSpec{Usage: "fjgo issue dependencies list [owner/repo] <number> [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin issue dependencies list 42", "fjgo issue dependencies list OWNER/REPO 42 --fields number,title,state"}}
	mutateDependency := func(command string) commandHelpSpec {
		return commandHelpSpec{Usage: "fjgo issue " + command + " [owner/repo] <number> <other-number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue " + command + " 42 7 --dry-run --yes", "fjgo issue " + command + " OWNER/REPO 42 7 --yes"}}
	}
	reactionList := commandHelpSpec{Usage: "fjgo issue reactions list [owner/repo] <number> [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--json"}, Examples: []string{"fjgo -R origin issue reactions list 42", "fjgo issue reactions list OWNER/REPO 42 --limit 100"}}
	reactionMutate := func(action string) commandHelpSpec {
		command := "reactions " + action
		return commandHelpSpec{Usage: "fjgo issue " + command + " [owner/repo] <number> <reaction> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue " + command + " 42 +1 --dry-run --yes", "fjgo issue " + command + " OWNER/REPO 42 +1 --yes"}}
	}
	deadline := func(action string) commandHelpSpec {
		date := ""
		if action == "set" {
			date = " <YYYY-MM-DD>"
		}
		return commandHelpSpec{Usage: "fjgo issue deadline " + action + " [owner/repo] <number>" + date + " --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue deadline " + action + " 42" + date + " --dry-run --yes", "fjgo issue deadline " + action + " OWNER/REPO 42" + date + " --yes"}}
	}
	timeList := commandHelpSpec{Usage: "fjgo issue time list [owner/repo] <number> [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--user <user>, --since <timestamp>, --before <timestamp>", "--json"}, Examples: []string{"fjgo -R origin issue time list 42", "fjgo issue time list OWNER/REPO 42 --user alice"}}
	timeAdd := commandHelpSpec{Usage: "fjgo issue time add [owner/repo] <number> --seconds <n> --yes", Flags: []string{"--user <user>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue time add 42 --seconds 900 --dry-run --yes", "fjgo issue time add OWNER/REPO 42 --seconds 900 --yes"}}
	timeMutate := func(action string) commandHelpSpec {
		extra := ""
		if action == "delete" {
			extra = " <time-id>"
		}
		return commandHelpSpec{Usage: "fjgo issue time " + action + " [owner/repo] <number>" + extra + " --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin issue time " + action + " 42" + extra + " --dry-run --yes", "fjgo issue time " + action + " OWNER/REPO 42" + extra + " --yes"}}
	}
	helps := map[string]commandHelpSpec{
		"dependencies list": listDependencies,
		"deps list":         renamedCommandHelp(listDependencies, "dependencies", "deps"),
		"reactions list":    reactionList,
		"deadline set":      deadline("set"),
		"deadline clear":    deadline("clear"),
		"deadline delete":   renamedCommandHelp(deadline("clear"), "deadline clear", "deadline delete"),
		"time list":         timeList,
		"times list":        renamedCommandHelp(timeList, "issue time", "issue times"),
		"time add":          timeAdd,
		"times add":         renamedCommandHelp(timeAdd, "issue time", "issue times"),
		"time reset":        timeMutate("reset"),
		"times reset":       renamedCommandHelp(timeMutate("reset"), "issue time", "issue times"),
		"time delete":       timeMutate("delete"),
		"time remove":       renamedCommandHelp(timeMutate("delete"), "time delete", "time remove"),
		"times delete":      renamedCommandHelp(timeMutate("delete"), "issue time", "issue times"),
		"times remove":      renamedCommandHelp(renamedCommandHelp(timeMutate("delete"), "issue time", "issue times"), "times delete", "times remove"),
	}
	for _, prefix := range []string{"dependencies", "deps", "blocks"} {
		for _, action := range []string{"add", "remove", "delete"} {
			canonical := action
			if canonical == "delete" {
				canonical = "remove"
			}
			spec := mutateDependency(prefix + " " + canonical)
			if action == "delete" {
				spec = renamedCommandHelp(spec, prefix+" remove", prefix+" delete")
			}
			helps[prefix+" "+action] = spec
		}
	}
	for _, action := range []string{"add", "remove", "delete"} {
		canonical := action
		if canonical == "delete" {
			canonical = "remove"
		}
		spec := reactionMutate(canonical)
		if action == "delete" {
			spec = renamedCommandHelp(spec, "reactions remove", "reactions delete")
		}
		helps["reactions "+action] = spec
	}
	return helps
}

func runIssue(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, issueHelp(), issueCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	switch args[0] {
	case "list":
		return runIssueList(ctx, client, cfg, args[1:], stdout)
	case "view":
		return runIssueView(ctx, client, cfg, args[1:], stdout)
	case "create":
		return runIssueCreate(ctx, client, cfg, args[1:], stdout)
	case "edit":
		return runIssueEdit(ctx, client, cfg, args[1:], stdout)
	case "close":
		return runIssueState(ctx, client, cfg, args[1:], stdout, "closed")
	case "reopen":
		return runIssueState(ctx, client, cfg, args[1:], stdout, "open")
	case "comment":
		return runIssueComment(ctx, client, cfg, args[1:], stdout)
	case "pinned":
		return runIssuePinned(ctx, client, cfg, args[1:], stdout)
	case "pin", "unpin", "move-pin":
		return runIssuePin(ctx, client, cfg, args[1:], stdout, args[0])
	case "dependencies", "deps":
		return runIssueDependencies(ctx, client, cfg, args[1:], stdout, args[0])
	case "blocks":
		return runIssueDependencies(ctx, client, cfg, args[1:], stdout, args[0])
	case "reactions":
		return runIssueReactions(ctx, client, cfg, args[1:], stdout)
	case "react":
		return runIssueReactionMutate(ctx, client, cfg, args[1:], stdout, true)
	case "deadline":
		return runIssueDeadline(ctx, client, cfg, args[1:], stdout)
	case "time", "times":
		return runIssueTime(ctx, client, cfg, args[1:], stdout)
	default:
		return unknownSubcommandError("issue", args[0], []string{"list", "view", "create", "edit", "close", "reopen", "comment", "pinned", "pin", "unpin", "dependencies", "blocks", "reactions", "deadline", "time"})
	}
}

func runIssueList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue list", []string{"--json", "--fields", "--state", "--labels", "--assignee", "--author", "--q", "--sort", "--limit", "--page"}, []string{"--fields", "--state", "--labels", "--assignee", "--author", "--q", "--sort", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, issueListDefaultFields, issueFields, "issue list")
	if err != nil {
		return err
	}
	query := url.Values{"state": {"open"}, "type": {"issues"}, "limit": {defaultListLimit}}
	requestedLabels := ""
	for _, spec := range []struct {
		flag string
		key  string
	}{
		{"--state", "state"},
		{"--labels", "labels"},
		{"--assignee", "assigned_by"},
		{"--author", "created_by"},
		{"--q", "q"},
		{"--sort", "sort"},
		{"--limit", "limit"},
		{"--page", "page"},
	} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
		if spec.key == "labels" && ok {
			requestedLabels = value
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil {
		return newUsageError("usage: fjgo issue list [owner/repo] [flags]", "Pass `owner/repo` or use `-R origin`")
	}
	extra, err := queryFromPairs(rest)
	if err != nil {
		return err
	}
	for key, values := range extra {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	if requestedLabels != "" {
		if err := validateIssueListLabels(ctx, client, ref, requestedLabels); err != nil {
			return err
		}
	}
	resp, err := rawOperationResponse(ctx, client, "issueListIssues", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	issues, err := decodeBody[[]*forgejo.Issue](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, issues)
	}
	rows := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		if issue != nil {
			rows = append(rows, issueRow(issue, false))
		}
	}
	return writeRows(stdout, "issues", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 issues found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "issue",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func validateIssueListLabels(ctx context.Context, client *forgejo.Client, ref repoRef, requested string) error {
	requestedSet := make(map[string]struct{})
	for _, name := range splitCSV(requested) {
		requestedSet[name] = struct{}{}
	}
	if len(requestedSet) == 0 {
		return nil
	}

	const pageSize = 100
	existing := make(map[string]struct{})
	var seen int64
	for page := 1; ; page++ {
		query := url.Values{
			"limit": {strconv.Itoa(pageSize)},
			"page":  {strconv.Itoa(page)},
		}
		resp, err := rawOperationResponse(ctx, client, "issueListLabels", repoPath(ref), query, nil)
		if err != nil {
			return err
		}
		labels, err := decodeBody[[]*forgejo.Label](resp.Body)
		if err != nil {
			return err
		}
		seen += int64(len(labels))
		for _, label := range labels {
			if label != nil {
				existing[label.Name] = struct{}{}
			}
		}
		total := responseTotal(resp)
		if len(labels) == 0 || (total > 0 && seen >= total) {
			break
		}
	}

	missing := make([]string, 0)
	for name := range requestedSet {
		if _, ok := existing[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	noun := "label"
	if len(missing) != 1 {
		noun = "labels"
	}
	return newCLIError(
		"LABEL_NOT_FOUND",
		fmt.Sprintf("missing repository %s: %s", noun, strings.Join(missing, ", ")),
		fmt.Sprintf("Run `%s` to inspect available labels", commandForRepo(refPtr(ref), "label list")),
	)
}

func runIssueView(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue view", []string{"--json", "--fields", "--comments", "--full"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	args, comments := boolFlag(args, "--comments")
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, issueViewDefaultFields, issueFields, "issue view")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue view [owner/repo] <number>", "Pass `owner/repo` or use `-R origin`")
	}
	issue, err := client.IssueGetIssue(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, issue)
	}
	if issue == nil {
		return writeTOON(stdout, map[string]any{"issue": "not found"})
	}
	blocks := toonBlocks{map[string]any{"issue": selectFields(issueRow(issue, !full), fields)}}
	if comments {
		items, err := client.IssueGetComments(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{Query: url.Values{"limit": {"100"}}})
		if err != nil {
			return err
		}
		rows := make([]map[string]any, 0, len(items))
		for _, comment := range items {
			if comment == nil {
				continue
			}
			body := comment.Body
			if !full {
				var report truncateReport
				body = truncateString(body, defaultTruncateChars, "comments.body", &report)
			}
			rows = append(rows, map[string]any{
				"id":      comment.ID,
				"author":  userName(comment.User),
				"created": comment.Created,
				"body":    body,
			})
		}
		blocks = append(blocks, tableBlock("comments", []string{"id", "author", "created", "body"}, rows))
	}
	if !full && len([]rune(issue.Body)) > defaultTruncateChars {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `fjgo issue view %s/%s %s --full` to see the complete body", ref.Owner, ref.Repo, rest[0])}))
	} else if stateString(issue.State) == "open" {
		blocks = append(blocks, suggestionHelp(suggestionContext{Domain: "issue", Action: "view", State: "open", ID: rest[0], Repo: refPtr(ref)}))
	}
	return writeTOON(stdout, blocks)
}

func runIssueCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue create", []string{"--title", "--body", "--body-file", "-body", "--assignee", "--label-id", "--milestone", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--title", "--body", "--body-file", "-body", "--assignee", "--label-id", "--milestone"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, _, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	var title, assignee, milestoneRaw string
	args, title, _, err = takeValueFlag(args, "--title")
	if err != nil {
		return err
	}
	args, assignee, _, err = takeValueFlag(args, "--assignee")
	if err != nil {
		return err
	}
	args, milestoneRaw, _, err = takeValueFlag(args, "--milestone")
	if err != nil {
		return err
	}
	args, labelsRaw, err := takeAllValueFlags(args, "--label-id")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo issue create [owner/repo] --title <text> [--body text|--body-file path] --yes")
	}
	if title == "" {
		return newUsageError("--title is required", "Run `fjgo issue create --title \"...\" --body-file <path> --dry-run --yes`")
	}
	if !yes {
		return newUsageError("issue create requires --yes")
	}
	bodyValue := &forgejo.CreateIssueOption{Title: title, Body: body, Assignee: assignee}
	if milestoneRaw != "" {
		milestone, err := parseInt64Value("--milestone", milestoneRaw)
		if err != nil {
			return err
		}
		bodyValue.Milestone = milestone
	}
	labels, err := parseIDList("--label-id", labelsRaw)
	if err != nil {
		return err
	}
	bodyValue.Labels = labels
	if dryRun {
		return writeMutationPreview(stdout, client, "issueCreateIssue", ref, nil, bodyValue, yes)
	}
	issue, err := client.IssueCreateIssue(ctx, ref.Owner, ref.Repo, bodyValue, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, issue)
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"issue": issueRow(issue, true)},
		suggestionHelp(suggestionContext{Domain: "issue", Action: "mutate", ID: strconv.FormatInt(issue.Index, 10), Repo: refPtr(ref)}),
	})
}

func runIssueEdit(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue edit", []string{"--title", "--body", "--body-file", "-body", "--state", "--assignee", "--milestone", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--title", "--body", "--body-file", "-body", "--state", "--assignee", "--milestone"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, bodySet, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, flag := range []string{"--title", "--state", "--assignee", "--milestone"} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, flag)
		if err != nil {
			return err
		}
		if ok {
			values[flag] = value
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue edit [owner/repo] <number> [flags] --yes")
	}
	if !yes {
		return newUsageError("issue edit requires --yes")
	}
	if len(values) == 0 && !bodySet {
		return newUsageError("issue edit requires at least one change", "Use --title, --body, --body-file, --state, --assignee, or --milestone")
	}
	edit := &forgejo.EditIssueOption{}
	if v, ok := values["--title"]; ok {
		edit.Title = v
	}
	if v, ok := values["--state"]; ok {
		edit.State = v
	}
	if v, ok := values["--assignee"]; ok {
		edit.Assignee = v
	}
	if v, ok := values["--milestone"]; ok {
		milestone, err := parseInt64Value("--milestone", v)
		if err != nil {
			return err
		}
		edit.Milestone = milestone
	}
	if bodySet {
		edit.Body = body
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueEditIssue", ref, map[string]string{"index": rest[0]}, edit, yes)
	}
	issue, err := client.IssueEditIssue(ctx, ref.Owner, ref.Repo, rest[0], edit, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, issue)
	}
	return writeTOON(stdout, map[string]any{"issue": issueRow(issue, true)})
}

func runIssueState(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, state string) error {
	command := "issue close"
	if state == "open" {
		command = "issue reopen"
	}
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	edit := &forgejo.EditIssueOption{State: state}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueEditIssue", ref, map[string]string{"index": rest[0]}, edit, yes)
	}
	current, err := client.IssueGetIssue(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if current != nil && stateString(current.State) == state {
		return writeTOON(stdout, map[string]any{"issue": fmt.Sprintf("#%s already %s (no-op)", rest[0], state)})
	}
	issue, err := client.IssueEditIssue(ctx, ref.Owner, ref.Repo, rest[0], edit, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, issue)
	}
	return writeTOON(stdout, map[string]any{"issue": issueRow(issue, true)})
}

func runIssueComment(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue comment", []string{"--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--body", "--body-file", "-body"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, _, err := takeBodyText(args, true)
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue comment [owner/repo] <number> --body <text> --yes")
	}
	if !yes {
		return newUsageError("issue comment requires --yes")
	}
	bodyValue := &forgejo.CreateIssueCommentOption{Body: body}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueCreateComment", ref, map[string]string{"index": rest[0]}, bodyValue, yes)
	}
	comment, err := client.IssueCreateComment(ctx, ref.Owner, ref.Repo, rest[0], bodyValue, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, comment)
	}
	return writeTOON(stdout, map[string]any{"comment": map[string]any{
		"id":      comment.ID,
		"author":  userName(comment.User),
		"created": comment.Created,
		"body":    truncateString(comment.Body, defaultTruncateChars, "comment.body", &truncateReport{}),
	}})
}

func issueRow(issue *forgejo.Issue, truncate bool) map[string]any {
	if issue == nil {
		return map[string]any{}
	}
	body := issue.Body
	if truncate {
		var report truncateReport
		body = truncateString(body, defaultTruncateChars, "body", &report)
	}
	return map[string]any{
		"number":   issue.Index,
		"title":    issue.Title,
		"state":    stateString(issue.State),
		"author":   userName(issue.User),
		"comments": issue.Comments,
		"labels":   joinLabelNames(issue.Labels),
		"updated":  issue.Updated,
		"url":      issue.HTMLURL,
		"body":     body,
	}
}

func parseIDList(name string, values []string) ([]int64, error) {
	var out []int64
	for _, value := range values {
		for _, part := range splitCSV(value) {
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, newUsageError(name + " expects integer IDs")
			}
			out = append(out, id)
		}
	}
	return out, nil
}

func rawOperationResponse(ctx context.Context, client *forgejo.Client, operation string, pathValues map[string]string, query url.Values, body any) (forgejo.RawResponse, error) {
	op, ok := forgejo.OperationByID(operation)
	if !ok {
		return forgejo.RawResponse{}, fmt.Errorf("unknown operation %q", operation)
	}
	return client.DoOperationRawResponse(ctx, op, pathValues, forgejo.RequestOptions{Query: query, Body: body})
}

func prHelp() string {
	return `usage: fjgo pr <subcommand> [owner/repo] [flags]

subcommands:
  list [owner/repo]                         list pull requests
  view [owner/repo] <number>                show pull request details
  create [owner/repo] --title <t> --head h  create a pull request
  edit [owner/repo] <number>                edit a pull request
  close [owner/repo] <number>               close a pull request idempotently
  reopen [owner/repo] <number>              reopen a pull request idempotently
  comment [owner/repo] <number>             add a conversation comment
  files [owner/repo] <number>               list changed files
  commits [owner/repo] <number>             list commits
  checks [owner/repo] <number>              list commit statuses for the PR head
  merge [owner/repo] <number>               merge a pull request
  review [owner/repo] <number>              submit a review
  reviews [owner/repo] <number>             list PR reviews
  review-requests <add|remove>              manage requested reviewers
  review-comment [owner/repo] <number> <id> add a review comment
  update [owner/repo] <number>              update PR branch from base
  diff|patch [owner/repo] <number>          download PR diff or patch

flags:
  --state <open|closed|all>, --author <user>, --sort <key>, --limit <n>, --page <n>
  --fields <a,b,c>, --full, --json
  --title <text>, --body <text>, --body-file <path>, --head <branch>, --base <branch>, --assignee <user>, --label-id <id>, --milestone <id>
  --method <merge|rebase|squash>, --delete-branch, --approve, --request-changes, --comment
  --reviewer <user>, --team <team>, --style <merge|rebase>, --path <file>, --new-line <n>, --old-line <n>, --binary
  --yes, --dry-run, --print-request

examples:
  fjgo -R origin pr list --state open
  fjgo -R origin pr view 12 --full
  fjgo -R origin pr view 12 --reviews
  fjgo -R origin pr close 12 --dry-run --yes
  fjgo -R origin pr checks 12
  fjgo -R origin pr merge 12 --method squash --dry-run --yes
  fjgo --repo OWNER/REPO pr review-requests add 12 --reviewer alice --dry-run --yes
  fjgo --repo OWNER/REPO pr diff 12`
}

func prCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":                   {Usage: "fjgo pr list [owner/repo] [flags]", Flags: []string{"--state <open|closed|all> (default open)", "--author <user>, --sort <key>, --limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin pr list --state open", "fjgo pr list OWNER/REPO --fields number,title,state,author"}},
		"view":                   {Usage: "fjgo pr view [owner/repo] <number> [flags]", Flags: []string{"--reviews", "--full", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin pr view 12 --full", "fjgo pr view OWNER/REPO 12 --reviews"}},
		"create":                 {Usage: "fjgo pr create [owner/repo] --title <text> --head <branch> [flags] --yes", Flags: []string{"--base <branch>, --body <text>, --body-file <path>", "--assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr create --title \"Fix\" --head feature --dry-run --yes", "fjgo pr create OWNER/REPO --title \"Fix\" --head feature --yes"}},
		"edit":                   {Usage: "fjgo pr edit [owner/repo] <number> [flags] --yes", Flags: []string{"--title <text>, --body <text>, --body-file <path>", "--base <branch>, --assignee <user>, --label-id <id>, --milestone <id>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr edit 12 --title \"Fix\" --dry-run --yes", "fjgo pr edit OWNER/REPO 12 --body-file body.md --yes"}},
		"close":                  {Usage: "fjgo pr close [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr close 12 --dry-run --yes", "fjgo pr close OWNER/REPO 12 --yes"}},
		"reopen":                 {Usage: "fjgo pr reopen [owner/repo] <number> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr reopen 12 --dry-run --yes", "fjgo pr reopen OWNER/REPO 12 --yes"}},
		"comment":                {Usage: "fjgo pr comment [owner/repo] <number> --body <text> --yes", Flags: []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr comment 12 --body \"done\" --dry-run --yes", "fjgo pr comment OWNER/REPO 12 --body-file comment.md --yes"}},
		"files":                  {Usage: "fjgo pr files [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr files 12", "fjgo pr files OWNER/REPO 12 --fields filename,status"}},
		"commits":                {Usage: "fjgo pr commits [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr commits 12", "fjgo pr commits OWNER/REPO 12 --fields sha,message,author"}},
		"checks":                 {Usage: "fjgo pr checks [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr checks 12", "fjgo pr checks OWNER/REPO 12 --fields context,state"}},
		"merge":                  {Usage: "fjgo pr merge [owner/repo] <number> [flags] --yes", Flags: []string{"--method <merge|rebase|squash>, --delete-branch", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr merge 12 --method squash --dry-run --yes", "fjgo pr merge OWNER/REPO 12 --yes"}},
		"review":                 {Usage: "fjgo pr review [owner/repo] <number> (--approve|--request-changes|--comment) --yes", Flags: []string{"--body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr review 12 --approve --dry-run --yes", "fjgo pr review OWNER/REPO 12 --comment --body \"note\" --yes"}},
		"reviews":                {Usage: "fjgo pr reviews [owner/repo] <number> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin pr reviews 12", "fjgo pr reviews OWNER/REPO 12 --fields id,state,user"}},
		"review-requests":        {Usage: "fjgo pr review-requests <add|remove> [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr review-requests add 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr review-requests remove 12 --reviewer alice --yes"}},
		"review-requests add":    {Usage: "fjgo pr review-requests add [owner/repo] <number> (--reviewer <user>|--team <team>) --yes", Flags: []string{"--reviewer <user> (repeatable), --team <team> (repeatable)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr review-requests add 12 --reviewer alice --dry-run --yes", "fjgo pr review-requests add OWNER/REPO 12 --team qa --yes"}},
		"review-requests remove": {Usage: "fjgo pr review-requests remove [owner/repo] <number> (--reviewer <user>|--team <team>) --yes", Flags: []string{"--reviewer <user> (repeatable), --team <team> (repeatable)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr review-requests remove 12 --reviewer alice --dry-run --yes", "fjgo pr review-requests remove OWNER/REPO 12 --team qa --yes"}},
		"review-requests delete": {Usage: "fjgo pr review-requests delete [owner/repo] <number> (--reviewer <user>|--team <team>) --yes", Flags: []string{"--reviewer <user> (repeatable), --team <team> (repeatable)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr review-requests delete 12 --reviewer alice --dry-run --yes", "fjgo pr review-requests delete OWNER/REPO 12 --team qa --yes"}},
		"request-review":         {Usage: "fjgo pr request-review [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr request-review 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr request-review 12 --team qa --yes"}},
		"unrequest-review":       {Usage: "fjgo pr unrequest-review [owner/repo] <number> --reviewer <user> [--team team] --yes", Flags: []string{"--reviewer <user>, --team <team>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr unrequest-review 12 --reviewer alice --dry-run --yes", "fjgo --repo OWNER/REPO pr unrequest-review 12 --team qa --yes"}},
		"review-comment":         {Usage: "fjgo pr review-comment [owner/repo] <number> <review-id> --path <file> --new-line <n> --body <text> --yes", Flags: []string{"--path <file>, --new-line <n>, --old-line <n>, --body <text>, --body-file <path>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO pr review-comment 12 34 --path main.go --new-line 10 --body \"note\" --dry-run --yes", "fjgo --repo OWNER/REPO pr review-comment 12 34 --path main.go --new-line 10 --body-file note.md --yes"}},
		"update":                 {Usage: "fjgo pr update [owner/repo] <number> [--style merge|rebase] --yes", Flags: []string{"--style <merge|rebase>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin pr update 12 --style rebase --dry-run --yes", "fjgo pr update OWNER/REPO 12 --yes"}},
		"diff":                   {Usage: "fjgo pr diff [owner/repo] <number> [--binary] [--full] [--json]", Examples: []string{"fjgo -R origin pr diff 12", "fjgo pr diff OWNER/REPO 12 --full"}},
		"patch":                  {Usage: "fjgo pr patch [owner/repo] <number> [--binary] [--full] [--json]", Examples: []string{"fjgo -R origin pr patch 12", "fjgo pr patch OWNER/REPO 12 --full"}},
	}
}

func runPR(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, prHelp(), prCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	switch args[0] {
	case "list":
		return runPRList(ctx, client, cfg, args[1:], stdout)
	case "view":
		return runPRView(ctx, client, cfg, args[1:], stdout)
	case "create":
		return runPRCreate(ctx, client, cfg, args[1:], stdout)
	case "edit":
		return runPREdit(ctx, client, cfg, args[1:], stdout)
	case "close":
		return runPRState(ctx, client, cfg, args[1:], stdout, "closed")
	case "reopen":
		return runPRState(ctx, client, cfg, args[1:], stdout, "open")
	case "comment":
		return runPRComment(ctx, client, cfg, args[1:], stdout)
	case "files":
		return runPRFiles(ctx, client, cfg, args[1:], stdout)
	case "commits":
		return runPRCommits(ctx, client, cfg, args[1:], stdout)
	case "checks":
		return runPRChecks(ctx, client, cfg, args[1:], stdout)
	case "merge":
		return runPRMerge(ctx, client, cfg, args[1:], stdout)
	case "review":
		return runPRReview(ctx, client, cfg, args[1:], stdout)
	case "reviews":
		return runPRReviews(ctx, client, cfg, args[1:], stdout)
	case "review-requests", "request-review":
		if len(args) > 1 && (args[1] == "remove" || args[1] == "delete") {
			return runPRReviewRequests(ctx, client, cfg, args[2:], stdout, false)
		}
		if len(args) > 1 && args[1] == "add" {
			return runPRReviewRequests(ctx, client, cfg, args[2:], stdout, true)
		}
		return runPRReviewRequests(ctx, client, cfg, args[1:], stdout, true)
	case "unrequest-review":
		return runPRReviewRequests(ctx, client, cfg, args[1:], stdout, false)
	case "review-comment":
		return runPRReviewComment(ctx, client, cfg, args[1:], stdout)
	case "update":
		return runPRUpdate(ctx, client, cfg, args[1:], stdout)
	case "diff":
		return runPRDiffPatch(ctx, client, cfg, args[1:], stdout, "diff")
	case "patch":
		return runPRDiffPatch(ctx, client, cfg, args[1:], stdout, "patch")
	default:
		return unknownSubcommandError("pr", args[0], []string{"list", "view", "create", "edit", "close", "reopen", "comment", "files", "commits", "checks", "merge", "review", "reviews", "review-requests", "review-comment", "update", "diff", "patch"})
	}
}

func runPRList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr list", []string{"--json", "--fields", "--state", "--author", "--sort", "--limit", "--page"}, []string{"--fields", "--state", "--author", "--sort", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, prListDefaultFields, prFields, "pr list")
	if err != nil {
		return err
	}
	query := url.Values{"state": {"open"}, "limit": {defaultListLimit}}
	for _, spec := range []struct {
		flag string
		key  string
	}{
		{"--state", "state"},
		{"--author", "poster"},
		{"--sort", "sort"},
		{"--limit", "limit"},
		{"--page", "page"},
	} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil {
		return newUsageError("usage: fjgo pr list [owner/repo] [flags]", "Pass `owner/repo` or use `-R origin`")
	}
	extra, err := queryFromPairs(rest)
	if err != nil {
		return err
	}
	for key, values := range extra {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	resp, err := rawOperationResponse(ctx, client, "repoListPullRequests", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	pulls, err := decodeBody[[]*forgejo.PullRequest](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, pulls)
	}
	rows := make([]map[string]any, 0, len(pulls))
	for _, pr := range pulls {
		if pr != nil {
			rows = append(rows, prRow(pr, false))
		}
	}
	return writeRows(stdout, "pulls", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 pull requests found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "pr",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runPRView(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr view", []string{"--json", "--fields", "--full", "--reviews"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	args, includeReviews := boolFlag(args, "--reviews")
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, prViewDefaultFields, prFields, "pr view")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr view [owner/repo] <number>", "Pass `owner/repo` or use `-R origin`")
	}
	pr, err := client.RepoGetPullRequest(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, pr)
	}
	if pr == nil {
		return writeTOON(stdout, map[string]any{"pull": "not found"})
	}
	row := prRow(pr, !full)
	if includeReviews {
		reviews, err := client.RepoListPullReviews(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{Query: url.Values{"limit": {"100"}}})
		if err != nil {
			return err
		}
		row["reviews"] = len(reviews)
		blocks := toonBlocks{map[string]any{"pull": selectFields(row, fieldsWith(fields, "reviews"))}}
		blocks = append(blocks, pullReviewsTable(reviews))
		return writeTOON(stdout, blocks)
	}
	blocks := toonBlocks{map[string]any{"pull": selectFields(row, fields)}}
	if !full && len([]rune(pr.Body)) > defaultTruncateChars {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `fjgo pr view %s/%s %s --full` to see the complete body", ref.Owner, ref.Repo, rest[0])}))
	} else if stateString(pr.State) == "open" {
		blocks = append(blocks, suggestionHelp(suggestionContext{Domain: "pr", Action: "view", State: "open", ID: rest[0], Repo: refPtr(ref)}))
	}
	return writeTOON(stdout, blocks)
}

func runPREdit(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr edit", []string{"--title", "--body", "--body-file", "-body", "--state", "--base", "--assignee", "--label-id", "--milestone", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--title", "--body", "--body-file", "-body", "--state", "--base", "--assignee", "--label-id", "--milestone"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, bodySet, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, flag := range []string{"--title", "--state", "--base", "--assignee", "--milestone"} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, flag)
		if err != nil {
			return err
		}
		if ok {
			values[flag] = value
		}
	}
	args, labelsRaw, err := takeAllValueFlags(args, "--label-id")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr edit [owner/repo] <number> [flags] --yes")
	}
	if !yes {
		return newUsageError("pr edit requires --yes")
	}
	if len(values) == 0 && !bodySet && len(labelsRaw) == 0 {
		return newUsageError("pr edit requires at least one change", "Use --title, --body, --base, --state, --assignee, --label-id, or --milestone")
	}
	edit := &forgejo.EditPullRequestOption{}
	if v, ok := values["--title"]; ok {
		edit.Title = v
	}
	if v, ok := values["--state"]; ok {
		edit.State = v
	}
	if v, ok := values["--base"]; ok {
		edit.Base = v
	}
	if v, ok := values["--assignee"]; ok {
		edit.Assignee = v
	}
	if v, ok := values["--milestone"]; ok {
		milestone, err := parseInt64Value("--milestone", v)
		if err != nil {
			return err
		}
		edit.Milestone = milestone
	}
	if bodySet {
		edit.Body = body
	}
	labels, err := parseIDList("--label-id", labelsRaw)
	if err != nil {
		return err
	}
	edit.Labels = labels
	if dryRun {
		return writeMutationPreview(stdout, client, "repoEditPullRequest", ref, map[string]string{"index": rest[0]}, edit, yes)
	}
	pr, err := client.RepoEditPullRequest(ctx, ref.Owner, ref.Repo, rest[0], edit, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, pr)
	}
	return writeTOON(stdout, map[string]any{"pull": prRow(pr, true)})
}

func runPRState(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, state string) error {
	command := "pr close"
	if state == "open" {
		command = "pr reopen"
	}
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	edit := &forgejo.EditPullRequestOption{State: state}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoEditPullRequest", ref, map[string]string{"index": rest[0]}, edit, yes)
	}
	current, err := client.RepoGetPullRequest(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if current != nil && stateString(current.State) == state {
		return writeTOON(stdout, map[string]any{"pull": fmt.Sprintf("#%s already %s (no-op)", rest[0], state)})
	}
	pr, err := client.RepoEditPullRequest(ctx, ref.Owner, ref.Repo, rest[0], edit, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, pr)
	}
	return writeTOON(stdout, map[string]any{"pull": prRow(pr, true)})
}

func runPRComment(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr comment", []string{"--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--body", "--body-file", "-body"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, _, err := takeBodyText(args, true)
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr comment [owner/repo] <number> --body <text> --yes")
	}
	if !yes {
		return newUsageError("pr comment requires --yes")
	}
	bodyValue := &forgejo.CreateIssueCommentOption{Body: body}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueCreateComment", ref, map[string]string{"index": rest[0]}, bodyValue, yes)
	}
	comment, err := client.IssueCreateComment(ctx, ref.Owner, ref.Repo, rest[0], bodyValue, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, comment)
	}
	return writeTOON(stdout, map[string]any{"comment": map[string]any{"id": comment.ID, "author": userName(comment.User), "created": comment.Created, "body": truncateString(comment.Body, defaultTruncateChars, "comment.body", &truncateReport{})}})
}

func runPRCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr create", []string{"--title", "--body", "--body-file", "-body", "--head", "--base", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--title", "--body", "--body-file", "-body", "--head", "--base"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, _, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for _, flag := range []string{"--title", "--head", "--base"} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, flag)
		if err != nil {
			return err
		}
		if ok {
			values[flag] = value
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo pr create [owner/repo] --title <text> --head <branch> [--base branch] --yes")
	}
	if values["--title"] == "" || values["--head"] == "" {
		return newUsageError("pr create requires --title and --head")
	}
	if !yes {
		return newUsageError("pr create requires --yes")
	}
	base := values["--base"]
	if base == "" {
		base = "main"
	}
	bodyValue := &forgejo.CreatePullRequestOption{Title: values["--title"], Head: values["--head"], Base: base, Body: body}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoCreatePullRequest", ref, nil, bodyValue, yes)
	}
	pr, err := client.RepoCreatePullRequest(ctx, ref.Owner, ref.Repo, bodyValue, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, pr)
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"pull": prRow(pr, true)},
		helpBlock([]string{fmt.Sprintf("Run `fjgo pr checks %s/%s %d` to monitor statuses", ref.Owner, ref.Repo, pr.Index)}),
	})
}

func runPRFiles(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr files", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"filename", "status", "additions", "deletions"}
	available := []string{"filename", "status", "additions", "deletions", "changes", "previous_filename", "html_url", "raw_url"}
	fields, err := validatedFields(fieldsArg, defaults, available, "pr files")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr files [owner/repo] <number>")
	}
	files, err := client.RepoGetPullRequestFiles(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, files)
	}
	rows := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		rows = append(rows, map[string]any{
			"filename":          file.Filename,
			"status":            file.Status,
			"additions":         file.Additions,
			"deletions":         file.Deletions,
			"changes":           file.Changes,
			"previous_filename": file.PreviousFilename,
			"html_url":          file.HTMLURL,
			"raw_url":           file.RawURL,
		})
	}
	return writeRows(stdout, "files", rowsSelect(rows, fields), fields, 0, fmt.Sprintf("0 changed files found for PR %s", rest[0]), nil)
}

func runPRCommits(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr commits", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"sha", "message", "author", "created"}
	available := []string{"sha", "message", "author", "created", "url"}
	fields, err := validatedFields(fieldsArg, defaults, available, "pr commits")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr commits [owner/repo] <number>")
	}
	commits, err := client.RepoGetPullRequestCommits(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, commits)
	}
	rows := make([]map[string]any, 0, len(commits))
	for _, commit := range commits {
		if commit == nil {
			continue
		}
		message := ""
		if commit.Commit != nil {
			message = strings.Split(commit.Commit.Message, "\n")[0]
		}
		rows = append(rows, map[string]any{
			"sha":     shortSHA(commit.SHA),
			"message": message,
			"author":  userName(commit.Author),
			"created": commit.Created,
			"url":     commit.HTMLURL,
		})
	}
	return writeRows(stdout, "commits", rowsSelect(rows, fields), fields, 0, fmt.Sprintf("0 commits found for PR %s", rest[0]), nil)
}

func runPRChecks(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr checks", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"context", "status", "description"}
	available := []string{"context", "status", "description", "creator", "target_url", "updated"}
	fields, err := validatedFields(fieldsArg, defaults, available, "pr checks")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr checks [owner/repo] <number>")
	}
	pr, err := client.RepoGetPullRequest(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	sha := ""
	if pr != nil && pr.Head != nil {
		sha = pr.Head.Sha
	}
	if sha == "" {
		return fmt.Errorf("pull request %s has no head sha", rest[0])
	}
	statuses, err := client.RepoListStatusesByRef(ctx, ref.Owner, ref.Repo, sha, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, statuses)
	}
	rows := make([]map[string]any, 0, len(statuses))
	for _, status := range statuses {
		if status == nil {
			continue
		}
		state := ""
		if status.Status != nil {
			state = string(*status.Status)
		}
		rows = append(rows, map[string]any{
			"context":     status.Context,
			"status":      state,
			"description": status.Description,
			"creator":     userName(status.Creator),
			"target_url":  status.TargetURL,
			"updated":     status.Updated,
		})
	}
	return writeRows(stdout, "checks", rowsSelect(rows, fields), fields, 0, fmt.Sprintf("0 statuses found for PR %s head %s", rest[0], shortSHA(sha)), nil)
}

func runPRMerge(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr merge", []string{"--method", "--delete-branch", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--method"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, deleteBranch := boolFlag(args, "--delete-branch")
	method := "merge"
	var ok bool
	var err error
	args, method, ok, err = takeValueFlag(args, "--method")
	if err != nil {
		return err
	}
	if !ok {
		method = "merge"
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr merge [owner/repo] <number> --yes")
	}
	if !yes {
		return newUsageError("pr merge requires --yes")
	}
	body := &forgejo.MergePullRequestOption{Do: method, DeleteBranchAfterMerge: deleteBranch}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoMergePullRequest", ref, map[string]string{"index": rest[0]}, body, yes)
	}
	if err := client.RepoMergePullRequest(ctx, ref.Owner, ref.Repo, rest[0], body, forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"merged": true, "number": rest[0]})
	}
	return writeTOON(stdout, map[string]any{"pull": fmt.Sprintf("#%s merged with %s", rest[0], method)})
}

func runPRReview(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr review", []string{"--approve", "--request-changes", "--comment", "--body", "--body-file", "-body", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--body", "--body-file", "-body"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, body, _, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	args, approve := boolFlag(args, "--approve")
	args, requestChanges := boolFlag(args, "--request-changes")
	args, commentOnly := boolFlag(args, "--comment")
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr review [owner/repo] <number> (--approve|--request-changes|--comment) --yes")
	}
	selected := 0
	for _, b := range []bool{approve, requestChanges, commentOnly} {
		if b {
			selected++
		}
	}
	if selected != 1 {
		return newUsageError("choose exactly one review event: --approve, --request-changes, or --comment")
	}
	if !yes {
		return newUsageError("pr review requires --yes")
	}
	event := "COMMENT"
	if approve {
		event = "APPROVED"
	}
	if requestChanges {
		event = "REQUEST_CHANGES"
	}
	bodyValue := &forgejo.CreatePullReviewOptions{Body: body, Event: reviewState(event)}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoCreatePullReview", ref, map[string]string{"index": rest[0]}, bodyValue, yes)
	}
	review, err := client.RepoCreatePullReview(ctx, ref.Owner, ref.Repo, rest[0], bodyValue, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, review)
	}
	return writeTOON(stdout, map[string]any{"review": map[string]any{
		"id":       review.ID,
		"state":    event,
		"author":   userName(review.User),
		"html_url": review.HTMLURL,
	}})
}

func prRow(pr *forgejo.PullRequest, truncate bool) map[string]any {
	if pr == nil {
		return map[string]any{}
	}
	body := pr.Body
	if truncate {
		var report truncateReport
		body = truncateString(body, defaultTruncateChars, "body", &report)
	}
	head := ""
	if pr.Head != nil {
		head = pr.Head.Ref
	}
	base := ""
	if pr.Base != nil {
		base = pr.Base.Ref
	}
	return map[string]any{
		"number":        pr.Index,
		"title":         pr.Title,
		"state":         stateString(pr.State),
		"author":        userName(pr.User),
		"draft":         pr.Draft,
		"mergeable":     pr.Mergeable,
		"merged":        pr.HasMerged,
		"comments":      pr.Comments,
		"additions":     pr.Additions,
		"deletions":     pr.Deletions,
		"changed_files": pr.ChangedFiles,
		"labels":        joinLabelNames(pr.Labels),
		"updated":       pr.Updated,
		"url":           pr.HTMLURL,
		"body":          body,
		"head":          head,
		"base":          base,
	}
}

func shortSHA(sha string) string {
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

package main

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

func runIssuePin(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, action string) error {
	command := "issue " + action
	operation := "pinIssue"
	if action == "unpin" {
		operation = "unpinIssue"
	}
	if action == "move-pin" {
		operation = "moveIssuePin"
	}
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || (action != "move-pin" && len(rest) != 1) || (action == "move-pin" && len(rest) != 2) {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> [position] --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	extra := map[string]string{"index": rest[0]}
	if action == "move-pin" {
		extra["position"] = rest[1]
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, extra, nil, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, extra), forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"issue": rest[0], "pin": action})
	}
	if len(out) != 0 {
		return writeJSONAsTOON(stdout, out, "issue", false)
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"issue": fmt.Sprintf("#%s %s ok", rest[0], action)},
		suggestionHelp(suggestionContext{Domain: "issue", Action: "mutate", ID: rest[0], Repo: refPtr(ref)}),
	})
}

func runIssuePinned(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue pinned", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, issueListDefaultFields, issueFields, "issue pinned")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo issue pinned [owner/repo]")
	}
	issues, err := client.RepoListPinnedIssues(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
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
	return writeRows(stdout, "pinned_issues", rowsSelect(rows, fields), fields, 0, fmt.Sprintf("0 pinned issues found for %s/%s", ref.Owner, ref.Repo), nil)
}

func runIssueDependencies(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, kind string) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo issue "+kind+" <list|add|remove> [owner/repo] <number> [other-number] [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue "+kind+" list 42\n  fjgo --repo OWNER/REPO issue "+kind+" add 42 7 --dry-run --yes\n  fjgo --repo OWNER/REPO issue "+kind+" remove 42 7 --dry-run --yes")
	}
	switch args[0] {
	case "list":
		if kind != "dependencies" && kind != "deps" {
			return newUsageError("issue blocks does not support list", "Run `fjgo issue dependencies list <number>` to list issues that block a target")
		}
		return runIssueDependencyList(ctx, client, cfg, args[1:], stdout)
	case "add":
		return runIssueDependencyMutate(ctx, client, cfg, args[1:], stdout, kind, true)
	case "remove", "delete":
		return runIssueDependencyMutate(ctx, client, cfg, args[1:], stdout, kind, false)
	default:
		return unknownSubcommandError("issue "+kind, args[0], []string{"list", "add", "remove", "delete"})
	}
}

func runIssueDependencyList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue dependencies list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, issueListDefaultFields, issueFields, "issue dependencies list")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {defaultListLimit}}
	for _, spec := range []struct{ flag, key string }{{"--limit", "limit"}, {"--page", "page"}} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue dependencies list [owner/repo] <number>")
	}
	resp, err := rawOperationResponse(ctx, client, "issueListIssueDependencies", repoPathWith(ref, map[string]string{"index": rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	issues, err := decodeBody[[]*forgejo.Issue](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		if issue != nil {
			rows = append(rows, issueRow(issue, false))
		}
	}
	return writeRows(stdout, "dependencies", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 dependencies found for issue %s", rest[0]), nil)
}

func runIssueDependencyMutate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, kind string, add bool) error {
	action := "add"
	if !add {
		action = "remove"
	}
	command := "issue " + kind + " " + action
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 2 {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> <other-number> --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	other, err := parseInt64Value("other-number", rest[1])
	if err != nil {
		return err
	}
	body := map[string]any{"index": other}
	operation := "issueCreateIssueDependencies"
	if !add {
		operation = "issueRemoveIssueDependencies"
	}
	if kind == "blocks" {
		operation = "issueCreateIssueBlocking"
		if !add {
			operation = "issueRemoveIssueBlocking"
		}
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, map[string]string{"index": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, map[string]string{"index": rest[0]}), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "issue", false)
}

func runIssueReactions(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo issue reactions <list|add|remove> [owner/repo] <number> [reaction] [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue reactions list 42\n  fjgo --repo OWNER/REPO issue reactions add 42 +1 --dry-run --yes\n  fjgo --repo OWNER/REPO issue reactions remove 42 +1 --dry-run --yes")
	}
	switch args[0] {
	case "list":
		return runIssueReactionList(ctx, client, cfg, args[1:], stdout)
	case "add":
		return runIssueReactionMutate(ctx, client, cfg, args[1:], stdout, true)
	case "remove", "delete":
		return runIssueReactionMutate(ctx, client, cfg, args[1:], stdout, false)
	default:
		return unknownSubcommandError("issue reactions", args[0], []string{"list", "add", "remove", "delete"})
	}
}

func runIssueReactionList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue reactions list", []string{"--json", "--limit", "--page"}, []string{"--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	query := url.Values{"limit": {defaultListLimit}}
	var err error
	for _, spec := range []struct{ flag, key string }{{"--limit", "limit"}, {"--page", "page"}} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue reactions list [owner/repo] <number>")
	}
	resp, err := rawOperationResponse(ctx, client, "issueGetIssueReactions", repoPathWith(ref, map[string]string{"index": rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	reactions, err := decodeBody[[]*forgejo.Reaction](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(reactions))
	for _, reaction := range reactions {
		if reaction != nil {
			rows = append(rows, map[string]any{"reaction": reaction.Reaction, "user": userName(reaction.User), "created": reaction.Created})
		}
	}
	return writeRows(stdout, "reactions", rows, []string{"reaction", "user", "created"}, responseTotal(resp), fmt.Sprintf("0 reactions found for issue %s", rest[0]), nil)
}

func runIssueReactionMutate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, add bool) error {
	action := "add"
	operation := "issuePostIssueReaction"
	if !add {
		action = "remove"
		operation = "issueDeleteIssueReaction"
	}
	command := "issue reactions " + action
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 2 {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> <reaction> --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	body := map[string]any{"content": rest[1]}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, map[string]string{"index": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, map[string]string{"index": rest[0]}), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	if len(out) == 0 {
		return writeTOON(stdout, map[string]any{"reaction": fmt.Sprintf("%s %s issue %s", rest[1], action, rest[0])})
	}
	return writeJSONAsTOON(stdout, out, "reaction", false)
}

func runIssueDeadline(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo issue deadline <set|clear> [owner/repo] <number> [date] --yes\nexamples:\n  fjgo --repo OWNER/REPO issue deadline set 42 2026-08-01 --dry-run --yes\n  fjgo --repo OWNER/REPO issue deadline clear 42 --dry-run --yes")
	}
	clear := args[0] == "clear" || args[0] == "delete"
	if args[0] != "set" && !clear {
		return unknownSubcommandError("issue deadline", args[0], []string{"set", "clear", "delete"})
	}
	restArgs := args[1:]
	if err := rejectUnknownFlags(restArgs, "issue deadline "+args[0], []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	restArgs, jsonOut := takeJSONFlag(restArgs)
	restArgs, yes := takeYesFlag(restArgs)
	restArgs, dryRun := takeDryRunFlag(restArgs)
	ref, rest, err := repoFromArgs(cfg, restArgs)
	if err != nil || (!clear && len(rest) != 2) || (clear && len(rest) != 1) {
		return newUsageError("usage: fjgo issue deadline " + args[0] + " [owner/repo] <number> [date] --yes")
	}
	if !yes {
		return newUsageError("issue deadline " + args[0] + " requires --yes")
	}
	var due any
	if !clear {
		due = rest[1]
	}
	body := map[string]any{"due_date": due}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueEditIssueDeadline", ref, map[string]string{"index": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID("issueEditIssueDeadline")
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, map[string]string{"index": rest[0]}), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "deadline", false)
}

func runIssueTime(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo issue time <list|add|reset|delete> [owner/repo] <number> [flags]\nexamples:\n  fjgo --repo OWNER/REPO issue time list 42\n  fjgo --repo OWNER/REPO issue time add 42 --seconds 900 --dry-run --yes\n  fjgo --repo OWNER/REPO issue time reset 42 --dry-run --yes")
	}
	switch args[0] {
	case "list":
		return runIssueTimeList(ctx, client, cfg, args[1:], stdout)
	case "add":
		return runIssueTimeAdd(ctx, client, cfg, args[1:], stdout)
	case "reset":
		return runIssueTimeReset(ctx, client, cfg, args[1:], stdout)
	case "delete", "remove":
		return runIssueTimeDelete(ctx, client, cfg, args[1:], stdout)
	default:
		return unknownSubcommandError("issue time", args[0], []string{"list", "add", "reset", "delete", "remove"})
	}
}

func runIssueTimeList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue time list", []string{"--json", "--limit", "--page", "--user", "--since", "--before"}, []string{"--limit", "--page", "--user", "--since", "--before"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	query := url.Values{"limit": {defaultListLimit}}
	var err error
	for _, spec := range []struct{ flag, key string }{{"--limit", "limit"}, {"--page", "page"}, {"--user", "user"}, {"--since", "since"}, {"--before", "before"}} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue time list [owner/repo] <number>")
	}
	resp, err := rawOperationResponse(ctx, client, "issueTrackedTimes", repoPathWith(ref, map[string]string{"index": rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	times, err := decodeBody[[]*forgejo.TrackedTime](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(times))
	for _, tracked := range times {
		if tracked != nil {
			rows = append(rows, map[string]any{"id": tracked.ID, "seconds": tracked.Time, "user": tracked.UserName, "created": tracked.Created})
		}
	}
	return writeRows(stdout, "tracked_time", rows, []string{"id", "seconds", "user", "created"}, responseTotal(resp), fmt.Sprintf("0 tracked time entries found for issue %s", rest[0]), nil)
}

func runIssueTimeAdd(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "issue time add", []string{"--seconds", "--user", "--created", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--seconds", "--user", "--created"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	body := map[string]any{}
	var secondsRaw string
	var ok bool
	var err error
	args, secondsRaw, ok, err = takeValueFlag(args, "--seconds")
	if err != nil {
		return err
	}
	if !ok {
		return newUsageError("issue time add requires --seconds")
	}
	seconds, err := parseInt64Value("--seconds", secondsRaw)
	if err != nil {
		return err
	}
	body["time"] = seconds
	for _, spec := range []struct{ flag, key string }{{"--user", "user_name"}, {"--created", "created"}} {
		args, err = takeStringBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo issue time add [owner/repo] <number> --seconds <n> --yes")
	}
	if !yes {
		return newUsageError("issue time add requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueAddTime", ref, map[string]string{"index": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID("issueAddTime")
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, map[string]string{"index": rest[0]}), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "tracked_time", false)
}

func runIssueTimeReset(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	return runIssueTimeDeleteLike(ctx, client, cfg, args, stdout, "issueResetTime", "issue time reset", false)
}

func runIssueTimeDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	return runIssueTimeDeleteLike(ctx, client, cfg, args, stdout, "issueDeleteTime", "issue time delete", true)
}

func runIssueTimeDeleteLike(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, operation, command string, needsID bool) error {
	if err := rejectUnknownFlags(args, command, []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || (!needsID && len(rest) != 1) || (needsID && len(rest) != 2) {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> [time-id] --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	extra := map[string]string{"index": rest[0]}
	if needsID {
		extra["id"] = rest[1]
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, extra, nil, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, extra), forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"issue": rest[0], "tracked_time": command, "ok": true})
	}
	if len(out) != 0 {
		return writeJSONAsTOON(stdout, out, "tracked_time", false)
	}
	return writeTOON(stdout, map[string]any{"tracked_time": "ok"})
}

func runPRReviews(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr reviews", []string{"--json", "--limit", "--page"}, []string{"--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	query := url.Values{"limit": {defaultListLimit}}
	var err error
	for _, spec := range []struct{ flag, key string }{{"--limit", "limit"}, {"--page", "page"}} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr reviews [owner/repo] <number>")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListPullReviews", repoPathWith(ref, map[string]string{"index": rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	reviews, err := decodeBody[[]*forgejo.PullReview](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(reviews))
	for _, review := range reviews {
		if review != nil {
			state := ""
			if review.State != nil {
				state = string(*review.State)
			}
			rows = append(rows, map[string]any{"id": review.ID, "state": state, "author": userName(review.User), "comments": review.CodeCommentsCount, "submitted": review.Submitted})
		}
	}
	return writeRows(stdout, "reviews", rows, []string{"id", "state", "author", "comments", "submitted"}, responseTotal(resp), fmt.Sprintf("0 reviews found for PR %s", rest[0]), nil)
}

func pullReviewsTable(reviews []*forgejo.PullReview) any {
	rows := make([]map[string]any, 0, len(reviews))
	for _, review := range reviews {
		if review == nil {
			continue
		}
		state := ""
		if review.State != nil {
			state = string(*review.State)
		}
		rows = append(rows, map[string]any{
			"id":        review.ID,
			"state":     state,
			"author":    userName(review.User),
			"comments":  review.CodeCommentsCount,
			"submitted": review.Submitted,
		})
	}
	if len(rows) == 0 {
		return map[string]any{"reviews": "0 reviews found"}
	}
	return tableBlock("reviews", []string{"id", "state", "author", "comments", "submitted"}, rows)
}

func fieldsWith(fields []string, extra string) []string {
	if slicesContains(fields, extra) {
		return fields
	}
	out := append([]string{}, fields...)
	return append(out, extra)
}

func runPRReviewRequests(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, add bool) error {
	action := "add"
	operation := "repoCreatePullReviewRequests"
	if !add {
		action = "remove"
		operation = "repoDeletePullReviewRequests"
	}
	command := "pr review-requests " + action
	if err := rejectUnknownFlags(args, command, []string{"--reviewer", "--team", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--reviewer", "--team"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, reviewers, err := takeAllValueFlags(args, "--reviewer")
	if err != nil {
		return err
	}
	args, teams, err := takeAllValueFlags(args, "--team")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <number> --reviewer <user> [--team team] --yes")
	}
	if len(reviewers) == 0 && len(teams) == 0 {
		return newUsageError(command + " requires --reviewer or --team")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	body := map[string]any{"reviewers": flattenCSV(reviewers), "team_reviewers": flattenCSV(teams)}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, map[string]string{"index": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, map[string]string{"index": rest[0]}), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	if len(out) == 0 {
		return writeTOON(stdout, map[string]any{"review_requests": action + " ok"})
	}
	return writeJSONAsTOON(stdout, out, "review_requests", false)
}

func runPRReviewComment(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr review-comment", []string{"--body", "--body-file", "-body", "--path", "--new-line", "--old-line", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--body", "--body-file", "-body", "--path", "--new-line", "--old-line"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, bodyText, _, err := takeBodyText(args, true)
	if err != nil {
		return err
	}
	body := map[string]any{"body": bodyText}
	args, err = takeStringBodyFlag(args, "--path", "path", body)
	if err != nil {
		return err
	}
	args, err = takeIntBodyFlag(args, "--new-line", "new_position", body)
	if err != nil {
		return err
	}
	args, err = takeIntBodyFlag(args, "--old-line", "old_position", body)
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 2 {
		return newUsageError("usage: fjgo pr review-comment [owner/repo] <number> <review-id> --path <file> --new-line <n> --body <text> --yes")
	}
	if body["path"] == nil || (body["new_position"] == nil && body["old_position"] == nil) {
		return newUsageError("pr review-comment requires --path and --new-line or --old-line")
	}
	if !yes {
		return newUsageError("pr review-comment requires --yes")
	}
	extra := map[string]string{"index": rest[0], "id": rest[1]}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoCreatePullReviewComment", ref, extra, body, yes)
	}
	op, _ := forgejo.OperationByID("repoCreatePullReviewComment")
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, extra), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "review_comment", false)
}

func runPRUpdate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "pr update", []string{"--style", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--style"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	style := ""
	var err error
	args, style, _, err = takeValueFlag(args, "--style")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr update [owner/repo] <number> [--style merge|rebase] --yes")
	}
	if !yes {
		return newUsageError("pr update requires --yes")
	}
	query := url.Values{}
	if style != "" {
		query.Set("style", style)
	}
	if dryRun {
		op, _ := forgejo.OperationByID("repoUpdatePullRequest")
		return writeRequestPreview(stdout, requestPreview{Operation: op.ID, Method: op.Method, Path: previewPath(op, repoPathWith(ref, map[string]string{"index": rest[0]})), Query: query, AuthPresent: clientHasAuth(client), RequiresYes: true, YesProvided: yes})
	}
	resp, err := rawOperationResponse(ctx, client, "repoUpdatePullRequest", repoPathWith(ref, map[string]string{"index": rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	if len(resp.Body) == 0 {
		return writeTOON(stdout, map[string]any{"pull": fmt.Sprintf("#%s updated", rest[0])})
	}
	return writeJSONAsTOON(stdout, resp.Body, "pull", false)
}

func runPRDiffPatch(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, diffType string) error {
	if err := rejectUnknownFlags(args, "pr "+diffType, []string{"--binary", "--full", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	args, binary := boolFlag(args, "--binary")
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo pr " + diffType + " [owner/repo] <number> [--binary] [--full]")
	}
	query := url.Values{}
	if binary {
		query.Set("binary", "true")
	}
	resp, err := rawOperationResponse(ctx, client, "repoDownloadPullDiffOrPatch", repoPathWith(ref, map[string]string{"index": rest[0], "diffType": diffType}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{diffType: string(resp.Body)})
	}
	text := string(resp.Body)
	truncated := !full && len([]rune(text)) > defaultTruncateChars
	if truncated {
		var report truncateReport
		text = truncateString(text, defaultTruncateChars, diffType, &report)
	}
	blocks := toonBlocks{map[string]any{diffType: text}}
	if truncated {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `fjgo --repo %s/%s pr %s %s --full` to see complete output", ref.Owner, ref.Repo, diffType, rest[0])}))
	}
	return writeTOON(stdout, blocks)
}

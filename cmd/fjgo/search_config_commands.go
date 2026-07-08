package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

func searchHelp() string {
	return `usage: fjgo search <issues|prs|repos|topics> <query> [flags]

flags:
  --repo <owner/repo>          limit issue/PR search to one repository
  --owner <owner>, --state <open|closed|all>, --labels <a,b> for issues/PRs
  --sort <key>, --limit <n>, --page <n>
  --fields <a,b,c>, --json

examples:
  fjgo -R origin search issues "login" --state open
  fjgo search prs "fix" --repo OWNER/REPO
  fjgo search repos "forgejo cli" --limit 20
  fjgo search topics "actions"`
}

func runSearch(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, searchHelp())
	}
	switch args[0] {
	case "issues":
		return runSearchIssues(ctx, client, cfg, args[1:], stdout, "issues")
	case "prs", "pulls":
		return runSearchIssues(ctx, client, cfg, args[1:], stdout, "pulls")
	case "repos":
		return runSearchRepos(ctx, client, args[1:], stdout)
	case "topics":
		return runSearchTopics(ctx, client, args[1:], stdout)
	default:
		return fmt.Errorf("unknown search type %q", args[0])
	}
}

func runSearchIssues(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, typ string) error {
	if err := rejectUnknownFlags(args, "search "+typ, []string{"--repo", "--owner", "--state", "--labels", "--sort", "--limit", "--page", "--fields", "--json"}, []string{"--repo", "--owner", "--state", "--labels", "--sort", "--limit", "--page", "--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	searchFields := append([]string{"repo"}, issueFields...)
	fields, err := validatedFields(fieldsArg, issueListDefaultFields, searchFields, "search "+typ)
	if err != nil {
		return err
	}
	var repoArg string
	var repoSet bool
	args, repoArg, repoSet, err = takeValueFlag(args, "--repo")
	if err != nil {
		return err
	}
	query := url.Values{"type": {typ}, "limit": {defaultListLimit}}
	for _, spec := range []struct {
		flag string
		key  string
	}{
		{"--owner", "owner"},
		{"--state", "state"},
		{"--labels", "labels"},
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
	searchQuery := strings.Join(args, " ")
	if searchQuery != "" {
		query.Set("q", searchQuery)
	}
	if searchQuery == "" && query.Get("state") == "" && query.Get("labels") == "" {
		return newUsageError("search query or filters required", "Run `fjgo search issues \"text\" --state open`")
	}
	if repoSet || cfg.RemoteRepo != nil {
		ref := repoRef{}
		if repoSet {
			owner, repo, err := splitRepo(repoArg)
			if err != nil {
				return err
			}
			ref = repoRef{Owner: owner, Repo: repo}
		} else {
			ref = *cfg.RemoteRepo
		}
		localQuery := url.Values{"type": {typ}, "limit": {query.Get("limit")}}
		for _, key := range []string{"state", "labels", "sort", "page", "q"} {
			if value := query.Get(key); value != "" {
				localQuery.Set(key, value)
			}
		}
		resp, err := rawOperationResponse(ctx, client, "issueListIssues", repoPath(ref), localQuery, nil)
		if err != nil {
			return err
		}
		items, err := decodeBody[[]*forgejo.Issue](resp.Body)
		if err != nil {
			return err
		}
		if jsonOut {
			return writeJSON(stdout, items)
		}
		rows := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if item != nil {
				rows = append(rows, issueRow(item, false))
			}
		}
		return writeRows(stdout, typ, rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 %s found for %s/%s", typ, ref.Owner, ref.Repo), nil)
	}
	resp, err := rawOperationResponse(ctx, client, "issueSearchIssues", nil, query, nil)
	if err != nil {
		return err
	}
	items, err := decodeBody[[]*forgejo.Issue](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, items)
	}
	rows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item != nil {
			row := issueRow(item, false)
			if item.Repository != nil {
				row["repo"] = item.Repository.FullName
			}
			rows = append(rows, row)
		}
	}
	outFields := fields
	if !slicesContains(outFields, "repo") {
		outFields = append([]string{"repo"}, outFields...)
	}
	return writeRows(stdout, typ, rowsSelect(rows, outFields), outFields, responseTotal(resp), "0 "+typ+" found", nil)
}

func runSearchRepos(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "search repos", []string{"--sort", "--limit", "--page", "--fields", "--json"}, []string{"--sort", "--limit", "--page", "--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"name", "description", "stars", "forks"}
	available := []string{"name", "description", "stars", "forks", "updated", "private", "archived", "url"}
	fields, err := validatedFields(fieldsArg, defaults, available, "search repos")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {defaultListLimit}}
	for _, spec := range []struct {
		flag string
		key  string
	}{
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
	if q := strings.Join(args, " "); q != "" {
		query.Set("q", q)
	}
	resp, err := rawOperationResponse(ctx, client, "repoSearch", nil, query, nil)
	if err != nil {
		return err
	}
	results, err := decodeBody[forgejo.SearchResults](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, results)
	}
	rows := make([]map[string]any, 0, len(results.Data))
	for _, repo := range results.Data {
		if repo == nil {
			continue
		}
		rows = append(rows, map[string]any{
			"name":        repo.FullName,
			"description": repo.Description,
			"stars":       repo.Stars,
			"forks":       repo.Forks,
			"updated":     repo.Updated,
			"private":     repo.Private,
			"archived":    repo.Archived,
			"url":         repo.HTMLURL,
		})
	}
	return writeRows(stdout, "repos", rowsSelect(rows, fields), fields, responseTotal(resp), "0 repositories found", nil)
}

func runSearchTopics(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "search topics", []string{"--limit", "--page", "--json"}, []string{"--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	query := url.Values{"limit": {defaultListLimit}}
	var err error
	for _, spec := range []struct {
		flag string
		key  string
	}{
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
	q := strings.Join(args, " ")
	if q == "" {
		return newUsageError("search topics requires a query")
	}
	query.Set("q", q)
	resp, err := rawOperationResponse(ctx, client, "topicSearch", nil, query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	return writeJSONAsTOON(stdout, resp.Body, "topics", false)
}

func labelHelp() string {
	return `usage: fjgo label <list|create|edit|delete> [owner/repo] [flags]

examples:
  fjgo -R origin label list
  fjgo -R origin label create --name bug --color ff0000 --dry-run --yes
  fjgo -R origin label edit 42 --name defect --dry-run --yes
  fjgo -R origin label delete 42 --yes`
}

func runLabel(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, labelHelp())
	}
	switch args[0] {
	case "list":
		return runLabelList(ctx, client, cfg, args[1:], stdout)
	case "create":
		return runLabelCreate(ctx, client, cfg, args[1:], stdout)
	case "edit":
		return runLabelEdit(ctx, client, cfg, args[1:], stdout)
	case "delete":
		return runLabelDelete(ctx, client, cfg, args[1:], stdout)
	default:
		return fmt.Errorf("unknown label command %q", args[0])
	}
}

func runLabelList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "label list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"id", "name", "color"}
	available := []string{"id", "name", "color", "description", "exclusive", "archived"}
	fields, err := validatedFields(fieldsArg, defaults, available, "label list")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {"100"}}
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
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo label list [owner/repo]")
	}
	resp, err := rawOperationResponse(ctx, client, "issueListLabels", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	labels, err := decodeBody[[]*forgejo.Label](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, labels)
	}
	rows := make([]map[string]any, 0, len(labels))
	for _, label := range labels {
		if label == nil {
			continue
		}
		rows = append(rows, labelRow(label))
	}
	return writeRows(stdout, "labels", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 labels found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "label",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runLabelCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	return runLabelMutate(ctx, client, cfg, args, stdout, false)
}

func runLabelEdit(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	return runLabelMutate(ctx, client, cfg, args, stdout, true)
}

func runLabelMutate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, edit bool) error {
	command := "label create"
	operation := "issueCreateLabel"
	if edit {
		command = "label edit"
		operation = "issueEditLabel"
	}
	if err := rejectUnknownFlags(args, command, []string{"--name", "--color", "--description", "--exclusive", "--archived", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--name", "--color", "--description", "--exclusive", "--archived"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	values := map[string]string{}
	var err error
	for _, flag := range []string{"--name", "--color", "--description", "--exclusive", "--archived"} {
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
	if err != nil || (!edit && len(rest) != 0) || (edit && len(rest) != 1) {
		return newUsageError("usage: fjgo " + command + " [owner/repo] [id] --name <name> --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	if !edit && values["--name"] == "" {
		return newUsageError("label create requires --name")
	}
	body := map[string]any{}
	for key, value := range values {
		switch key {
		case "--name":
			body["name"] = value
		case "--color":
			body["color"] = strings.TrimPrefix(value, "#")
		case "--description":
			body["description"] = value
		case "--exclusive":
			parsed, err := parseBoolValue(key, value)
			if err != nil {
				return err
			}
			body["exclusive"] = parsed
		case "--archived":
			parsed, err := parseBoolValue(key, value)
			if err != nil {
				return err
			}
			body["is_archived"] = parsed
		}
	}
	extraPath := map[string]string{}
	if edit {
		extraPath["id"] = rest[0]
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, extraPath, body, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, mergePath(repoPath(ref), extraPath), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(out)
		return err
	}
	return writeJSONAsTOON(stdout, out, "label", false)
}

func runLabelDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "label delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo label delete [owner/repo] <id> --yes")
	}
	if !yes {
		return newUsageError("label delete requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "issueDeleteLabel", ref, map[string]string{"id": rest[0]}, nil, yes)
	}
	if err := client.IssueDeleteLabel(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"deleted": true, "id": rest[0]})
	}
	return writeTOON(stdout, map[string]any{"label": fmt.Sprintf("%s deleted", rest[0])})
}

func labelRow(label *forgejo.Label) map[string]any {
	return map[string]any{
		"id":          label.ID,
		"name":        label.Name,
		"color":       label.Color,
		"description": label.Description,
		"exclusive":   label.Exclusive,
		"archived":    label.IsArchived,
	}
}

func mergePath(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range a {
		out[key] = value
	}
	for key, value := range b {
		out[key] = value
	}
	return out
}

func slicesContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

var runFields = []string{"id", "number", "title", "status", "workflow", "ref", "event", "created", "updated", "url", "sha"}
var runDefaultFields = []string{"id", "number", "title", "status", "workflow"}

func runHelp() string {
	return `usage: fjgo run <subcommand> [owner/repo] [flags]

subcommands:
  list [owner/repo]             list Forgejo Actions runs
  view [owner/repo] <id>        show an Actions run
  watch [owner/repo] <id>       poll until a run reaches a terminal state

flags:
  --status <status>, --workflow <file>, --ref <ref>, --event <event>, --sha <sha>, --limit <n>, --page <n>
  --fields <a,b,c>, --json
  --log-failed                  show failed tasks exposed by the Actions tasks API
  --interval <duration>, --timeout <duration>, --full

examples:
  fjgo -R origin run list --status failure
  fjgo -R origin run view 123
  fjgo -R origin run watch 123 --timeout 2m
  fjgo -R origin run view 123 --log-failed`
}

func runActions(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, runHelp())
	}
	switch args[0] {
	case "list":
		return runActionList(ctx, client, cfg, args[1:], stdout)
	case "view":
		return runActionView(ctx, client, cfg, args[1:], stdout)
	case "watch":
		return runActionWatch(ctx, client, cfg, args[1:], stdout)
	default:
		return fmt.Errorf("unknown run command %q", args[0])
	}
}

func runActionList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "run list", []string{"--json", "--fields", "--status", "--workflow", "--ref", "--event", "--sha", "--limit", "--page"}, []string{"--fields", "--status", "--workflow", "--ref", "--event", "--sha", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, runDefaultFields, runFields, "run list")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {defaultListLimit}}
	for _, spec := range []struct {
		flag string
		key  string
	}{
		{"--status", "status"},
		{"--workflow", "workflow_id"},
		{"--ref", "ref"},
		{"--event", "event"},
		{"--sha", "head_sha"},
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
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo run list [owner/repo] [flags]", "Pass `owner/repo` or use `-R origin`")
	}
	out, err := client.ListActionRuns(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{Query: query})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	var runs []*forgejo.ActionRun
	var total int64
	if out != nil {
		runs = out.Entries
		total = out.TotalCount
	}
	rows := make([]map[string]any, 0, len(runs))
	for _, run := range runs {
		if run != nil {
			rows = append(rows, runRow(run))
		}
	}
	return writeRows(stdout, "runs", rowsSelect(rows, fields), fields, total, fmt.Sprintf("0 action runs found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "run",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runActionView(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "run view", []string{"--json", "--fields", "--log-failed", "--full"}, []string{"--fields"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	_ = full
	args, logFailed := boolFlag(args, "--log-failed")
	var err error
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, runDefaultFields, runFields, "run view")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo run view [owner/repo] <id> [--log-failed]", "Pass `owner/repo` or use `-R origin`")
	}
	run, err := client.ActionRun(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut && !logFailed {
		return writeJSON(stdout, run)
	}
	blocks := toonBlocks{map[string]any{"run": selectFields(runRow(run), fields)}}
	if logFailed {
		tasks, err := failedTasksForRun(ctx, client, ref, run)
		if err != nil {
			return err
		}
		blocks = append(blocks, taskTable(tasks))
	}
	return writeTOON(stdout, blocks)
}

func runActionWatch(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "run watch", []string{"--json", "--fields", "--interval", "--timeout"}, []string{"--fields", "--interval", "--timeout"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, runDefaultFields, runFields, "run watch")
	if err != nil {
		return err
	}
	interval := 5 * time.Second
	timeout := 10 * time.Minute
	var value string
	var ok bool
	args, value, ok, err = takeValueFlag(args, "--interval")
	if err != nil {
		return err
	}
	if ok {
		interval, err = time.ParseDuration(value)
		if err != nil || interval <= 0 {
			return newUsageError("--interval expects a positive duration")
		}
	}
	args, value, ok, err = takeValueFlag(args, "--timeout")
	if err != nil {
		return err
	}
	if ok {
		timeout, err = time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			return newUsageError("--timeout expects a positive duration")
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo run watch [owner/repo] <id> [--timeout 10m]")
	}
	deadline := time.Now().Add(timeout)
	var run *forgejo.ActionRun
	for {
		run, err = client.ActionRun(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		if runTerminal(run) || time.Now().Add(interval).After(deadline) {
			break
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if jsonOut {
		return writeJSON(stdout, run)
	}
	blocks := toonBlocks{map[string]any{"run": selectFields(runRow(run), fields)}}
	if !runTerminal(run) {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `%s` again to continue watching", commandForRepo(refPtr(ref), "run watch "+rest[0]))}))
	}
	return writeTOON(stdout, blocks)
}

func runTerminal(run *forgejo.ActionRun) bool {
	if run == nil {
		return false
	}
	switch strings.ToLower(run.Status) {
	case "success", "failure", "cancelled", "canceled", "skipped", "timed_out", "timed-out":
		return true
	default:
		return false
	}
}

func failedTasksForRun(ctx context.Context, client *forgejo.Client, ref repoRef, run *forgejo.ActionRun) ([]*forgejo.ActionTask, error) {
	out, err := client.ListActionTasks(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{Query: url.Values{"status": {"failure"}, "limit": {"100"}}})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, nil
	}
	var tasks []*forgejo.ActionTask
	for _, task := range out.Entries {
		if task == nil {
			continue
		}
		if run == nil || run.Index == 0 || task.RunNumber == run.Index {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func taskTable(tasks []*forgejo.ActionTask) any {
	rows := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, map[string]any{
			"id":       task.ID,
			"title":    task.DisplayTitle,
			"name":     task.Name,
			"status":   task.Status,
			"workflow": task.WorkflowID,
			"branch":   task.HeadBranch,
			"sha":      shortSHA(task.HeadSHA),
		})
	}
	if len(rows) == 0 {
		return map[string]any{"failed_tasks": "0 failed tasks found"}
	}
	return tableBlock("failed_tasks", []string{"id", "title", "name", "status", "workflow", "branch", "sha"}, rows)
}

func runRow(run *forgejo.ActionRun) map[string]any {
	if run == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":       run.ID,
		"number":   run.Index,
		"title":    run.Title,
		"status":   run.Status,
		"workflow": run.WorkflowID,
		"ref":      run.PrettyRef,
		"event":    run.Event,
		"created":  run.Created,
		"updated":  run.Updated,
		"url":      run.HTMLURL,
		"sha":      shortSHA(run.CommitSHA),
	}
}

func workflowHelp() string {
	return `usage: fjgo workflow <subcommand> [owner/repo] [flags]

subcommands:
  list [owner/repo]                  list .forgejo/workflows files
  view [owner/repo] <workflow.yml>   show workflow file content
  run [owner/repo] <workflow.yml>    dispatch a workflow

flags:
  --ref <ref>                        git ref for list or dispatch
  --input <key=value>                workflow dispatch input; repeatable
  --json
  --yes, --dry-run, --print-request

examples:
  fjgo -R origin workflow list
  fjgo -R origin workflow view verify.yml --full
  fjgo -R origin workflow run verify.yml --ref main --input smoke=true --dry-run --yes`
}

func runWorkflow(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, workflowHelp())
	}
	switch args[0] {
	case "list":
		return runWorkflowList(ctx, client, cfg, args[1:], stdout)
	case "view":
		return runWorkflowView(ctx, client, cfg, args[1:], stdout)
	case "run":
		return runWorkflowDispatch(ctx, client, cfg, args[1:], stdout)
	default:
		return fmt.Errorf("unknown workflow command %q", args[0])
	}
}

func runWorkflowList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "workflow list", []string{"--ref", "--json"}, []string{"--ref"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	var refName string
	var ok bool
	var err error
	args, refName, ok, err = takeValueFlag(args, "--ref")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo workflow list [owner/repo] [--ref branch]")
	}
	query := url.Values{}
	if ok {
		query.Set("ref", refName)
	}
	resp, err := rawOperationResponse(ctx, client, "repoGetContents", repoPathWith(ref, map[string]string{"filepath": ".forgejo/workflows"}), query, nil)
	if err != nil {
		var httpErr forgejo.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return writeRows(stdout, "workflows", nil, []string{"name", "path", "type"}, 0, fmt.Sprintf("0 workflows found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
				Domain: "workflow",
				Action: "list",
				Empty:  true,
				Repo:   refPtr(ref),
			}))
		}
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	rows, err := workflowRows(resp.Body)
	if err != nil {
		return err
	}
	return writeRows(stdout, "workflows", rows, []string{"name", "path", "type"}, responseTotal(resp), fmt.Sprintf("0 workflows found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "workflow",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runWorkflowView(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "workflow view", []string{"--ref", "--json", "--full"}, []string{"--ref"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	refName := ""
	var err error
	args, refName, _, err = takeValueFlag(args, "--ref")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo workflow view [owner/repo] <workflow.yml> [--ref branch]")
	}
	query := url.Values{}
	if refName != "" {
		query.Set("ref", refName)
	}
	resp, err := rawOperationResponse(ctx, client, "repoGetContents", repoPathWith(ref, map[string]string{"filepath": ".forgejo/workflows/" + rest[0]}), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	view, err := workflowFileView(resp.Body, full)
	if err != nil {
		return err
	}
	blocks := toonBlocks{map[string]any{"workflow": view}}
	if truncated, _ := view["truncated"].(bool); truncated {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `%s` to see complete workflow content", commandForRepo(refPtr(ref), "workflow view "+rest[0]+" --full"))}))
	}
	return writeTOON(stdout, blocks)
}

func runWorkflowDispatch(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "workflow run", []string{"--ref", "--input", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--ref", "--input"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	refName := ""
	var err error
	args, refName, _, err = takeValueFlag(args, "--ref")
	if err != nil {
		return err
	}
	args, inputsRaw, err := takeAllValueFlags(args, "--input")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo workflow run [owner/repo] <workflow.yml> --ref <ref> --yes")
	}
	if refName == "" {
		return newUsageError("workflow run requires --ref")
	}
	if !yes {
		return newUsageError("workflow run requires --yes")
	}
	inputs := map[string]string{}
	for _, item := range inputsRaw {
		key, value, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			return newUsageError("--input expects key=value")
		}
		inputs[key] = value
	}
	body := &forgejo.DispatchWorkflowOption{Ref: refName, Inputs: inputs, ReturnRunInfo: true}
	if dryRun {
		return writeMutationPreview(stdout, client, "DispatchWorkflow", ref, map[string]string{"workflowfilename": rest[0]}, body, yes)
	}
	out, err := client.DispatchWorkflow(ctx, ref.Owner, ref.Repo, rest[0], body, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	return writeTOON(stdout, map[string]any{"workflow_run": out})
}

func workflowFileView(body []byte, full bool) (map[string]any, error) {
	var item struct {
		Name     string `json:"name"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		Size     int64  `json:"size"`
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		HTMLURL  string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &item); err != nil {
		return nil, err
	}
	content := item.Content
	if strings.EqualFold(item.Encoding, "base64") {
		decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(item.Content, "\n", ""))
		if err != nil {
			return nil, err
		}
		content = string(decoded)
	}
	truncated := false
	if !full && len([]rune(content)) > defaultTruncateChars {
		var report truncateReport
		content = truncateString(content, defaultTruncateChars, "workflow.content", &report)
		truncated = report.Truncated()
	}
	return map[string]any{
		"name":      item.Name,
		"path":      item.Path,
		"type":      item.Type,
		"size":      item.Size,
		"url":       item.HTMLURL,
		"content":   content,
		"truncated": truncated,
	}, nil
}

func workflowRows(raw []byte) ([]map[string]any, error) {
	var many []*forgejo.ContentsResponse
	if err := json.Unmarshal(raw, &many); err == nil {
		rows := make([]map[string]any, 0, len(many))
		for _, item := range many {
			if item == nil || !isWorkflowFile(item.Name) {
				continue
			}
			rows = append(rows, map[string]any{"name": item.Name, "path": item.Path, "type": item.Type})
		}
		return rows, nil
	}
	var one forgejo.ContentsResponse
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	if !isWorkflowFile(one.Name) {
		return nil, nil
	}
	return []map[string]any{{"name": one.Name, "path": one.Path, "type": one.Type}}, nil
}

func isWorkflowFile(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"repos.astrazds.net/astrazds/fjgo/internal/fjgoskill"
	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

const axiDescription = "Inspect and manage Forgejo repositories through a token-safe CLI for coding agents"

var repoViewFields = []string{"full_name", "default_branch", "private", "archived", "open_issues", "open_pulls", "releases", "url", "description", "stars", "forks"}
var repoViewDefaultFields = []string{"full_name", "default_branch", "private", "archived", "open_issues", "open_pulls"}

type usageError struct {
	Message string
	Help    []string
}

func (e usageError) Error() string {
	return e.Message
}

func newUsageError(message string, help ...string) error {
	return usageError{Message: message, Help: help}
}

func isUsage(err error) bool {
	var u usageError
	if errors.As(err, &u) {
		return true
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "usage:") ||
		strings.HasPrefix(msg, "unknown ") ||
		strings.HasPrefix(msg, "missing ") ||
		strings.HasPrefix(msg, "expected ") ||
		strings.Contains(msg, " requires --yes") ||
		strings.Contains(msg, " requires -body") ||
		strings.Contains(msg, "invalid value")
}

func errorHelp(err error) []string {
	var u usageError
	if errors.As(err, &u) {
		return u.Help
	}
	msg := err.Error()
	switch {
	case forgejoStatus(err) == http.StatusUnauthorized:
		return suggestionLines(suggestionContext{Domain: "error", Action: "auth"})
	case forgejoStatus(err) == http.StatusForbidden:
		return suggestionLines(suggestionContext{Domain: "error", Action: "forbidden"})
	case forgejoStatus(err) == http.StatusNotFound:
		return suggestionLines(suggestionContext{Domain: "error", Action: "not_found"})
	case forgejoStatus(err) == http.StatusConflict:
		return []string{"Refresh the target state, then retry only if the requested change is still needed"}
	case forgejoStatus(err) == http.StatusUnprocessableEntity:
		return suggestionLines(suggestionContext{Domain: "error", Action: "validation"})
	case forgejoStatus(err) == http.StatusTooManyRequests:
		return []string{"Wait for the Forgejo API rate limit window to reset, then retry"}
	case strings.Contains(msg, "requires --yes"):
		return []string{"Rerun the same command with `--dry-run --yes` first, then remove `--dry-run` when the target is correct"}
	case strings.Contains(msg, "requires -body"):
		return []string{"Run `fjgo api inspect <operationId>` or `fjgo model inspect <Model>` to build the request body"}
	case strings.Contains(msg, "missing path parameter"):
		return []string{"Run `fjgo api inspect <operationId>` to see required path parameters"}
	case strings.Contains(msg, "missing owner/repo"):
		return suggestionLines(suggestionContext{Domain: "error", Action: "missing_repo"})
	default:
		return nil
	}
}

func unknownFlagError(command, flag string, valid []string) error {
	help := fmt.Sprintf("valid flags for `%s`: %s (--help always allowed)", command, strings.Join(valid, ", "))
	if len(valid) == 0 {
		help = fmt.Sprintf("`%s` accepts no flags except --help", command)
	}
	return newUsageError(fmt.Sprintf("unknown flag %s for `%s`", flag, command), help)
}

func unknownSubcommandError(command, got string, valid []string) error {
	return newUsageError(
		fmt.Sprintf("unknown %s command %q", command, got),
		fmt.Sprintf("valid subcommands for `%s`: %s", command, strings.Join(valid, ", ")),
	)
}

func unknownCommandError(got string, valid []string) error {
	return newUsageError(
		fmt.Sprintf("unknown command %q", got),
		"valid commands: "+strings.Join(valid, ", "),
	)
}

func rootFlags() []string {
	return []string{"-R", "-base-url", "-timeout", "-token", "--help", "--host", "--json", "--repo", "--repo-from-remote", "--version"}
}

func rootValueFlags() []string {
	return []string{"-R", "-base-url", "-timeout", "-token", "--host", "--repo", "--repo-from-remote"}
}

func rootCommands() []string {
	return []string{
		"setup", "hook", "doctor", "context", "version", "me", "auth", "whoami",
		"get", "api", "alias", "model", "repo", "issue", "pr", "pull", "run",
		"workflow", "search", "label", "secret", "variable", "release", "update",
		"skill", "install",
	}
}

func rejectUnknownRootFlags(args []string) error {
	allowed := rootFlags()
	valueFlags := rootValueFlags()
	allowedSet := map[string]bool{"-h": true}
	valueSet := map[string]bool{}
	for _, flag := range allowed {
		allowedSet[flag] = true
	}
	for _, flag := range valueFlags {
		valueSet[flag] = true
	}
	valid := slices.Clone(allowed)
	sortFlags(valid)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			return nil
		}
		name := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			name = before
		}
		if !allowedSet[name] {
			return unknownFlagError("fjgo", name, valid)
		}
		if valueSet[name] && !strings.Contains(arg, "=") {
			i++
		}
	}
	return nil
}

func rejectUnknownFlags(args []string, command string, allowed, valueFlags []string) error {
	allowedSet := map[string]bool{"--help": true}
	valueSet := map[string]bool{}
	for _, flag := range allowed {
		allowedSet[flag] = true
	}
	for _, flag := range valueFlags {
		valueSet[flag] = true
	}
	valid := slices.Clone(allowed)
	sortFlags(valid)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		name := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			name = before
		}
		if !allowedSet[name] {
			return unknownFlagError(command, name, valid)
		}
		if valueSet[name] && !strings.Contains(arg, "=") {
			i++
		}
	}
	return nil
}

func parseKnownFlagSet(fs *flag.FlagSet, args []string, command string, allowed, valueFlags []string) error {
	if err := rejectUnknownFlags(args, command, allowed, valueFlags); err != nil {
		return err
	}
	if err := fs.Parse(args); err != nil {
		return newUsageError(err.Error())
	}
	return nil
}

func sortFlags(flags []string) {
	slices.Sort(flags)
}

func hasHelp(args []string) bool {
	return slices.Contains(args, "--help") || slices.Contains(args, "-h")
}

type commandHelpSpec struct {
	Usage    string
	Flags    []string
	Examples []string
}

func formatCommandHelp(spec commandHelpSpec) string {
	var b strings.Builder
	b.WriteString("usage: ")
	b.WriteString(spec.Usage)
	if len(spec.Flags) != 0 {
		b.WriteString("\n\nflags:\n")
		for _, line := range spec.Flags {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if len(spec.Examples) != 0 {
		if len(spec.Flags) != 0 {
			b.WriteString("\nexamples:\n")
		} else {
			b.WriteString("\n\nexamples:\n")
		}
		for _, line := range spec.Examples {
			b.WriteString("  ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func subcommandHelp(args []string, groupHelp string, specs map[string]commandHelpSpec) (string, bool) {
	if len(args) == 0 {
		return groupHelp, true
	}
	if !hasHelp(args) {
		return "", false
	}
	if spec, ok := specs[args[0]]; ok {
		return formatCommandHelp(spec), true
	}
	return groupHelp, true
}

type peeledContextFlags struct {
	Repo       string
	RepoSource string
	Remote     string
	Host       string
}

func peelContextFlags(args []string) ([]string, peeledContextFlags, error) {
	if hasHelp(args) {
		return args, peeledContextFlags{}, nil
	}
	if firstCommand(args) == "update" {
		return args, peeledContextFlags{}, nil
	}
	out := make([]string, 0, len(args))
	var ctx peeledContextFlags
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--repo":
			value, next, err := contextFlagValue(args, i, "--repo")
			if err != nil {
				return nil, ctx, err
			}
			ctx.Repo = value
			ctx.RepoSource = "--repo"
			i = next
		case strings.HasPrefix(arg, "--repo="):
			ctx.Repo = strings.TrimPrefix(arg, "--repo=")
			ctx.RepoSource = "--repo"
		case arg == "-R" || arg == "--repo-from-remote":
			value, next, err := contextFlagValue(args, i, arg)
			if err != nil {
				return nil, ctx, err
			}
			ctx.Remote = value
			i = next
		case strings.HasPrefix(arg, "-R="):
			ctx.Remote = strings.TrimPrefix(arg, "-R=")
		case strings.HasPrefix(arg, "--repo-from-remote="):
			ctx.Remote = strings.TrimPrefix(arg, "--repo-from-remote=")
		case arg == "--host":
			value, next, err := contextFlagValue(args, i, "--host")
			if err != nil {
				return nil, ctx, err
			}
			ctx.Host = value
			i = next
		case strings.HasPrefix(arg, "--host="):
			ctx.Host = strings.TrimPrefix(arg, "--host=")
		default:
			out = append(out, arg)
		}
	}
	return out, ctx, nil
}

func firstCommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return ""
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name := strings.TrimLeft(arg, "-")
			if before, _, ok := strings.Cut(name, "="); ok {
				name = before
			} else if rootFlagTakesValue(name) {
				i++
			}
			continue
		}
		return arg
	}
	return ""
}

func contextFlagValue(args []string, index int, name string) (string, int, error) {
	if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
		return "", index, newUsageError(name + " requires a value")
	}
	return args[index+1], index + 1, nil
}

func apiBaseURLFromHost(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", newUsageError("--host requires a hostname")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse host: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", newUsageError("--host must be a hostname or absolute URL")
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/api/v1"
	} else if !strings.HasSuffix(path, "/api/v1") {
		path += "/api/v1"
	}
	u.Path = path
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func takeFieldsFlag(args []string) ([]string, []string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--fields" {
			if i+1 == len(args) {
				return nil, nil, newUsageError("--fields requires a comma-separated value")
			}
			return append(out, args[i+2:]...), splitCSV(args[i+1]), nil
		}
		if strings.HasPrefix(arg, "--fields=") {
			_, value, _ := strings.Cut(arg, "=")
			return append(out, args[i+1:]...), splitCSV(value), nil
		}
		out = append(out, arg)
	}
	return out, nil, nil
}

func takeFullFlag(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	full := false
	for _, arg := range args {
		if arg == "--full" {
			full = true
			continue
		}
		out = append(out, arg)
	}
	return out, full
}

func writeHelp(w io.Writer, text string) error {
	_, err := io.WriteString(w, strings.TrimSpace(text)+"\n")
	return err
}

func rootHelp() string {
	return `usage: fjgo [flags] <command>
description: ` + axiDescription + `

commands:
  setup hooks                 install or repair agent session hooks
  version                     show Forgejo server version
  doctor [owner/repo]         redacted Forgejo/repo diagnostics
  me                          show authenticated user summary
  auth status                 show token-safe auth diagnostics
  get <path>                  GET an API path
  api <list|inspect|call|upload|raw>
  alias <list|inspect|collisions>
  model inspect <Model>
  repo <list|create|get|edit|fork|branches|collaborators|branch-protection|topics|avatar|issue>
  issue <list|view|create|edit|close|reopen|comment>
  pr <list|view|create|files|commits|checks|merge|review>
  run <list|view|watch>
  workflow <list|view|run>
  search <issues|prs|repos|topics>
  label <list|create|edit|delete>
  secret <list|set|delete>
  variable <list|get|set|delete>
  release <list|view|latest|create|edit|delete|upload|assets>
  skill <install|status>
  update [--check]             check for a newer fjgo release

global flags:
  -base-url <url>             Forgejo API base URL
  --host <host>               Forgejo host; derives https://host/api/v1
  -token <token>              Forgejo access token; prefer FJGO_TOKEN
  -timeout <duration>         HTTP timeout (default 15s)
  --repo <owner/repo>         explicit repo context; also supported via FJGO_REPO
  -R, --repo-from-remote <n>  resolve owner/repo from a Forgejo git remote
  --version                   show fjgo binary version
  --json                      output JSON for explicit JSON-capable surfaces/errors
  --help                      show this help

examples:
  fjgo -R origin
  fjgo issue list --repo OWNER/REPO --state open
  fjgo --host forgejo.example.com --repo OWNER/REPO repo get
  fjgo -R origin doctor
  fjgo -R origin issue list --state open
  fjgo -R origin pr list
  fjgo -R origin run list
  fjgo setup hooks`
}

func apiHelp() string {
	return `usage: fjgo api <list|inspect|call|upload|raw> [flags]

subcommands:
  list [filter]                 list generated Forgejo operations
  inspect <operationId>         show operation metadata
  call <operationId> [k=v ...]  call any non-upload operation
  upload <operationId> [k=v ...] attachment=@file --yes
  raw <METHOD> <path> [k=v ...] call an API path without an operation ID

flags:
  --json                        output JSON instead of TOON
  --fields <a,b,c>              list fields to include
  --full                        do not truncate long response strings
  -body <json|@file|->          request body for operations with body schema
  --yes                         required for mutating operations
  --dry-run, --print-request    print token-safe request preview

examples:
  fjgo api list release --fields id,method,path,summary
  fjgo api inspect repoSearch
  fjgo api call repoGet owner=OWNER repo=REPO
  fjgo api raw GET /repos/OWNER/REPO
  fjgo api upload repoCreateReleaseAttachment owner=OWNER repo=REPO id=1 attachment=@dist/app.tar.gz --yes`
}

func aliasHelp() string {
	return `usage: fjgo alias <list|inspect|collisions> [flags]

flags:
  --json              output JSON instead of TOON
  --fields <a,b,c>    list fields to include

examples:
  fjgo alias list repo
  fjgo alias inspect repo issues create
  fjgo alias collisions`
}

func repoHelp() string {
	return `usage: fjgo repo <list|create|get|edit|fork|branches|collaborators|branch-protection|topics|avatar|issue> [owner/repo] [flags]

subcommands:
  list                              list current-user, user, org, or search repos
  create <name>                     create a current-user or org repository
  get [owner/repo]                  show repository details
  edit [owner/repo]                 edit repository properties
  fork [owner/repo]                 fork a repository
  branches <list|get|create|delete>
  collaborators <list|check|permission|add|remove>
  branch-protection <list|get|create|edit|delete>
  topics [owner/repo]               list or replace repository topics
  avatar [owner/repo] <png>         update repository avatar
  issue <close|comment>             compatibility aliases

flags:
  --json                        output JSON where supported
  --full                        do not truncate long response strings
  --fields <a,b,c>              list fields to include
  --user <user>, --org <org>, --q <text> for repo list
  --name <name>, --description <text>, --private, --auto-init for repo create
  --private <bool>, --archived <bool>, --has-issues <bool>, --has-pulls <bool>, --has-wiki <bool> for repo edit
  --set <topic,topic>           replace topics for repo topics
  --yes                         required for mutating operations
  --dry-run, --print-request    print token-safe request preview

examples:
  fjgo --repo OWNER/REPO repo get
  fjgo repo list --org OWNER --fields name,private,archived
  fjgo repo create demo --private --dry-run --yes
  fjgo --repo OWNER/REPO repo branches list
  fjgo --repo OWNER/REPO repo collaborators add alice --permission write --dry-run --yes
  fjgo --repo OWNER/REPO repo branch-protection create --name main --required-approvals 1 --dry-run --yes
  fjgo repo topics OWNER/REPO --set forgejo,go --dry-run --yes`
}

func releaseHelp() string {
	return `usage: fjgo release <list|view|latest|create|edit|delete|upload|assets> [owner/repo] [flags]

subcommands:
  list [owner/repo]                  list releases
  view [owner/repo] <id|tag>         show release details
  latest [owner/repo]                show latest non-draft release
  create [owner/repo] <tag>          create a release
  edit [owner/repo] <id>             edit a release
  delete [owner/repo] <id|tag>       delete a release
  upload [owner/repo] <id> <file>    upload a release asset
  assets <list|delete>               list or delete release assets

flags:
  --json                        output JSON where supported
  --full                        do not truncate long response strings
  --name <text>, --target <ref>
  --body|--notes <text>, --body-file|--notes-file <path>
  --draft, --prerelease, --hide-archive-links for create
  --draft <bool>, --prerelease <bool>, --hide-archive-links <bool> for edit
  --fields <a,b,c>              list fields to include
  --yes                         required for create/edit/delete/upload/assets delete
  --dry-run, --print-request    print token-safe request preview

examples:
  fjgo --repo OWNER/REPO release list
  fjgo --repo OWNER/REPO release view v1.2.3
  fjgo --repo OWNER/REPO release create v1.2.3 --body-file notes.md --dry-run --yes
  fjgo --repo OWNER/REPO release edit 123 --prerelease false --dry-run --yes
  fjgo --repo OWNER/REPO release assets list 123
  fjgo --repo OWNER/REPO release upload 123 dist/app.tar.gz name=app.tar.gz --yes`
}

func setupHelp() string {
	return `usage: fjgo setup hooks [--check]
Install or repair agent SessionStart hooks for fjgo ambient context.

flags:
  --check      report hook target paths without writing

examples:
  fjgo setup hooks
  fjgo setup hooks --check`
}

func writeHome(ctx context.Context, client *forgejo.Client, cfg runConfig, stdout io.Writer) error {
	exe, _ := os.Executable()
	blocks := toonBlocks{
		map[string]any{
			"bin":         collapseHome(exe),
			"description": axiDescription,
			"base_url":    cfg.BaseURL,
		},
	}
	if cfg.RemoteRepo == nil {
		blocks = append(blocks,
			map[string]any{"repo": "none"},
			suggestionHelp(suggestionContext{Domain: "home", Action: "missing_repo"}),
		)
		return writeTOON(stdout, blocks)
	}
	ref := *cfg.RemoteRepo
	blocks = append(blocks, map[string]any{"repo": ref.Owner + "/" + ref.Repo})
	repo, err := client.RepoGet(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
	if err != nil {
		blocks = append(blocks, map[string]any{"repo_error": errorMessage(err)})
	} else if repo != nil {
		blocks = append(blocks, repoSummaryBlock(*repo))
	}
	issuesResp, issueErr := rawOperationResponse(ctx, client, "issueListIssues", repoPath(ref), url.Values{"state": {"open"}, "type": {"issues"}, "limit": {"3"}}, nil)
	if issueErr != nil {
		blocks = append(blocks, map[string]any{"issues_error": errorMessage(issueErr)})
	} else {
		issues, err := decodeBody[[]*forgejo.Issue](issuesResp.Body)
		if err != nil {
			blocks = append(blocks, map[string]any{"issues_error": errorMessage(err)})
		} else {
			blocks = append(blocks, issueListBlock("issues", issues, responseTotal(issuesResp))...)
		}
	}
	pullsResp, pullErr := rawOperationResponse(ctx, client, "repoListPullRequests", repoPath(ref), url.Values{"state": {"open"}, "limit": {"3"}}, nil)
	if pullErr != nil {
		blocks = append(blocks, map[string]any{"pulls_error": errorMessage(pullErr)})
	} else {
		pulls, err := decodeBody[[]*forgejo.PullRequest](pullsResp.Body)
		if err != nil {
			blocks = append(blocks, map[string]any{"pulls_error": errorMessage(err)})
		} else {
			blocks = append(blocks, pullListBlock("pulls", pulls, responseTotal(pullsResp))...)
		}
	}
	blocks = append(blocks, suggestionHelp(suggestionContext{Domain: "home", Action: "repo", Repo: &ref}))
	return writeTOON(stdout, blocks)
}

func repoSummaryBlock(repo forgejo.Repository) any {
	return map[string]any{
		"repository": map[string]any{
			"full_name":      repo.FullName,
			"default_branch": repo.DefaultBranch,
			"private":        repo.Private,
			"archived":       repo.Archived,
			"open_issues":    repo.OpenIssues,
			"open_pulls":     repo.OpenPulls,
			"releases":       repo.Releases,
		},
	}
}

func issueListBlock(label string, issues []*forgejo.Issue, total int64) toonBlocks {
	rows := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		author := ""
		if issue.User != nil {
			author = issue.User.UserName
		}
		state := ""
		if issue.State != nil {
			state = string(*issue.State)
		}
		rows = append(rows, map[string]any{
			"number": issue.Index,
			"title":  issue.Title,
			"state":  strings.ToLower(state),
			"author": author,
		})
	}
	if len(rows) == 0 {
		return toonBlocks{map[string]any{label: "0 open"}}
	}
	return homeListBlocks(label, []string{"number", "title", "state", "author"}, rows, total)
}

func pullListBlock(label string, pulls []*forgejo.PullRequest, total int64) toonBlocks {
	rows := make([]map[string]any, 0, len(pulls))
	for _, pr := range pulls {
		if pr == nil {
			continue
		}
		author := ""
		if pr.User != nil {
			author = pr.User.UserName
		}
		state := ""
		if pr.State != nil {
			state = string(*pr.State)
		}
		rows = append(rows, map[string]any{
			"number": pr.Index,
			"title":  pr.Title,
			"state":  strings.ToLower(state),
			"author": author,
		})
	}
	if len(rows) == 0 {
		return toonBlocks{map[string]any{label: "0 open"}}
	}
	return homeListBlocks(label, []string{"number", "title", "state", "author"}, rows, total)
}

func homeListBlocks(label string, fields []string, rows []map[string]any, total int64) toonBlocks {
	count := any(len(rows))
	if total > int64(len(rows)) {
		count = fmt.Sprintf("%d of %d total", len(rows), total)
	}
	return toonBlocks{
		map[string]any{label + "_count": count},
		tableBlock(label, fields, rows),
	}
}

func writeOperationsList(stdout io.Writer, ops []forgejo.Operation, fields []string) error {
	if len(fields) == 0 {
		fields = []string{"id", "method", "path"}
	}
	rows := make([]map[string]any, 0, len(ops))
	for _, op := range ops {
		row := map[string]any{
			"id":      op.ID,
			"method":  op.Method,
			"path":    op.Path,
			"summary": op.Summary,
			"returns": op.ReturnType,
			"body":    op.BodyType,
			"upload":  op.Upload,
		}
		rows = append(rows, selectFields(row, fields))
	}
	if len(rows) == 0 {
		return writeTOON(stdout, map[string]any{"operations": "0 operations found"})
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"count": len(rows)},
		tableBlock("operations", fields, rows),
		helpBlock([]string{
			"Run `fjgo api inspect <operationId>` for path/query/body details",
			"Run `fjgo api list <filter> --fields id,method,path,summary` for summaries",
		}),
	})
}

func operationBlocks(op forgejo.Operation) toonBlocks {
	info := map[string]any{
		"id":     op.ID,
		"method": op.Method,
		"path":   op.Path,
	}
	if op.Summary != "" {
		info["summary"] = op.Summary
	}
	if op.BodyType != "" {
		info["body"] = op.BodyType
	}
	if op.ReturnType != "" {
		info["returns"] = op.ReturnType
	}
	if op.Upload {
		info["upload"] = "multipart/form-data"
	}
	blocks := toonBlocks{map[string]any{"operation": info}}
	if len(op.PathParams) != 0 {
		blocks = append(blocks, map[string]any{"path_params": op.PathParams})
	}
	if len(op.QueryParams) != 0 {
		blocks = append(blocks, paramsTable("query_params", op.QueryParams))
	}
	if op.BodyType != "" {
		if model, ok := forgejo.ModelByName(strings.TrimPrefix(op.BodyType, "*")); ok && len(model.Fields) != 0 {
			blocks = append(blocks, modelFieldsTable("body_fields", model.Fields))
		}
	}
	if len(op.FormParams) != 0 {
		blocks = append(blocks, paramsTable("form_params", op.FormParams))
	}
	return blocks
}

func aliasBlocks(alias forgejo.Alias) toonBlocks {
	op, _ := forgejo.OperationByID(alias.Operation)
	info := map[string]any{
		"command":   strings.Join(alias.Command, " "),
		"operation": alias.Operation,
		"method":    op.Method,
		"path":      op.Path,
	}
	if alias.Unsafe {
		info["requires"] = "--yes"
	}
	blocks := toonBlocks{map[string]any{"alias": info}}
	if len(alias.Args) != 0 {
		blocks = append(blocks, map[string]any{"args": alias.Args})
	}
	blocks = append(blocks, operationBlocks(op)...)
	return blocks
}

func paramsTable(label string, params []forgejo.OperationParam) toonTable {
	rows := make([]map[string]any, 0, len(params))
	for _, param := range params {
		rows = append(rows, map[string]any{
			"name":        param.Name,
			"type":        param.Type,
			"required":    param.Required,
			"description": param.Description,
		})
	}
	return tableBlock(label, []string{"name", "type", "required", "description"}, rows)
}

func modelFieldsTable(label string, fields []forgejo.ModelField) toonTable {
	rows := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		rows = append(rows, map[string]any{
			"name":     field.Name,
			"type":     field.Type,
			"required": field.Required,
		})
	}
	return tableBlock(label, []string{"name", "type", "required"}, rows)
}

func writeAliasList(stdout io.Writer, aliases []forgejo.Alias, fields []string) error {
	if len(fields) == 0 {
		fields = []string{"command", "operation", "method", "path"}
	}
	rows := make([]map[string]any, 0, len(aliases))
	for _, alias := range aliases {
		op, _ := forgejo.OperationByID(alias.Operation)
		row := map[string]any{
			"command":   strings.Join(alias.Command, " "),
			"operation": alias.Operation,
			"method":    op.Method,
			"path":      op.Path,
			"unsafe":    alias.Unsafe,
		}
		rows = append(rows, selectFields(row, fields))
	}
	if len(rows) == 0 {
		return writeTOON(stdout, map[string]any{"aliases": "0 aliases found"})
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"count": len(rows)},
		tableBlock("aliases", fields, rows),
		helpBlock([]string{
			"Run `fjgo alias inspect <command...>` before calling an unfamiliar alias",
			"Run `fjgo api inspect <operationId>` to inspect the underlying operation",
		}),
	})
}

func selectFields(row map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, field := range fields {
		out[field] = row[field]
	}
	return out
}

func writeRepository(stdout io.Writer, repo *forgejo.Repository, jsonOut bool, fields ...[]string) error {
	if jsonOut {
		return writeJSON(stdout, repo)
	}
	if repo == nil {
		return writeTOON(stdout, map[string]any{"repository": "not found"})
	}
	if len(fields) != 0 && len(fields[0]) != 0 {
		return writeTOON(stdout, map[string]any{"repository": selectFields(repositoryRow(repo), fields[0])})
	}
	blocks := toonBlocks{
		repoSummaryBlock(*repo),
	}
	if len(repo.Topics) == 0 {
		blocks = append(blocks, map[string]any{"topics": "0 topics"})
	} else {
		rows := make([]map[string]any, 0, len(repo.Topics))
		for _, topic := range repo.Topics {
			rows = append(rows, map[string]any{"name": topic})
		}
		blocks = append(blocks, tableBlock("topics", []string{"name"}, rows))
	}
	return writeTOON(stdout, blocks)
}

func repositoryRow(repo *forgejo.Repository) map[string]any {
	if repo == nil {
		return map[string]any{}
	}
	return map[string]any{
		"full_name":      repo.FullName,
		"default_branch": repo.DefaultBranch,
		"private":        repo.Private,
		"archived":       repo.Archived,
		"open_issues":    repo.OpenIssues,
		"open_pulls":     repo.OpenPulls,
		"releases":       repo.Releases,
		"url":            repo.HTMLURL,
		"description":    repo.Description,
		"stars":          repo.Stars,
		"forks":          repo.Forks,
	}
}

func writeTopics(stdout io.Writer, ref repoRef, topics []string, jsonOut bool) error {
	if jsonOut {
		return writeJSON(stdout, map[string][]string{"topics": topics})
	}
	if len(topics) == 0 {
		return writeTOON(stdout, toonBlocks{
			map[string]any{"topics": fmt.Sprintf("0 topics found for %s/%s", ref.Owner, ref.Repo)},
			helpBlock([]string{fmt.Sprintf("Run `fjgo repo topics %s/%s --set topic1,topic2 --dry-run --yes` to preview replacing topics", ref.Owner, ref.Repo)}),
		})
	}
	rows := make([]map[string]any, 0, len(topics))
	for _, topic := range topics {
		rows = append(rows, map[string]any{"name": topic})
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"count": len(rows)},
		tableBlock("topics", []string{"name"}, rows),
		helpBlock([]string{fmt.Sprintf("Run `fjgo repo topics %s/%s --set topic1,topic2 --dry-run --yes` to preview replacing topics", ref.Owner, ref.Repo)}),
	})
}

func writeReleases(stdout io.Writer, ref repoRef, releases []*forgejo.Release, jsonOut bool) error {
	if jsonOut {
		return writeJSON(stdout, releases)
	}
	rows := make([]map[string]any, 0, len(releases))
	for _, release := range releases {
		if release == nil {
			continue
		}
		title := release.Title
		if title == "" {
			title = release.TagName
		}
		rows = append(rows, map[string]any{
			"id":         release.ID,
			"tag":        release.TagName,
			"title":      title,
			"draft":      release.IsDraft,
			"prerelease": release.IsPrerelease,
		})
	}
	if len(rows) == 0 {
		return writeTOON(stdout, toonBlocks{
			map[string]any{"releases": fmt.Sprintf("0 releases found for %s/%s", ref.Owner, ref.Repo)},
			suggestionHelp(suggestionContext{Domain: "release", Action: "list", Empty: true, Repo: refPtr(ref)}),
		})
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"count": len(rows)},
		tableBlock("releases", []string{"id", "tag", "title", "draft", "prerelease"}, rows),
		suggestionHelp(suggestionContext{Domain: "release", Action: "list", Empty: false, Repo: refPtr(ref)}),
	})
}

func collapseHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	absHome, _ := filepath.Abs(home)
	absPath, _ := filepath.Abs(path)
	if absPath == absHome {
		return "~"
	}
	if strings.HasPrefix(absPath, absHome+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(absPath, absHome)
	}
	return path
}

func skillStaticGuidance(commandPrefix string) string {
	return fjgoskill.StaticGuidance(commandPrefix)
}

func skillStatusBlock(dir string) map[string]any {
	status := fjgoskill.Check(dir)
	return map[string]any{
		"path":      status.Path,
		"installed": status.Installed,
		"current":   status.Current,
		"missing":   status.Missing,
		"outdated":  status.Outdated,
	}
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

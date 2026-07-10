package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

var repoListFields = []string{"name", "description", "private", "archived", "stars", "forks", "updated", "url"}
var repoListDefaultFields = []string{"name", "description", "private", "archived"}
var branchFields = []string{"name", "protected", "required_approvals", "status_checks", "can_push", "can_merge"}
var branchProtectionFields = []string{"name", "branch", "required_approvals", "status_checks", "signed_commits", "push", "admins"}
var collaboratorFields = []string{"login", "full_name", "active", "admin", "url"}

func runRepoList(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo list", []string{"--json", "--fields", "--limit", "--page", "--user", "--org", "--q", "--sort", "--private"}, []string{"--fields", "--limit", "--page", "--user", "--org", "--q", "--sort", "--private"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, repoListDefaultFields, repoListFields, "repo list")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {defaultListLimit}}
	var user, org, q string
	for _, spec := range []struct{ flag, key string }{
		{"--limit", "limit"},
		{"--page", "page"},
		{"--sort", "sort"},
		{"--private", "private"},
	} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		addQueryFlag(query, value, ok, spec.key)
	}
	args, user, _, err = takeValueFlag(args, "--user")
	if err != nil {
		return err
	}
	args, org, _, err = takeValueFlag(args, "--org")
	if err != nil {
		return err
	}
	args, q, _, err = takeValueFlag(args, "--q")
	if err != nil {
		return err
	}
	if len(args) != 0 {
		return newUsageError("usage: fjgo repo list [--user user|--org org|--q text] [flags]")
	}
	var resp forgejo.RawResponse
	switch {
	case q != "":
		query.Set("q", q)
		resp, err = rawOperationResponse(ctx, client, "repoSearch", nil, query, nil)
	case org != "":
		resp, err = rawOperationResponse(ctx, client, "orgListRepos", map[string]string{"org": org}, query, nil)
	case user != "":
		resp, err = rawOperationResponse(ctx, client, "userListRepos", map[string]string{"username": user}, query, nil)
	default:
		resp, err = rawOperationResponse(ctx, client, "userCurrentListRepos", nil, query, nil)
	}
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	var repos []*forgejo.Repository
	if q != "" {
		results, err := decodeBody[forgejo.SearchResults](resp.Body)
		if err != nil {
			return err
		}
		repos = results.Data
	} else {
		repos, err = decodeBody[[]*forgejo.Repository](resp.Body)
		if err != nil {
			return err
		}
	}
	rows := make([]map[string]any, 0, len(repos))
	for _, repo := range repos {
		if repo != nil {
			rows = append(rows, repoListRow(repo))
		}
	}
	return writeRows(stdout, "repositories", rowsSelect(rows, fields), fields, responseTotal(resp), "0 repositories found", suggestionLines(suggestionContext{
		Domain: "repo",
		Action: "list",
		Empty:  len(rows) == 0,
	}))
}

func runRepoCreate(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "repo create", []string{"--org", "--name", "--description", "--website", "--default-branch", "--gitignores", "--license", "--readme", "--object-format", "--private", "--auto-init", "--template", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--org", "--name", "--description", "--website", "--default-branch", "--gitignores", "--license", "--readme", "--object-format"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	body := map[string]any{}
	var org, name string
	var err error
	args, org, _, err = takeValueFlag(args, "--org")
	if err != nil {
		return err
	}
	args, name, _, err = takeValueFlag(args, "--name")
	if err != nil {
		return err
	}
	for _, spec := range []struct{ flag, key string }{
		{"--description", "description"},
		{"--website", "website"},
		{"--default-branch", "default_branch"},
		{"--gitignores", "gitignores"},
		{"--license", "license"},
		{"--readme", "readme"},
		{"--object-format", "object_format_name"},
	} {
		args, err = takeStringBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	for _, spec := range []struct{ flag, key string }{
		{"--private", "private"},
		{"--auto-init", "auto_init"},
		{"--template", "template"},
	} {
		args, _ = takePresenceBodyFlag(args, spec.flag, spec.key, body)
	}
	if name == "" && len(args) == 1 {
		name = args[0]
		args = nil
	}
	if len(args) != 0 || name == "" {
		return newUsageError("usage: fjgo repo create <name> [--org org] [flags] --yes")
	}
	if !yes {
		return newUsageError("repo create requires --yes")
	}
	body["name"] = name
	operation := "createCurrentUserRepo"
	pathValues := map[string]string{}
	if org != "" {
		operation = "createOrgRepo"
		pathValues["org"] = org
	}
	if dryRun {
		op, _ := forgejo.OperationByID(operation)
		return writeRequestPreview(stdout, requestPreview{Operation: op.ID, Method: op.Method, Path: previewPath(op, pathValues), Body: body, AuthPresent: clientHasAuth(client), RequiresYes: true, YesProvided: yes})
	}
	op, _ := forgejo.OperationByID(operation)
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "repository", full)
}

func runRepoEdit(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "repo edit", []string{"--name", "--description", "--website", "--default-branch", "--private", "--archived", "--has-issues", "--has-pulls", "--has-wiki", "--has-actions", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--description", "--website", "--default-branch", "--private", "--archived", "--has-issues", "--has-pulls", "--has-wiki", "--has-actions"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	body := map[string]any{}
	var err error
	for _, spec := range []struct{ flag, key string }{
		{"--name", "name"},
		{"--description", "description"},
		{"--website", "website"},
		{"--default-branch", "default_branch"},
	} {
		args, err = takeStringBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	for _, spec := range []struct{ flag, key string }{
		{"--private", "private"},
		{"--archived", "archived"},
		{"--has-issues", "has_issues"},
		{"--has-pulls", "has_pull_requests"},
		{"--has-wiki", "has_wiki"},
		{"--has-actions", "has_actions"},
	} {
		args, err = takeBoolBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo edit [owner/repo] [flags] --yes")
	}
	if len(body) == 0 {
		return newUsageError("repo edit requires at least one change")
	}
	if !yes {
		return newUsageError("repo edit requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoEdit", ref, nil, body, yes)
	}
	op, _ := forgejo.OperationByID("repoEdit")
	out, err := client.DoOperationRaw(ctx, op, repoPath(ref), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "repository", full)
}

func runRepoFork(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "repo fork", []string{"--name", "--org", "--organization", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--org", "--organization"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	body := map[string]any{}
	var name, org string
	var err error
	args, name, _, err = takeValueFlag(args, "--name")
	if err != nil {
		return err
	}
	args, org, _, err = takeValueFlag(args, "--org")
	if err != nil {
		return err
	}
	if org == "" {
		args, org, _, err = takeValueFlag(args, "--organization")
		if err != nil {
			return err
		}
	}
	if name != "" {
		body["name"] = name
	}
	if org != "" {
		body["organization"] = org
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo fork [owner/repo] [--name name] [--org org] --yes")
	}
	if !yes {
		return newUsageError("repo fork requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "createFork", ref, nil, body, yes)
	}
	op, _ := forgejo.OperationByID("createFork")
	out, err := client.DoOperationRaw(ctx, op, repoPath(ref), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "repository", full)
}

func runRepoBranches(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo repo branches <list|get|create|delete> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo branches list\n  fjgo --repo OWNER/REPO repo branches create --name feature --from main --dry-run --yes\n  fjgo --repo OWNER/REPO repo branches delete feature --dry-run --yes")
	}
	switch args[0] {
	case "list":
		return runRepoBranchesList(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "get":
		return runRepoBranchGet(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "create":
		return runRepoBranchCreate(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "delete", "remove":
		return runRepoBranchDelete(ctx, client, cfg, args[1:], stdout, jsonOut)
	default:
		return unknownSubcommandError("repo branches", args[0], []string{"list", "get", "create", "delete", "remove"})
	}
}

func runRepoBranchesList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branches list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, []string{"name", "protected", "required_approvals"}, branchFields, "repo branches list")
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
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo branches list [owner/repo] [flags]")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListBranches", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	branches, err := decodeBody[[]*forgejo.Branch](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(branches))
	for _, branch := range branches {
		if branch != nil {
			rows = append(rows, branchRow(branch))
		}
	}
	return writeRows(stdout, "branches", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 branches found for %s/%s", ref.Owner, ref.Repo), nil)
}

func runRepoBranchGet(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branches get", []string{"--json"}, nil); err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo branches get [owner/repo] <branch>")
	}
	out, err := client.RepoGetBranch(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	return writeTOON(stdout, map[string]any{"branch": branchRow(out)})
}

func runRepoBranchCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branches create", []string{"--name", "--from", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--name", "--from"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	var name, from string
	var err error
	args, name, _, err = takeValueFlag(args, "--name")
	if err != nil {
		return err
	}
	args, from, _, err = takeValueFlag(args, "--from")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil {
		return newUsageError("usage: fjgo repo branches create [owner/repo] --name <branch> [--from ref] --yes")
	}
	if name == "" && len(rest) > 0 {
		name = rest[0]
		rest = rest[1:]
	}
	if len(rest) != 0 || name == "" {
		return newUsageError("usage: fjgo repo branches create [owner/repo] --name <branch> [--from ref] --yes")
	}
	if !yes {
		return newUsageError("repo branches create requires --yes")
	}
	body := map[string]any{"new_branch_name": name}
	if from != "" {
		body["old_ref_name"] = from
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoCreateBranch", ref, nil, body, yes)
	}
	op, _ := forgejo.OperationByID("repoCreateBranch")
	out, err := client.DoOperationRaw(ctx, op, repoPath(ref), forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "branch", false)
}

func runRepoBranchDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branches delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo branches delete [owner/repo] <branch> --yes")
	}
	if !yes {
		return newUsageError("repo branches delete requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoDeleteBranch", ref, map[string]string{"branch": rest[0]}, nil, yes)
	}
	if err := client.RepoDeleteBranch(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"branch": rest[0], "deleted": true})
	}
	return writeTOON(stdout, map[string]any{"branch": rest[0] + " deleted"})
}

func runRepoCollaborators(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo repo collaborators <list|check|permission|add|remove> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo collaborators list\n  fjgo --repo OWNER/REPO repo collaborators add alice --permission write --dry-run --yes\n  fjgo --repo OWNER/REPO repo collaborators permission alice")
	}
	switch args[0] {
	case "list":
		return runRepoCollaboratorsList(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "check", "get":
		return runRepoCollaboratorCheck(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "permission":
		return runRepoCollaboratorPermission(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "add":
		return runRepoCollaboratorAdd(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "remove", "delete":
		return runRepoCollaboratorRemove(ctx, client, cfg, args[1:], stdout, jsonOut)
	default:
		return unknownSubcommandError("repo collaborators", args[0], []string{"list", "check", "get", "permission", "add", "remove", "delete"})
	}
}

func runRepoCollaboratorsList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo collaborators list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, []string{"login", "full_name", "active"}, collaboratorFields, "repo collaborators list")
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
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo collaborators list [owner/repo] [flags]")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListCollaborators", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	users, err := decodeBody[[]*forgejo.User](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(users))
	for _, user := range users {
		if user != nil {
			rows = append(rows, userRow(user))
		}
	}
	return writeRows(stdout, "collaborators", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 collaborators found for %s/%s", ref.Owner, ref.Repo), nil)
}

func runRepoCollaboratorCheck(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo collaborators check", []string{"--json"}, nil); err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo collaborators check [owner/repo] <user>")
	}
	err = client.RepoCheckCollaborator(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"collaborator": rest[0], "present": true})
	}
	return writeTOON(stdout, map[string]any{"collaborator": rest[0] + " present"})
}

func runRepoCollaboratorPermission(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo collaborators permission", []string{"--json"}, nil); err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo collaborators permission [owner/repo] <user>")
	}
	out, err := client.RepoGetRepoPermissions(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	row := map[string]any{"permission": "", "role": "", "login": rest[0]}
	if out != nil {
		row["permission"] = out.Permission
		row["role"] = out.RoleName
		row["login"] = userName(out.User)
	}
	return writeTOON(stdout, map[string]any{"collaborator": row})
}

func runRepoCollaboratorAdd(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo collaborators add", []string{"--permission", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--permission"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	permission := "write"
	var ok bool
	var err error
	args, permission, ok, err = takeValueFlag(args, "--permission")
	if err != nil {
		return err
	}
	if !ok {
		permission = "write"
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo collaborators add [owner/repo] <user> [--permission read|write|admin] --yes")
	}
	if !yes {
		return newUsageError("repo collaborators add requires --yes")
	}
	body := map[string]any{"permission": permission}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoAddCollaborator", ref, map[string]string{"collaborator": rest[0]}, body, yes)
	}
	if err := client.RepoAddCollaborator(ctx, ref.Owner, ref.Repo, rest[0], &forgejo.AddCollaboratorOption{Permission: permission}, forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"collaborator": rest[0], "permission": permission, "added": true})
	}
	return writeTOON(stdout, map[string]any{"collaborator": fmt.Sprintf("%s added with %s permission", rest[0], permission)})
}

func runRepoCollaboratorRemove(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo collaborators remove", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo collaborators remove [owner/repo] <user> --yes")
	}
	if !yes {
		return newUsageError("repo collaborators remove requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoDeleteCollaborator", ref, map[string]string{"collaborator": rest[0]}, nil, yes)
	}
	if err := client.RepoDeleteCollaborator(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"collaborator": rest[0], "removed": true})
	}
	return writeTOON(stdout, map[string]any{"collaborator": rest[0] + " removed"})
}

func runRepoBranchProtection(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo repo branch-protection <list|get|create|edit|delete> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo branch-protection list\n  fjgo --repo OWNER/REPO repo branch-protection create --name main --required-approvals 1 --dry-run --yes\n  fjgo --repo OWNER/REPO repo branch-protection delete main --dry-run --yes")
	}
	switch args[0] {
	case "list":
		return runRepoBranchProtectionList(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "get":
		return runRepoBranchProtectionGet(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "create":
		return runRepoBranchProtectionMutate(ctx, client, cfg, args[1:], stdout, jsonOut, full, false)
	case "edit":
		return runRepoBranchProtectionMutate(ctx, client, cfg, args[1:], stdout, jsonOut, full, true)
	case "delete", "remove":
		return runRepoBranchProtectionDelete(ctx, client, cfg, args[1:], stdout, jsonOut)
	default:
		return unknownSubcommandError("repo branch-protection", args[0], []string{"list", "get", "create", "edit", "delete", "remove"})
	}
}

func runRepoBranchProtectionList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branch-protection list", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, []string{"name", "required_approvals", "status_checks"}, branchProtectionFields, "repo branch-protection list")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo branch-protection list [owner/repo]")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListBranchProtection", repoPath(ref), nil, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, resp.Body, "application/json", true, true)
	}
	protections, err := decodeBody[[]*forgejo.BranchProtection](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(protections))
	for _, protection := range protections {
		if protection != nil {
			rows = append(rows, branchProtectionRow(protection))
		}
	}
	return writeRows(stdout, "branch_protections", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 branch protections found for %s/%s", ref.Owner, ref.Repo), nil)
}

func runRepoBranchProtectionGet(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branch-protection get", []string{"--json"}, nil); err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo branch-protection get [owner/repo] <name>")
	}
	out, err := client.RepoGetBranchProtection(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	return writeTOON(stdout, map[string]any{"branch_protection": branchProtectionRow(out)})
}

func runRepoBranchProtectionMutate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full, edit bool) error {
	command := "repo branch-protection create"
	operation := "repoCreateBranchProtection"
	if edit {
		command = "repo branch-protection edit"
		operation = "repoEditBranchProtection"
	}
	if err := rejectUnknownFlags(args, command, []string{"--name", "--required-approvals", "--status-check", "--enable-status-check", "--require-signed-commits", "--enable-push", "--apply-to-admins", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--required-approvals", "--status-check", "--enable-status-check", "--require-signed-commits", "--enable-push", "--apply-to-admins"}); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	body := map[string]any{}
	var name string
	var err error
	args, name, _, err = takeValueFlag(args, "--name")
	if err != nil {
		return err
	}
	args, checks, err := takeAllValueFlags(args, "--status-check")
	if err != nil {
		return err
	}
	if len(checks) != 0 {
		body["status_check_contexts"] = flattenCSV(checks)
	}
	args, err = takeIntBodyFlag(args, "--required-approvals", "required_approvals", body)
	if err != nil {
		return err
	}
	for _, spec := range []struct{ flag, key string }{
		{"--enable-status-check", "enable_status_check"},
		{"--require-signed-commits", "require_signed_commits"},
		{"--enable-push", "enable_push"},
		{"--apply-to-admins", "apply_to_admins"},
	} {
		args, err = takeBoolBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <name> [flags] --yes")
	}
	if name == "" && len(rest) > 0 {
		name = rest[0]
		rest = rest[1:]
	}
	if len(rest) != 0 || name == "" {
		return newUsageError("usage: fjgo " + command + " [owner/repo] <name> [flags] --yes")
	}
	if !yes {
		return newUsageError(command + " requires --yes")
	}
	extra := map[string]string{}
	if edit {
		extra["name"] = name
	} else {
		body["rule_name"] = name
		body["branch_name"] = name
	}
	if len(body) == 0 {
		return newUsageError(command + " requires at least one setting")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, extra, body, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	pathValues := repoPath(ref)
	for key, value := range extra {
		pathValues[key] = value
	}
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "branch_protection", full)
}

func runRepoBranchProtectionDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "repo branch-protection delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo repo branch-protection delete [owner/repo] <name> --yes")
	}
	if !yes {
		return newUsageError("repo branch-protection delete requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoDeleteBranchProtection", ref, map[string]string{"name": rest[0]}, nil, yes)
	}
	if err := client.RepoDeleteBranchProtection(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"branch_protection": rest[0], "deleted": true})
	}
	return writeTOON(stdout, map[string]any{"branch_protection": rest[0] + " deleted"})
}

func repoListRow(repo *forgejo.Repository) map[string]any {
	return map[string]any{
		"name":        repo.FullName,
		"description": repo.Description,
		"private":     repo.Private,
		"archived":    repo.Archived,
		"stars":       repo.Stars,
		"forks":       repo.Forks,
		"updated":     repo.Updated,
		"url":         repo.HTMLURL,
	}
}

func branchRow(branch *forgejo.Branch) map[string]any {
	if branch == nil {
		return map[string]any{}
	}
	return map[string]any{
		"name":               branch.Name,
		"protected":          branch.Protected,
		"required_approvals": branch.RequiredApprovals,
		"status_checks":      strings.Join(branch.StatusCheckContexts, "|"),
		"can_push":           branch.UserCanPush,
		"can_merge":          branch.UserCanMerge,
	}
}

func branchProtectionRow(protection *forgejo.BranchProtection) map[string]any {
	if protection == nil {
		return map[string]any{}
	}
	name := protection.RuleName
	if name == "" {
		name = protection.BranchName
	}
	return map[string]any{
		"name":               name,
		"branch":             protection.BranchName,
		"required_approvals": protection.RequiredApprovals,
		"status_checks":      strings.Join(protection.StatusCheckContexts, "|"),
		"signed_commits":     protection.RequireSignedCommits,
		"push":               protection.EnablePush,
		"admins":             protection.ApplyToAdmins,
	}
}

func userRow(user *forgejo.User) map[string]any {
	if user == nil {
		return map[string]any{}
	}
	return map[string]any{
		"login":     user.UserName,
		"full_name": user.FullName,
		"active":    user.IsActive,
		"admin":     user.IsAdmin,
		"url":       user.HTMLURL,
	}
}

func takeStringBodyFlag(args []string, flag, key string, body map[string]any) ([]string, error) {
	next, value, ok, err := takeValueFlag(args, flag)
	if err != nil {
		return nil, err
	}
	if ok {
		body[key] = value
	}
	return next, nil
}

func takeBoolBodyFlag(args []string, flag, key string, body map[string]any) ([]string, error) {
	next, value, ok, err := takeValueFlag(args, flag)
	if err != nil {
		return nil, err
	}
	if ok {
		parsed, err := parseBoolValue(flag, value)
		if err != nil {
			return nil, err
		}
		body[key] = parsed
	}
	return next, nil
}

func takePresenceBodyFlag(args []string, flag, key string, body map[string]any) ([]string, bool) {
	next, ok := boolFlag(args, flag)
	if ok {
		body[key] = true
	}
	return next, ok
}

func takeIntBodyFlag(args []string, flag, key string, body map[string]any) ([]string, error) {
	next, value, ok, err := takeValueFlag(args, flag)
	if err != nil {
		return nil, err
	}
	if ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, newUsageError(flag + " expects an integer")
		}
		body[key] = parsed
	}
	return next, nil
}

func flattenCSV(values []string) []string {
	var out []string
	for _, value := range values {
		out = append(out, splitCSV(value)...)
	}
	return out
}

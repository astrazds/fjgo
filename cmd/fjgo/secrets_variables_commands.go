package main

import (
	"context"
	"fmt"
	"io"
	"net/url"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

func secretHelp() string {
	return `usage: fjgo secret <list|set|delete> [owner/repo] [flags]

notes:
  secret values are stdin-only; do not pass secrets as argv

examples:
  fjgo -R origin secret list
  echo -n "$TOKEN" | fjgo -R origin secret set DEPLOY_TOKEN --dry-run --yes
	  fjgo -R origin secret delete DEPLOY_TOKEN --yes`
}

func secretCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":   {Usage: "fjgo secret list [owner/repo] [flags]", Flags: []string{"--limit <n> (default 100), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin secret list", "fjgo secret list OWNER/REPO --fields name,created"}},
		"set":    {Usage: "fjgo secret set [owner/repo] <name> --yes", Flags: []string{"secret value is read from stdin", "--dry-run, --print-request, --json"}, Examples: []string{"printf secret | fjgo -R origin secret set DEPLOY_TOKEN --dry-run --yes", "printf secret | fjgo secret set OWNER/REPO DEPLOY_TOKEN --yes"}},
		"delete": {Usage: "fjgo secret delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin secret delete DEPLOY_TOKEN --dry-run --yes", "fjgo secret delete OWNER/REPO DEPLOY_TOKEN --yes"}},
	}
}

func runSecret(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, secretHelp(), secretCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	switch args[0] {
	case "list":
		return runSecretList(ctx, client, cfg, args[1:], stdout)
	case "set":
		return runSecretSet(ctx, client, cfg, args[1:], stdout)
	case "delete":
		return runSecretDelete(ctx, client, cfg, args[1:], stdout)
	default:
		return unknownSubcommandError("secret", args[0], []string{"list", "set", "delete"})
	}
}

func runSecretList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "secret list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"name", "created"}
	available := []string{"name", "created"}
	fields, err := validatedFields(fieldsArg, defaults, available, "secret list")
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
		return newUsageError("usage: fjgo secret list [owner/repo]")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListActionsSecrets", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	secrets, err := decodeBody[[]*forgejo.Secret](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, secrets)
	}
	rows := make([]map[string]any, 0, len(secrets))
	for _, secret := range secrets {
		if secret == nil {
			continue
		}
		rows = append(rows, map[string]any{"name": secret.Name, "created": secret.Created})
	}
	return writeRows(stdout, "secrets", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 secrets found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "secret",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runSecretSet(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "secret set", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo secret set [owner/repo] <name> --yes", "Pipe the secret value on stdin")
	}
	if !yes {
		return newUsageError("secret set requires --yes")
	}
	value, err := readPipedValue("secret", "", false)
	if err != nil {
		return err
	}
	body := &forgejo.CreateOrUpdateSecretOption{Data: value}
	if dryRun {
		return writeMutationPreview(stdout, client, "updateRepoSecret", ref, map[string]string{"secretname": rest[0]}, map[string]any{"data": "redacted"}, yes)
	}
	if err := client.UpdateRepoSecret(ctx, ref.Owner, ref.Repo, rest[0], body, forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"secret": rest[0], "updated": true})
	}
	return writeTOON(stdout, map[string]any{"secret": fmt.Sprintf("%s updated", rest[0])})
}

func runSecretDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "secret delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo secret delete [owner/repo] <name> --yes")
	}
	if !yes {
		return newUsageError("secret delete requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "deleteRepoSecret", ref, map[string]string{"secretname": rest[0]}, nil, yes)
	}
	if err := client.DeleteRepoSecret(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"secret": rest[0], "deleted": true})
	}
	return writeTOON(stdout, map[string]any{"secret": fmt.Sprintf("%s deleted", rest[0])})
}

func variableHelp() string {
	return `usage: fjgo variable <list|get|set|delete> [owner/repo] [flags]

examples:
  fjgo -R origin variable list
  fjgo -R origin variable get BUILD_MODE
  fjgo -R origin variable set BUILD_MODE --body release --dry-run --yes
  echo -n release | fjgo -R origin variable set BUILD_MODE --yes
	  fjgo -R origin variable delete BUILD_MODE --yes`
}

func variableCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":   {Usage: "fjgo variable list [owner/repo] [flags]", Flags: []string{"--limit <n> (default 100), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin variable list", "fjgo variable list OWNER/REPO --fields name,value"}},
		"get":    {Usage: "fjgo variable get [owner/repo] <name> [--json]", Examples: []string{"fjgo -R origin variable get BUILD_MODE", "fjgo variable get OWNER/REPO BUILD_MODE --json"}},
		"set":    {Usage: "fjgo variable set [owner/repo] <name> [--body <value>] --yes", Flags: []string{"--body <value>; otherwise value is read from stdin", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin variable set BUILD_MODE --body release --dry-run --yes", "printf release | fjgo variable set OWNER/REPO BUILD_MODE --yes"}},
		"delete": {Usage: "fjgo variable delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin variable delete BUILD_MODE --dry-run --yes", "fjgo variable delete OWNER/REPO BUILD_MODE --yes"}},
	}
}

func runVariable(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, variableHelp(), variableCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	switch args[0] {
	case "list":
		return runVariableList(ctx, client, cfg, args[1:], stdout)
	case "get":
		return runVariableGet(ctx, client, cfg, args[1:], stdout)
	case "set":
		return runVariableSet(ctx, client, cfg, args[1:], stdout)
	case "delete":
		return runVariableDelete(ctx, client, cfg, args[1:], stdout)
	default:
		return unknownSubcommandError("variable", args[0], []string{"list", "get", "set", "delete"})
	}
}

func runVariableList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "variable list", []string{"--json", "--fields", "--limit", "--page"}, []string{"--fields", "--limit", "--page"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	defaults := []string{"name", "value"}
	available := []string{"name", "value"}
	fields, err := validatedFields(fieldsArg, defaults, available, "variable list")
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
		return newUsageError("usage: fjgo variable list [owner/repo]")
	}
	resp, err := rawOperationResponse(ctx, client, "getRepoVariablesList", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	variables, err := decodeBody[[]*forgejo.ActionVariable](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, variables)
	}
	rows := make([]map[string]any, 0, len(variables))
	for _, variable := range variables {
		if variable != nil {
			rows = append(rows, variableRow(variable))
		}
	}
	return writeRows(stdout, "variables", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 variables found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "variable",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runVariableGet(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "variable get", []string{"--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo variable get [owner/repo] <name>")
	}
	variable, err := client.GetRepoVariable(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, variable)
	}
	return writeTOON(stdout, map[string]any{"variable": variableRow(variable)})
}

func runVariableSet(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "variable set", []string{"--body", "-body", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--body", "-body"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	var flagValue string
	var bodySet bool
	var err error
	args, flagValue, bodySet, err = takeValueFlag(args, "--body")
	if err != nil {
		return err
	}
	if !bodySet {
		args, flagValue, bodySet, err = takeValueFlag(args, "-body")
		if err != nil {
			return err
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo variable set [owner/repo] <name> --body <value> --yes")
	}
	if !yes {
		return newUsageError("variable set requires --yes")
	}
	value, err := readPipedValue("variable", flagValue, bodySet)
	if err != nil {
		return err
	}
	body := &forgejo.UpdateVariableOption{Value: value}
	if dryRun {
		return writeMutationPreview(stdout, client, "updateRepoVariable", ref, map[string]string{"variablename": rest[0]}, body, yes)
	}
	if err := client.UpdateRepoVariable(ctx, ref.Owner, ref.Repo, rest[0], body, forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"variable": rest[0], "updated": true})
	}
	return writeTOON(stdout, map[string]any{"variable": fmt.Sprintf("%s updated", rest[0])})
}

func runVariableDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if err := rejectUnknownFlags(args, "variable delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo variable delete [owner/repo] <name> --yes")
	}
	if !yes {
		return newUsageError("variable delete requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "deleteRepoVariable", ref, map[string]string{"variablename": rest[0]}, nil, yes)
	}
	if err := client.DeleteRepoVariable(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{}); err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"variable": rest[0], "deleted": true})
	}
	return writeTOON(stdout, map[string]any{"variable": fmt.Sprintf("%s deleted", rest[0])})
}

func variableRow(variable *forgejo.ActionVariable) map[string]any {
	if variable == nil {
		return map[string]any{}
	}
	return map[string]any{"name": variable.Name, "value": variable.Data}
}

package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var apiListFlags = newFlagSet([]string{"--json", "--fields"}, []string{"--fields"})
var apiInspectFlags = newFlagSet([]string{"--json"}, nil)
var apiCallFlags = newFlagSet([]string{"-body", "-body-raw", "--content-type", "--accept", "--output", "--raw", "--include-response", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"-body", "-body-raw", "--content-type", "--accept", "--output"})
var apiUploadFlags = newFlagSet([]string{"--include-response", "--yes", "--dry-run", "--print-request", "--json", "--full"}, nil)
var apiRawFlags = apiCallFlags

func apiGroup() commandGroup {
	return commandGroup{
		name:     "api",
		overview: apiHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: apiListHelp(), flags: apiListFlags, run: runAPIListLeaf},
			{names: []string{"inspect"}, help: apiInspectHelp(), flags: apiInspectFlags, run: runAPIInspectLeaf},
			{names: []string{"call"}, help: apiCallHelp(), flags: apiCallFlags, mutation: mutationRequired, run: runAPICallLeaf},
			{names: []string{"upload"}, help: apiUploadHelp(), flags: apiUploadFlags, mutation: mutationRequired, run: runAPIUploadLeaf},
			{names: []string{"raw", "request"}, help: apiRawHelp(), flags: apiRawFlags, mutation: mutationRequired, run: runAPIRawLeaf},
		},
	}
}

func apiCommandHelps() map[string]commandHelpSpec { return apiGroup().helpSpecs() }

func apiListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo api list [filter] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo api list repo", "fjgo api list release --fields id,method,path"}}
}
func apiInspectHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo api inspect <operationId> [--json]", Examples: []string{"fjgo api inspect createCurrentUserRepo", "fjgo api inspect repoSearch --json"}}
}
func apiCallHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo api call <operationId> [name=value ...] [flags]", Flags: []string{"-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>, --accept <type>", "--output <path>, --raw", "--include-response", "--yes, --dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api call repoSearch q=forgejo", "fjgo api call getVersion --include-response", "fjgo api call repoGetArchive owner=OWNER repo=REPO archive=main.zip --output repo.zip"}}
}
func apiUploadHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo api upload <operationId> [name=value ...] attachment=@file [flags] --yes", Flags: []string{"--include-response", "--dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api upload repoCreateReleaseAttachment owner=OWNER repo=REPO id=1 attachment=@dist.tgz --dry-run --yes", "fjgo api upload issueCreateIssueAttachment owner=OWNER repo=REPO index=1 attachment=@file.txt --include-response --yes"}}
}
func apiRawHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo api raw <METHOD> <path> [name=value ...] [flags]", Flags: []string{"-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>, --accept <type>", "--output <path>, --raw", "--include-response", "--yes, --dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api raw GET /version --include-response", "fjgo api raw POST /markdown/raw -body-raw @README.md --content-type text/plain --yes", "fjgo api raw GET /repos/OWNER/REPO/archive/main.zip --output repo.zip"}}
}

func runAPIListLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "api list", apiListFlags); err != nil {
		return err
	}
	args, fields, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err = validatedFields(fields, []string{"id", "method", "path"}, []string{"id", "method", "path", "summary", "description", "returns", "body", "upload", "tags", "deprecated"}, "api list")
	if err != nil {
		return err
	}
	filter := ""
	if len(args) > 0 {
		filter = strings.ToLower(args[0])
	}
	var out []forgejo.Operation
	for _, op := range forgejo.Operations() {
		line := fmt.Sprintf("%-45s %-6s %s", op.ID, op.Method, op.Path)
		if filter == "" || strings.Contains(strings.ToLower(line+" "+op.Summary+" "+op.Description+" "+strings.Join(op.Tags, " ")), filter) {
			out = append(out, op)
		}
	}
	if jsonOut {
		return writeJSON(stdout, out)
	}
	return writeOperationsList(stdout, out, fields)
}

func runAPIInspectLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "api inspect", apiInspectFlags); err != nil {
		return err
	}
	if len(args) != 1 {
		return newUsageError("usage: fjgo api inspect <operationId>", "Run `fjgo api list <filter>` to find operation IDs")
	}
	op, ok := forgejo.OperationByID(args[0])
	if !ok {
		return fmt.Errorf("unknown operation %q", args[0])
	}
	if jsonOut {
		return writeJSON(stdout, operationView(op))
	}
	return writeTOON(stdout, operationBlocks(op))
}

func runAPICallLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut, full := peelJSONFull(args)
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return callOperation(ctx, client, cfg, args, stdout, stderr, jsonOut, full)
}

func runAPIUploadLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut, full := peelJSONFull(args)
	return uploadOperation(ctx, client, cfg, args, stdout, jsonOut, full)
}

func runAPIRawLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut, full := peelJSONFull(args)
	return rawRequest(ctx, client, args, stdout, jsonOut, full)
}

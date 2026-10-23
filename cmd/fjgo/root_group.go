package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var noFlags = newFlagSet(nil, nil)
var getFlags = newFlagSet([]string{"--json", "--full"}, nil)
var doctorFlags = jsonOnlyFlags
var modelFlags = jsonOnlyFlags
var authStatusFlags = jsonOnlyFlags
var updateFlags = newFlagSet([]string{"--check", "--yes", "--dry-run", "--print-request", "--json", "--base-url", "--repo"}, []string{"--base-url", "--repo"})
var generatedAliasFlags = newFlagSet([]string{"--json", "--full", "-body", "-body-raw", "--content-type", "--accept", "--output", "--raw", "--include-response", "--yes", "--dry-run", "--print-request"}, []string{"-body", "-body-raw", "--content-type", "--accept", "--output"})

func rootGroup() commandGroup {
	return commandGroup{
		name: "fjgo",
		leaves: []commandLeaf{
			{names: []string{"setup"}, flags: noFlags, run: runSetupLeaf},
			{names: []string{"hook"}, flags: noFlags, run: runHookLeaf},
			{names: []string{"doctor", "context"}, flags: doctorFlags, run: runLeaf(runDoctor)},
			{names: []string{"version"}, flags: noFlags, run: runVersionLeaf},
			{names: []string{"me"}, flags: noFlags, run: runMeLeaf},
			{names: []string{"auth", "whoami"}, flags: authStatusFlags, run: runAuthLeaf},
			{names: []string{"get"}, flags: getFlags, run: runGetLeaf},
			{names: []string{"api"}, flags: noFlags, run: runAPILeaf},
			{names: []string{"alias"}, flags: noFlags, run: runAliasLeaf},
			{names: []string{"model"}, flags: modelFlags, run: runModelLeaf},
			{names: []string{"repo"}, flags: noFlags, run: runLeaf(runRepo)},
			{names: []string{"issue"}, flags: noFlags, run: runLeaf(runIssue)},
			{names: []string{"pr", "pull"}, flags: noFlags, run: runLeaf(runPR)},
			{names: []string{"run"}, flags: noFlags, run: runLeaf(runActions)},
			{names: []string{"workflow"}, flags: noFlags, run: runLeaf(runWorkflow)},
			{names: []string{"search"}, flags: noFlags, run: runLeaf(runSearch)},
			{names: []string{"label"}, flags: noFlags, run: runLeaf(runLabel)},
			{names: []string{"secret"}, flags: noFlags, run: runLeaf(runSecret)},
			{names: []string{"variable"}, flags: noFlags, run: runLeaf(runVariable)},
			{names: []string{"release"}, flags: noFlags, run: runLeaf(runRelease)},
			{names: []string{"update"}, flags: updateFlags, run: runUpdateLeaf},
			{names: []string{"skill"}, flags: noFlags, run: runSkillLeaf},
			{names: []string{"install"}, flags: installFlags, run: runInstallLeaf},
		},
	}
}

func rootCommands() []string { return rootGroup().names() }

func modelGroup() commandGroup {
	return commandGroup{
		name: "model",
		overview: func() string {
			return "usage: fjgo model inspect <Model> [--json]\nexamples:\n  fjgo model inspect CreateIssueOption\n  fjgo model --json inspect CreateIssueOption"
		},
		leaves: []commandLeaf{{
			names: []string{"inspect"},
			flags: modelFlags,
			run:   runModelInspectLeaf,
		}},
	}
}

func runModelInspectLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "model inspect", modelFlags); err != nil {
		return err
	}
	if len(args) != 1 {
		return newUsageError("usage: fjgo model inspect <Model>", "Run `fjgo api inspect <operationId>` to find model names")
	}
	model, ok := forgejo.ModelByName(strings.TrimPrefix(args[0], "*"))
	if !ok {
		return fmt.Errorf("unknown model %q", args[0])
	}
	if jsonOut {
		return writeJSON(stdout, model)
	}
	info := map[string]any{"model": model.Name, "additional_properties": model.AdditionalProperties}
	if model.Title != "" {
		info["title"] = model.Title
	}
	if model.Description != "" {
		info["description"] = model.Description
	}
	return writeTOON(stdout, toonBlocks{
		info,
		modelFieldsTable("fields", model.Fields),
	})
}

func runSetupLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runSetup(args, stdout)
}

func runHookLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runHook(args, stdout)
}

func runAuthLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runAuth(ctx, client, cfg, args, stdout)
}

func runAliasLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runAliasInfo(args, stdout)
}

func runModelLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runModel(args, stdout)
}

func runAPILeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runAPI(ctx, client, cfg, args, stdout, stderr)
}

func runSkillLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runSkill(args, stdout, stderr)
}

func runInstallLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runInstall(args, stdout, stderr)
}

func runUpdateLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runUpdate(ctx, cfg.HTTPClient, args, stdout)
}

func runVersionLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo version\nShow the Forgejo server version.\n\nexamples:\n  fjgo version\n  fjgo --host forgejo.example.com version")
	}
	if err := rejectUnknownFlags(args, "version", noFlags); err != nil {
		return err
	}
	if len(args) != 0 {
		return newUsageError("usage: fjgo version")
	}
	body, err := callOperationRaw(ctx, client, "getVersion", nil, nil)
	if err != nil {
		return err
	}
	return writeJSONAsTOON(stdout, body, "version", false)
}

func runMeLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo me\nShow the authenticated Forgejo user summary.\n\nexamples:\n  fjgo me\n  fjgo --host forgejo.example.com me")
	}
	if err := rejectUnknownFlags(args, "me", noFlags); err != nil {
		return err
	}
	if len(args) != 0 {
		return newUsageError("usage: fjgo me")
	}
	user, err := client.Me(ctx)
	if err != nil {
		return err
	}
	return writeTOON(stdout, userSummary(user))
}

func runGetLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo get <path> [--json] [--full]\nexamples:\n  fjgo get /version\n  fjgo get /repos/OWNER/REPO --json")
	}
	if err := rejectUnknownFlags(args, "get", getFlags); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	if len(args) != 1 {
		return newUsageError("usage: fjgo get <path>", "Run `fjgo get /version`")
	}
	body, err := client.GetRaw(ctx, args[0])
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return writeAPIOK(stdout, jsonOut)
	}
	return writeAPIBody(stdout, body, "", jsonOut, full)
}

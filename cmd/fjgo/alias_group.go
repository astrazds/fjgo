package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var aliasListFlags = newFlagSet([]string{"--json", "--fields"}, []string{"--fields"})
var aliasJSONFlags = newFlagSet([]string{"--json"}, nil)

func aliasGroup() commandGroup {
	return commandGroup{
		name:     "alias",
		overview: aliasHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: aliasListHelp(), flags: aliasListFlags, run: runAliasListLeaf},
			{names: []string{"inspect"}, help: aliasInspectHelp(), flags: aliasJSONFlags, run: runAliasInspectLeaf},
			{names: []string{"collisions"}, help: aliasCollisionsHelp(), flags: aliasJSONFlags, run: runAliasCollisionsLeaf},
			{names: []string{"omissions"}, help: aliasOmissionsHelp(), flags: aliasJSONFlags, run: runAliasOmissionsLeaf},
		},
	}
}

func aliasCommandHelps() map[string]commandHelpSpec { return aliasGroup().helpSpecs() }

func aliasListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo alias list [filter] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo alias list repo", "fjgo alias list pulls --fields command,operation,path"}}
}
func aliasInspectHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo alias inspect <command...> [--json]", Examples: []string{"fjgo alias inspect repo pulls get", "fjgo alias inspect repo issues list --json"}}
}
func aliasCollisionsHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo alias collisions [filter] [--json]", Examples: []string{"fjgo alias collisions pulls", "fjgo alias collisions --json"}}
}
func aliasOmissionsHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo alias omissions [filter] [--json]", Examples: []string{"fjgo alias omissions release", "fjgo alias omissions --json"}}
}

func runAliasInfo(args []string, stdout io.Writer) error {
	moved := deferBoolFlags(args, "--json")
	return dispatchGroup(aliasGroup(), context.Background(), nil, runConfig{}, moved, stdout)
}

func runAliasListLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "alias list", aliasListFlags); err != nil {
		return err
	}
	args, fields, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err = validatedFields(fields, []string{"command", "operation", "method", "path"}, []string{"command", "operation", "method", "path", "unsafe"}, "alias list")
	if err != nil {
		return err
	}
	if len(args) > 1 {
		return newUsageError("usage: fjgo alias list [filter] [--fields <a,b,c>] [--json]")
	}
	filter := ""
	if len(args) == 1 {
		filter = strings.ToLower(args[0])
	}
	var aliases []forgejo.Alias
	for _, alias := range forgejo.Aliases() {
		op, _ := forgejo.OperationByID(alias.Operation)
		line := strings.Join(alias.Command, " ") + " " + alias.Operation + " " + op.Path
		if filter == "" || strings.Contains(strings.ToLower(line), filter) {
			aliases = append(aliases, alias)
		}
	}
	if jsonOut {
		return writeJSON(stdout, aliases)
	}
	return writeAliasList(stdout, aliases, fields)
}

func runAliasCollisionsLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "alias collisions", aliasJSONFlags); err != nil {
		return err
	}
	if len(args) > 1 {
		return newUsageError("usage: fjgo alias collisions [filter] [--json]")
	}
	filter := ""
	if len(args) == 1 {
		filter = strings.ToLower(args[0])
	}
	collisions := make([]forgejo.AliasCollision, 0, len(forgejo.AliasCollisions()))
	for _, collision := range forgejo.AliasCollisions() {
		text := strings.Join(collision.Command, " ") + " " + collision.Kept + " " + collision.Skipped + " " + collision.Reason
		if filter == "" || strings.Contains(strings.ToLower(text), filter) {
			collisions = append(collisions, collision)
		}
	}
	if jsonOut {
		return writeJSON(stdout, collisions)
	}
	rows := make([]map[string]any, 0, len(collisions))
	for _, c := range collisions {
		rows = append(rows, map[string]any{
			"command": strings.Join(c.Command, " "),
			"kept":    c.Kept,
			"skipped": c.Skipped,
			"reason":  c.Reason,
		})
	}
	if len(rows) == 0 {
		return writeTOON(stdout, map[string]any{"collisions": fmt.Sprintf("0 alias collisions found for %q", filter)})
	}
	return writeTOON(stdout, tableBlock("collisions", []string{"command", "kept", "skipped", "reason"}, rows))
}

func runAliasOmissionsLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "alias omissions", aliasJSONFlags); err != nil {
		return err
	}
	if len(args) > 1 {
		return newUsageError("usage: fjgo alias omissions [filter] [--json]")
	}
	filter := ""
	if len(args) == 1 {
		filter = strings.ToLower(args[0])
	}
	omissions := make([]forgejo.AliasOmission, 0, len(forgejo.AliasOmissions()))
	for _, omission := range forgejo.AliasOmissions() {
		text := omission.Operation + " " + omission.Reason + " " + omission.Use
		if filter == "" || strings.Contains(strings.ToLower(text), filter) {
			omissions = append(omissions, omission)
		}
	}
	if jsonOut {
		return writeJSON(stdout, omissions)
	}
	rows := make([]map[string]any, 0, len(omissions))
	for _, omission := range omissions {
		rows = append(rows, map[string]any{
			"operation": omission.Operation,
			"reason":    omission.Reason,
			"use":       omission.Use,
		})
	}
	if len(rows) == 0 {
		return writeTOON(stdout, map[string]any{"omissions": fmt.Sprintf("0 alias omissions found for %q", filter)})
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{"count": len(rows)},
		tableBlock("omissions", []string{"operation", "reason", "use"}, rows),
	})
}

func runAliasInspectLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut := takeJSONFlag(args)
	if err := rejectUnknownFlags(args, "alias inspect", aliasJSONFlags); err != nil {
		return err
	}
	if len(args) < 1 {
		return newUsageError("usage: fjgo alias inspect <command...>", "Run `fjgo alias list <filter>` to find aliases")
	}
	alias, ok := aliasByCommand(args)
	if !ok {
		return fmt.Errorf("unknown alias %q", strings.Join(args, " "))
	}
	if jsonOut {
		return writeJSON(stdout, aliasView(alias))
	}
	return writeTOON(stdout, aliasBlocks(alias))
}

package main

import (
	"context"
	"io"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var releaseListFlags = newFlagSet([]string{"--json", "--fields", "--limit", "--page", "--draft", "--prerelease", "--q"}, []string{"--fields", "--limit", "--page", "--draft", "--prerelease", "--q"})
var releaseViewFlags = newFlagSet([]string{"--json", "--full", "--tag", "--fields"}, []string{"--fields"})
var releaseLatestFlags = newFlagSet([]string{"--json", "--full", "--fields"}, []string{"--fields"})
var releaseCreateFlags = newFlagSet([]string{"--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body"})
var releaseEditFlags = newFlagSet([]string{"--tag-name", "--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--tag-name", "--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links"})
var releaseDeleteFlags = newFlagSet([]string{"--tag", "--yes", "--dry-run", "--print-request", "--json"}, nil)
var releaseUploadFlags = newFlagSet([]string{"--yes", "--dry-run", "--print-request", "--json", "--full"}, nil)
var releaseAssetListFlags = newFlagSet([]string{"--json", "--fields"}, []string{"--fields"})
var releaseAssetDeleteFlags = newFlagSet([]string{"--yes", "--dry-run", "--print-request", "--json"}, nil)
var releaseAssetParentFlags = newFlagSet([]string{"--fields", "--json", "--yes", "--dry-run", "--print-request"}, []string{"--fields"})

func releaseGroup() commandGroup {
	assets := releaseAssetGroup()
	return commandGroup{
		name:     "release",
		overview: releaseHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: releaseListHelp(), flags: releaseListFlags, run: releaseJSONLeaf(runReleaseList)},
			{names: []string{"view"}, help: releaseViewHelp(), flags: releaseViewFlags, run: releaseFullLeaf(runReleaseView)},
			{names: []string{"latest"}, help: releaseLatestHelp(), flags: releaseLatestFlags, run: releaseFullLeaf(runReleaseLatest)},
			{names: []string{"create"}, help: releaseCreateHelp(), flags: releaseCreateFlags, mutation: mutationRequired, run: releaseFullLeaf(runReleaseCreate)},
			{names: []string{"edit"}, help: releaseEditHelp(), flags: releaseEditFlags, mutation: mutationRequired, run: releaseFullLeaf(runReleaseEdit)},
			{names: []string{"delete"}, help: releaseDeleteHelp(), flags: releaseDeleteFlags, mutation: mutationRequired, run: releaseJSONLeaf(runReleaseDelete)},
			{names: []string{"upload"}, help: releaseUploadHelp(), flags: releaseUploadFlags, mutation: mutationRequired, run: releaseFullLeaf(runReleaseUpload)},
			{names: []string{"assets", "asset"}, help: releaseAssetParentHelp(), flags: releaseAssetParentFlags, children: &assets, run: dispatchFixed(releaseAssetGroup)},
		},
	}
}

func releaseCommandHelps() map[string]commandHelpSpec { return releaseGroup().helpSpecs() }

func releaseJSONLeaf(fn func(context.Context, *forgejo.Client, runConfig, []string, io.Writer, bool) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, _ := peelJSONFull(args)
		return fn(ctx, client, cfg, args, stdout, jsonOut)
	}
}

func releaseFullLeaf(fn func(context.Context, *forgejo.Client, runConfig, []string, io.Writer, bool, bool) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, full := peelJSONFull(args)
		return fn(ctx, client, cfg, args, stdout, jsonOut, full)
	}
}

func releaseAssetGroup() commandGroup {
	remove := commandLeaf{names: []string{"delete", "remove"}, help: releaseAssetDeleteHelp(), flags: releaseAssetDeleteFlags, mutation: mutationRequired, run: releaseJSONLeaf(runReleaseAssetDelete)}
	return commandGroup{
		name:     "release assets",
		overview: func() string { return releaseAssetOverview },
		leaves: []commandLeaf{
			{names: []string{"list"}, help: releaseAssetListHelp(), flags: releaseAssetListFlags, run: releaseJSONLeaf(runReleaseAssetsList)},
			remove,
		},
	}
}

const releaseAssetOverview = "usage: fjgo release assets <list|delete> [owner/repo] <release-id> [asset-id] [flags]\nexamples:\n  fjgo --repo OWNER/REPO release assets list 123\n  fjgo --repo OWNER/REPO release assets delete 123 456 --dry-run --yes"

func releaseListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--draft <bool>, --prerelease <bool>, --q <text>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release list", "fjgo release list OWNER/REPO --fields id,tag,title"}}
}
func releaseViewHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release view [owner/repo] <id|tag> [flags]", Flags: []string{"--tag", "--full", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release view v1.2.3", "fjgo release view OWNER/REPO 123 --full"}}
}
func releaseLatestHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release latest [owner/repo] [--full] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo --repo OWNER/REPO release latest", "fjgo release latest OWNER/REPO --fields id,tag,title"}}
}
func releaseCreateHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release create [owner/repo] <tag> [flags] --yes", Flags: []string{"--name <text>, --target <ref>", "--body|--notes <text>, --body-file|--notes-file <path>", "--draft, --prerelease, --hide-archive-links", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release create v1.2.3 --body-file notes.md --dry-run --yes", "fjgo release create OWNER/REPO v1.2.3 --yes"}}
}
func releaseEditHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release edit [owner/repo] <id> [flags] --yes", Flags: []string{"--name <text>, --target <ref>", "--body|--notes <text>, --body-file|--notes-file <path>", "--draft <bool>, --prerelease <bool>, --hide-archive-links <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release edit 123 --prerelease false --dry-run --yes", "fjgo release edit OWNER/REPO 123 --name \"v1.2.3\" --yes"}}
}
func releaseDeleteHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release delete [owner/repo] <id|tag> --yes", Flags: []string{"--tag", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release delete 123 --dry-run --yes", "fjgo release delete OWNER/REPO v1.2.3 --tag --yes"}}
}
func releaseUploadHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release upload [owner/repo] <release-id> <file> [name=value ...] --yes", Flags: []string{"--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release upload 123 dist/app.tar.gz name=app.tar.gz --dry-run --yes", "fjgo release upload OWNER/REPO 123 dist/app.tar.gz --yes"}}
}
func releaseAssetParentHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release assets <list|delete> [owner/repo] <release-id> [asset-id] [flags]", Flags: []string{"--fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO release assets list 123", "fjgo --repo OWNER/REPO release assets delete 123 456 --dry-run --yes"}}
}
func releaseAssetListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release assets list [owner/repo] <release-id> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin release assets list 123", "fjgo release assets list OWNER/REPO 123 --fields id,name,size"}}
}
func releaseAssetDeleteHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo release assets delete [owner/repo] <release-id> <asset-id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin release assets delete 123 456 --dry-run --yes", "fjgo release assets delete OWNER/REPO 123 456 --yes"}}
}

func runRelease(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, releaseHelp(), releaseGroup().helpSpecs()); ok {
		return writeHelp(stdout, help)
	}
	moved := deferBoolFlags(args, "--json", "--full")
	if _, _, _, err := releaseGroup().lookup(moved); err != nil {
		peeled, jsonOut, full := peelJSONFull(args)
		aliasArgs := append([]string{"repo", "releases"}, peeled...)
		if jsonOut {
			aliasArgs = append(aliasArgs, "--json")
		}
		if full {
			aliasArgs = append(aliasArgs, "--full")
		}
		return runAlias(ctx, client, cfg, aliasArgs, stdout)
	}
	return dispatchGroup(releaseGroup(), ctx, client, cfg, moved, stdout)
}

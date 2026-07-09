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

var releaseAssetFields = []string{"id", "name", "size", "downloads", "created", "url"}
var releaseFields = []string{"id", "tag", "title", "draft", "prerelease", "target", "author", "created", "published", "url", "body"}
var releaseListDefaultFields = []string{"id", "tag", "title", "draft", "prerelease"}
var releaseViewDefaultFields = []string{"id", "tag", "title", "draft", "prerelease", "target", "body"}

func runReleaseList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "release list", []string{"--json", "--fields", "--limit", "--page", "--draft", "--prerelease", "--q"}, []string{"--fields", "--limit", "--page", "--draft", "--prerelease", "--q"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, releaseListDefaultFields, releaseFields, "release list")
	if err != nil {
		return err
	}
	query := url.Values{"limit": {defaultListLimit}}
	for _, spec := range []struct{ flag, key string }{
		{"--limit", "limit"},
		{"--page", "page"},
		{"--draft", "draft"},
		{"--prerelease", "pre-release"},
		{"--q", "q"},
	} {
		var value string
		var ok bool
		args, value, ok, err = takeValueFlag(args, spec.flag)
		if err != nil {
			return err
		}
		if ok {
			query.Set(spec.key, value)
		}
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo release list [owner/repo]", "Pass `owner/repo`, root or command `--repo OWNER/REPO`, FJGO_REPO, or `-R origin`")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListReleases", repoPath(ref), query, nil)
	if err != nil {
		return err
	}
	releases, err := decodeBody[[]*forgejo.Release](resp.Body)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, releases)
	}
	rows := make([]map[string]any, 0, len(releases))
	for _, release := range releases {
		rows = append(rows, releaseRow(release, true))
	}
	return writeRows(stdout, "releases", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 releases found for %s/%s", ref.Owner, ref.Repo), suggestionLines(suggestionContext{
		Domain: "release",
		Action: "list",
		Empty:  len(rows) == 0,
		Repo:   refPtr(ref),
	}))
}

func runReleaseView(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "release view", []string{"--json", "--full", "--tag", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, byTag := boolFlag(args, "--tag")
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, releaseViewDefaultFields, releaseFields, "release view")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo release view [owner/repo] <id|tag>")
	}
	operation := "repoGetRelease"
	pathValues := repoPath(ref)
	if byTag || !isInteger(rest[0]) {
		operation = "repoGetReleaseByTag"
		pathValues["tag"] = rest[0]
	} else {
		pathValues["id"] = rest[0]
	}
	resp, err := rawOperationResponse(ctx, client, operation, pathValues, nil, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	release, err := decodeBody[*forgejo.Release](resp.Body)
	if err != nil {
		return err
	}
	return writeRelease(stdout, release, full, ref, fields)
}

func runReleaseLatest(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "release latest", []string{"--json", "--full", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, releaseViewDefaultFields, releaseFields, "release latest")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo release latest [owner/repo]")
	}
	resp, err := rawOperationResponse(ctx, client, "repoGetLatestRelease", repoPath(ref), nil, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	release, err := decodeBody[*forgejo.Release](resp.Body)
	if err != nil {
		return err
	}
	return writeRelease(stdout, release, full, ref, fields)
}

func runReleaseEdit(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "release edit", []string{"--tag-name", "--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--tag-name", "--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links"}); err != nil {
		return err
	}
	args, err := normalizeReleaseNotesArgs(args)
	if err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, bodyText, bodySet, err := takeBodyText(args, false)
	if err != nil {
		return err
	}
	body := map[string]any{}
	for _, spec := range []struct{ flag, key string }{
		{"--tag-name", "tag_name"},
		{"--name", "name"},
		{"--target", "target_commitish"},
	} {
		args, err = takeStringBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	for _, spec := range []struct{ flag, key string }{
		{"--draft", "draft"},
		{"--prerelease", "prerelease"},
		{"--hide-archive-links", "hide_archive_links"},
	} {
		args, err = takeBoolBodyFlag(args, spec.flag, spec.key, body)
		if err != nil {
			return err
		}
	}
	if bodySet {
		body["body"] = bodyText
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo release edit [owner/repo] <id> [flags] --yes")
	}
	if len(body) == 0 {
		return newUsageError("release edit requires at least one change")
	}
	if !yes {
		return newUsageError("release edit requires --yes")
	}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoEditRelease", ref, map[string]string{"id": rest[0]}, body, yes)
	}
	op, _ := forgejo.OperationByID("repoEditRelease")
	pathValues := repoPath(ref)
	pathValues["id"] = rest[0]
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(out)
		return err
	}
	return writeJSONAsTOON(stdout, out, "release", full)
}

func runReleaseDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "release delete", []string{"--tag", "--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	args, byTag := boolFlag(args, "--tag")
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo release delete [owner/repo] <id|tag> --yes")
	}
	if !yes {
		return newUsageError("release delete requires --yes")
	}
	operation := "repoDeleteRelease"
	extra := map[string]string{"id": rest[0]}
	if byTag || !isInteger(rest[0]) {
		operation = "repoDeleteReleaseByTag"
		extra = map[string]string{"tag": rest[0]}
	}
	if dryRun {
		return writeMutationPreview(stdout, client, operation, ref, extra, nil, yes)
	}
	op, _ := forgejo.OperationByID(operation)
	pathValues := repoPath(ref)
	for key, value := range extra {
		pathValues[key] = value
	}
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"release": rest[0], "deleted": true})
	}
	if len(out) != 0 {
		return writeJSONAsTOON(stdout, out, "release", false)
	}
	return writeTOON(stdout, map[string]any{"release": rest[0] + " deleted"})
}

func runReleaseAssets(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo release assets <list|delete> [owner/repo] <release-id> [asset-id] [flags]\nexamples:\n  fjgo --repo OWNER/REPO release assets list 123\n  fjgo --repo OWNER/REPO release assets delete 123 456 --dry-run --yes")
	}
	switch args[0] {
	case "list":
		return runReleaseAssetsList(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "delete", "remove":
		return runReleaseAssetDelete(ctx, client, cfg, args[1:], stdout, jsonOut)
	default:
		return unknownSubcommandError("release assets", args[0], []string{"list", "delete", "remove"})
	}
}

func runReleaseAssetsList(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "release assets list", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
		return err
	}
	args, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, []string{"id", "name", "size", "downloads"}, releaseAssetFields, "release assets list")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 1 {
		return newUsageError("usage: fjgo release assets list [owner/repo] <release-id>")
	}
	resp, err := rawOperationResponse(ctx, client, "repoListReleaseAttachments", repoPathWith(ref, map[string]string{"id": rest[0]}), nil, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		_, err = stdout.Write(resp.Body)
		return err
	}
	assets, err := decodeBody[[]*forgejo.Attachment](resp.Body)
	if err != nil {
		return err
	}
	rows := make([]map[string]any, 0, len(assets))
	for _, asset := range assets {
		if asset != nil {
			rows = append(rows, releaseAssetRow(asset))
		}
	}
	return writeRows(stdout, "assets", rowsSelect(rows, fields), fields, responseTotal(resp), fmt.Sprintf("0 assets found for release %s", rest[0]), nil)
}

func runReleaseAssetDelete(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut bool) error {
	if err := rejectUnknownFlags(args, "release assets delete", []string{"--yes", "--dry-run", "--print-request", "--json"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) != 2 {
		return newUsageError("usage: fjgo release assets delete [owner/repo] <release-id> <asset-id> --yes")
	}
	if !yes {
		return newUsageError("release assets delete requires --yes")
	}
	extra := map[string]string{"id": rest[0], "attachment_id": rest[1]}
	if dryRun {
		return writeMutationPreview(stdout, client, "repoDeleteReleaseAttachment", ref, extra, nil, yes)
	}
	op, _ := forgejo.OperationByID("repoDeleteReleaseAttachment")
	out, err := client.DoOperationRaw(ctx, op, repoPathWith(ref, extra), forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, map[string]any{"release": rest[0], "asset": rest[1], "deleted": true})
	}
	if len(out) != 0 {
		return writeJSONAsTOON(stdout, out, "asset", false)
	}
	return writeTOON(stdout, map[string]any{"asset": rest[1] + " deleted"})
}

func writeRelease(stdout io.Writer, release *forgejo.Release, full bool, ref repoRef, fields []string) error {
	if release == nil {
		return writeTOON(stdout, map[string]any{"release": "not found"})
	}
	row := releaseRow(release, !full)
	if len(fields) == 0 {
		fields = releaseViewDefaultFields
	}
	blocks := toonBlocks{map[string]any{"release": selectFields(row, fields)}}
	if !full && len([]rune(release.Note)) > defaultTruncateChars {
		blocks = append(blocks, helpBlock([]string{fmt.Sprintf("Run `fjgo --repo %s/%s release view %d --full` to see complete release notes", ref.Owner, ref.Repo, release.ID)}))
	}
	return writeTOON(stdout, blocks)
}

func releaseRow(release *forgejo.Release, truncate bool) map[string]any {
	if release == nil {
		return map[string]any{}
	}
	body := release.Note
	if truncate {
		var report truncateReport
		body = truncateString(body, defaultTruncateChars, "release.body", &report)
	}
	title := release.Title
	if title == "" {
		title = release.TagName
	}
	return map[string]any{
		"id":         release.ID,
		"tag":        release.TagName,
		"title":      title,
		"draft":      release.IsDraft,
		"prerelease": release.IsPrerelease,
		"target":     release.Target,
		"author":     userName(release.Author),
		"created":    release.CreatedAt,
		"published":  release.PublishedAt,
		"url":        release.HTMLURL,
		"body":       body,
	}
}

func releaseAssetRow(asset *forgejo.Attachment) map[string]any {
	if asset == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id":        asset.ID,
		"name":      asset.Name,
		"size":      asset.Size,
		"downloads": asset.DownloadCount,
		"created":   asset.Created,
		"url":       asset.DownloadURL,
	}
}

func normalizeReleaseNotesArgs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--notes":
			out = append(out, "--body")
		case strings.HasPrefix(arg, "--notes="):
			out = append(out, "--body="+strings.TrimPrefix(arg, "--notes="))
		case arg == "--notes-file":
			out = append(out, "--body-file")
		case strings.HasPrefix(arg, "--notes-file="):
			out = append(out, "--body-file="+strings.TrimPrefix(arg, "--notes-file="))
		default:
			out = append(out, arg)
		}
	}
	return out, nil
}

func repoPathWith(ref repoRef, extra map[string]string) map[string]string {
	values := repoPath(ref)
	for key, value := range extra {
		values[key] = value
	}
	return values
}

func isInteger(raw string) bool {
	if raw == "" {
		return false
	}
	_, err := strconv.ParseInt(raw, 10, 64)
	return err == nil
}

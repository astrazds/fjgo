package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

const (
	defaultUpdateBaseURL = "https://api.github.com"
	defaultUpdateRepo    = "astrazds/fjgo"
)

func runUpdate(ctx context.Context, httpClient *http.Client, args []string, stdout io.Writer) error {
	if len(args) != 0 && hasHelp(args) {
		return writeHelp(stdout, updateHelp())
	}
	if err := rejectUnknownFlags(args, "update", []string{"--check", "--yes", "--dry-run", "--print-request", "--json", "--base-url", "--repo"}, []string{"--base-url", "--repo"}); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	args, check := boolFlag(args, "--check")
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	baseURL := getenv("FJGO_UPDATE_BASE_URL", defaultUpdateBaseURL)
	repo := defaultUpdateRepo
	var value string
	var ok bool
	var err error
	args, value, ok, err = takeValueFlag(args, "--base-url")
	if err != nil {
		return err
	}
	if ok {
		baseURL = value
	}
	args, value, ok, err = takeValueFlag(args, "--repo")
	if err != nil {
		return err
	}
	if ok {
		repo = value
	}
	if len(args) != 0 {
		return newUsageError("usage: fjgo update [--check|--dry-run|--yes]", "Run `fjgo update --check` first")
	}
	owner, name, err := splitRepo(repo)
	if err != nil {
		return err
	}
	updateClient, err := forgejo.NewClient(baseURL, "", httpClient)
	if err != nil {
		return err
	}
	resp, err := rawOperationResponse(ctx, updateClient, "repoGetLatestRelease", map[string]string{"owner": owner, "repo": name}, nil, nil)
	if err != nil {
		return err
	}
	release, err := decodeBody[*forgejo.Release](resp.Body)
	if err != nil {
		return err
	}
	status := updateStatus(release, baseURL, repo)
	if check || dryRun {
		if jsonOut {
			return writeJSON(stdout, status)
		}
		return writeTOON(stdout, map[string]any{"update": status})
	}
	if !yes {
		return newUsageError("update requires --yes", "Run `fjgo update --check` to inspect the latest release first")
	}
	assetURL, _ := status["asset_url"].(string)
	if assetURL == "" {
		return fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	asset, err := updateClient.GetExternalRawResponse(ctx, assetURL)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := replaceExecutableFromArchive(asset.Body, exe); err != nil {
		return err
	}
	status["updated"] = true
	status["path"] = exe
	if jsonOut {
		return writeJSON(stdout, status)
	}
	return writeTOON(stdout, map[string]any{"update": status})
}

func updateHelp() string {
	return `usage: fjgo update [--check|--dry-run|--yes]

flags:
  --check              check the latest fjgo release without changing files
  --dry-run            show the selected release asset without changing files
  --yes                replace the current executable with the selected asset
  --base-url <url>     API base URL that hosts fjgo releases
  --repo <owner/repo>  repository that publishes fjgo releases
  --json               output JSON instead of TOON

examples:
  fjgo update --check
  fjgo update --dry-run
  fjgo update --yes`
}

func updateStatus(release *forgejo.Release, baseURL, repo string) map[string]any {
	latest := ""
	if release != nil {
		latest = release.TagName
	}
	status := map[string]any{
		"current":          version,
		"latest":           latest,
		"update_available": latest != "" && strings.TrimPrefix(latest, "v") != strings.TrimPrefix(version, "v"),
		"base_url":         baseURL,
		"repo":             repo,
		"asset":            updateAssetName(latest),
	}
	if release != nil {
		for _, asset := range release.Attachments {
			if asset == nil {
				continue
			}
			if asset.Name == updateAssetName(latest) {
				status["asset_url"] = asset.DownloadURL
				status["asset_size"] = asset.Size
				break
			}
		}
	}
	return status
}

func updateAssetName(tag string) string {
	if tag == "" {
		tag = version
	}
	return fmt.Sprintf("fjgo_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
}

func replaceExecutableFromArchive(archive []byte, exe string) error {
	gr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != filepath.Base(exe) {
			continue
		}
		tmp, err := os.CreateTemp(filepath.Dir(exe), ".fjgo-update-*")
		if err != nil {
			return err
		}
		tmpPath := tmp.Name()
		_, copyErr := io.Copy(tmp, tr)
		closeErr := tmp.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			return copyErr
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			return closeErr
		}
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		return os.Rename(tmpPath, exe)
	}
	return fmt.Errorf("archive did not contain %s", filepath.Base(exe))
}

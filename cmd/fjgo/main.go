package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"repos.astrazds.net/astrazds/fjgo/internal/fjgoskill"
	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

const defaultBaseURL = "https://v15.next.forgejo.org/api/v1"

var (
	version = "v0.13.0"
	commit  = "none"
	date    = "unknown"
)

type repoRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type runConfig struct {
	BaseURL    string
	Token      string
	RemoteRepo *repoRef
	RemoteName string
	RemoteURL  string
}

type doctorRelease struct {
	ID      int64  `json:"id"`
	TagName string `json:"tag_name,omitempty"`
	Name    string `json:"name,omitempty"`
	HTMLURL string `json:"html_url,omitempty"`
	Draft   bool   `json:"draft,omitempty"`
}

func main() {
	if code := runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := run(ctx, args, stdout, stderr); err != nil {
		if jsonErrorRequested(args) {
			_ = writeJSON(stderr, errorView(args, err))
		} else {
			fmt.Fprintln(stderr, "fjgo:", errorMessage(err))
		}
		return 1
	}
	return 0
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	envBaseURL := os.Getenv("FJGO_BASE_URL")
	baseURL := fs.String("base-url", getenv("FJGO_BASE_URL", defaultBaseURL), "Forgejo API base URL")
	jsonErrors := fs.Bool("json", false, "print errors as JSON when used before the command")
	token := fs.String("token", "", "Forgejo access token (or FJGO_TOKEN)")
	timeout := fs.Duration("timeout", 15*time.Second, "HTTP timeout")
	showVersion := fs.Bool("version", false, "print fjgo version")
	remote := fs.String("R", "", "resolve owner/repo from git remote")
	remoteLong := fs.String("repo-from-remote", "", "resolve owner/repo from git remote")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: fjgo [flags] <version|doctor|me|auth|get|api|alias|model|repo|release|skill|install>")
		fmt.Fprintln(stderr, "\ncommands:")
		fmt.Fprintln(stderr, "  version     print the Forgejo server version")
		fmt.Fprintln(stderr, "  doctor      print redacted agent diagnostics as JSON")
		fmt.Fprintln(stderr, "  me          print the authenticated user")
		fmt.Fprintln(stderr, "  auth status print base-url/token diagnostics")
		fmt.Fprintln(stderr, "  get <path>  GET an API path, for example /version")
		fmt.Fprintln(stderr, "  api list [filter]")
		fmt.Fprintln(stderr, "  api inspect <operationId>")
		fmt.Fprintln(stderr, "  api call <operationId> [name=value ...] [-body JSON|@file|-] [--yes]")
		fmt.Fprintln(stderr, "  api upload <operationId> [name=value ...] attachment=@file [--yes]")
		fmt.Fprintln(stderr, "  alias list")
		fmt.Fprintln(stderr, "  alias inspect <command...>")
		fmt.Fprintln(stderr, "  alias collisions")
		fmt.Fprintln(stderr, "  model inspect <Model>")
		fmt.Fprintln(stderr, "  repo get <owner/repo>")
		fmt.Fprintln(stderr, "  repo topics <owner/repo> [--set comma,separated,topics] [--yes] [--dry-run]")
		fmt.Fprintln(stderr, "  repo avatar <owner/repo> <png> --yes [--dry-run]")
		fmt.Fprintln(stderr, "  release list [owner/repo]")
		fmt.Fprintln(stderr, "  release create [owner/repo] <tag> [name=value ...] --yes [--dry-run]")
		fmt.Fprintln(stderr, "  release upload [owner/repo] <release-id> <file> [name=value ...] --yes")
		fmt.Fprintln(stderr, "  skill install [--dir path] [--force]")
		fmt.Fprintln(stderr, "  skill status [--dir path]")
		fmt.Fprintln(stderr, "  install --skills [--dir path] [--force|--check]")
		fmt.Fprintln(stderr, "\nflags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "fjgo %s %s %s\n", version, commit, date)
		return nil
	}
	_ = *jsonErrors
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("missing command")
	}
	baseURLConfigured := envBaseURL != ""
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "base-url" {
			baseURLConfigured = true
		}
	})
	if *token == "" && (*baseURL != defaultBaseURL || baseURLConfigured) {
		*token = os.Getenv("FJGO_TOKEN")
	}
	if *remoteLong != "" {
		*remote = *remoteLong
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	client, err := forgejo.NewClient(*baseURL, *token, http.DefaultClient)
	if err != nil {
		return err
	}
	cfg := runConfig{BaseURL: *baseURL, Token: *token}
	if *remote != "" {
		ref, remoteURL, err := resolveRemoteRepo(*remote, *baseURL)
		if err != nil {
			return err
		}
		cfg.RemoteRepo = ref
		cfg.RemoteName = *remote
		cfg.RemoteURL = remoteURL
	}

	switch fs.Arg(0) {
	case "doctor", "context":
		return runDoctor(ctx, client, cfg, fs.Args()[1:], stdout)
	case "version":
		body, err := callOperationRaw(ctx, client, "getVersion", nil, nil)
		if err != nil {
			return err
		}
		_, err = stdout.Write(body)
		return err
	case "me":
		user, err := client.Me(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, user)
	case "auth", "whoami":
		return runAuth(ctx, client, cfg, fs.Args()[1:], stdout)
	case "get":
		if fs.NArg() != 2 {
			return errors.New("usage: fjgo get <path>")
		}
		body, err := client.GetRaw(ctx, fs.Arg(1))
		if err != nil {
			return err
		}
		_, err = stdout.Write(body)
		return err
	case "api":
		return runAPI(ctx, client, fs.Args()[1:], stdout, stderr)
	case "alias":
		return runAliasInfo(fs.Args()[1:], stdout)
	case "model":
		return runModel(fs.Args()[1:], stdout)
	case "repo":
		return runRepo(ctx, client, cfg, fs.Args()[1:], stdout)
	case "release":
		return runRelease(ctx, client, cfg, fs.Args()[1:], stdout)
	case "skill":
		return runSkill(fs.Args()[1:], stdout, stderr)
	case "install":
		return runInstall(fs.Args()[1:], stdout, stderr)
	default:
		return runAlias(ctx, client, cfg, fs.Args(), stdout)
	}
}

func runAPI(ctx context.Context, client *forgejo.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo api <list|inspect|call|upload>")
	}
	args, jsonOut := takeJSONFlag(args)
	switch args[0] {
	case "list":
		filter := ""
		if len(args) > 1 {
			filter = strings.ToLower(args[1])
		}
		var out []forgejo.Operation
		for _, op := range forgejo.Operations() {
			line := fmt.Sprintf("%-45s %-6s %s", op.ID, op.Method, op.Path)
			if filter == "" || strings.Contains(strings.ToLower(line+" "+op.Summary), filter) {
				if jsonOut {
					out = append(out, op)
					continue
				}
				fmt.Fprintln(stdout, line)
			}
		}
		if jsonOut {
			return writeJSON(stdout, out)
		}
		return nil
	case "inspect":
		if len(args) != 2 {
			return errors.New("usage: fjgo api inspect <operationId>")
		}
		op, ok := forgejo.OperationByID(args[1])
		if !ok {
			return fmt.Errorf("unknown operation %q", args[1])
		}
		if jsonOut {
			return writeJSON(stdout, operationView(op))
		}
		writeOperation(stdout, op)
		return nil
	case "call":
		return callOperation(ctx, client, args[1:], stdout, stderr)
	case "upload":
		return uploadOperation(ctx, client, args[1:], stdout)
	default:
		return fmt.Errorf("unknown api command %q", args[0])
	}
}

func writeOperation(w io.Writer, op forgejo.Operation) {
	fmt.Fprintf(w, "id: %s\n", op.ID)
	fmt.Fprintf(w, "method: %s\n", op.Method)
	fmt.Fprintf(w, "path: %s\n", op.Path)
	if op.Summary != "" {
		fmt.Fprintf(w, "summary: %s\n", op.Summary)
	}
	if len(op.PathParams) != 0 {
		fmt.Fprintf(w, "path_params: %s\n", strings.Join(op.PathParams, ", "))
	}
	writeOperationParams(w, "query_params", op.QueryParams)
	if op.BodyType != "" {
		fmt.Fprintf(w, "body: %s\n", op.BodyType)
		writeModelFields(w, "body_fields", op.BodyType)
	}
	writeOperationParams(w, "form_params", op.FormParams)
	if op.ReturnType != "" {
		fmt.Fprintf(w, "returns: %s\n", op.ReturnType)
	}
	if op.Upload {
		fmt.Fprintln(w, "upload: multipart/form-data")
	}
}

type operationInspect struct {
	forgejo.Operation
	BodyFields []forgejo.ModelField `json:"body_fields,omitempty"`
}

type aliasInspect struct {
	Command       []string         `json:"command"`
	Operation     string           `json:"operation"`
	Args          []string         `json:"args,omitempty"`
	Requires      []string         `json:"requires,omitempty"`
	OperationInfo operationInspect `json:"operation_info"`
}

type requestPreview struct {
	Operation   string               `json:"operation,omitempty"`
	Method      string               `json:"method"`
	Path        string               `json:"path"`
	Query       url.Values           `json:"query,omitempty"`
	Body        any                  `json:"body,omitempty"`
	Form        map[string]string    `json:"form,omitempty"`
	Files       []forgejo.UploadPart `json:"files,omitempty"`
	Upload      bool                 `json:"upload,omitempty"`
	AuthPresent bool                 `json:"auth_present"`
	RequiresYes bool                 `json:"requires_yes"`
	YesProvided bool                 `json:"yes_provided"`
}

func operationView(op forgejo.Operation) operationInspect {
	view := operationInspect{Operation: op}
	if op.BodyType != "" {
		if model, ok := forgejo.ModelByName(strings.TrimPrefix(op.BodyType, "*")); ok {
			view.BodyFields = model.Fields
		}
	}
	return view
}

func writeOperationParams(w io.Writer, label string, params []forgejo.OperationParam) {
	if len(params) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	for _, p := range params {
		required := ""
		if p.Required {
			required = " required"
		}
		desc := ""
		if p.Description != "" {
			desc = " - " + p.Description
		}
		fmt.Fprintf(w, "  %s: %s%s%s\n", p.Name, p.Type, required, desc)
	}
}

func callOperation(ctx context.Context, client *forgejo.Client, args []string, stdout, stderr io.Writer) error {
	args, body, err := takeBodyFlag(args)
	if err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	if len(args) == 0 {
		return errors.New("usage: fjgo api call <operationId> [name=value ...]")
	}
	op, ok := forgejo.OperationByID(args[0])
	if !ok {
		return fmt.Errorf("unknown operation %q", args[0])
	}
	if op.Upload {
		return fmt.Errorf("%s uses multipart/form-data upload; use `fjgo api upload %s ... attachment=@file --yes`", op.ID, op.ID)
	}
	if op.Method != http.MethodGet && !yes {
		return fmt.Errorf("%s requires --yes", op.ID)
	}
	pathValues, query, err := splitArgs(op, args[1:])
	if err != nil {
		return err
	}
	if missing := missingPathParams(op, pathValues); len(missing) != 0 {
		return fmt.Errorf("missing path parameter %q for %s; run `fjgo api inspect %s`", missing[0], op.ID, op.ID)
	}
	var bodyValue any
	if body != "" {
		bodyValue, err = readJSONBody(body)
		if err != nil {
			return err
		}
	} else if op.BodyType != "" {
		return missingBodyError(op)
	}
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Body:        bodyValue,
			AuthPresent: clientHasAuth(client),
			RequiresYes: op.Method != http.MethodGet,
			YesProvided: yes,
		})
	}
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{
		Query: query,
		Body:  bodyValue,
	})
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	_, err = stdout.Write(out)
	return err
}

func uploadOperation(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer) error {
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	if len(args) == 0 {
		return errors.New("usage: fjgo api upload <operationId> [name=value ...] attachment=@file --yes")
	}
	op, ok := forgejo.OperationByID(args[0])
	if !ok {
		return fmt.Errorf("unknown operation %q", args[0])
	}
	if !op.Upload {
		return fmt.Errorf("%s is not a multipart upload operation", op.ID)
	}
	if !yes {
		return fmt.Errorf("%s requires --yes", op.ID)
	}
	pathValues, query, fields, files, err := splitUploadArgs(op, args[1:])
	if err != nil {
		return err
	}
	if missing := missingPathParams(op, pathValues); len(missing) != 0 {
		return fmt.Errorf("missing path parameter %q for %s; run `fjgo api inspect %s`", missing[0], op.ID, op.ID)
	}
	if len(files) == 0 && fields["external_url"] == "" {
		return fmt.Errorf("%s requires attachment=@file or external_url=value", op.ID)
	}
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Form:        fields,
			Files:       files,
			Upload:      true,
			AuthPresent: clientHasAuth(client),
			RequiresYes: true,
			YesProvided: yes,
		})
	}
	out, err := client.DoOperationMultipart(ctx, op, pathValues, query, fields, files, nil)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	_, err = stdout.Write(out)
	return err
}

func runRepo(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo repo <get|topics> <owner/repo>; fjgo repo avatar <owner/repo> <png>")
	}
	switch args[0] {
	case "get":
		ref, rest, err := repoFromArgs(cfg, args[1:])
		if err != nil || len(rest) != 0 {
			return errors.New("usage: fjgo repo get <owner/repo>")
		}
		out, err := client.RepoGet(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeJSON(stdout, out)
	case "topics":
		ref, rest, err := repoFromArgs(cfg, args[1:])
		if err != nil {
			return errors.New("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]")
		}
		args, yes := takeYesFlag(rest)
		args, dryRun := takeDryRunFlag(args)
		args, topics, err := takeSetFlag(args)
		if err != nil {
			return err
		}
		if len(args) != 0 {
			return errors.New("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]")
		}
		if topics != "" {
			if !yes {
				return errors.New("repo topics --set requires --yes")
			}
			if dryRun {
				op, _ := forgejo.OperationByID("repoUpdateTopics")
				return writeRequestPreview(stdout, requestPreview{
					Operation:   op.ID,
					Method:      op.Method,
					Path:        previewPath(op, map[string]string{"owner": ref.Owner, "repo": ref.Repo}),
					Body:        map[string]any{"topics": splitCSV(topics)},
					AuthPresent: clientHasAuth(client),
					RequiresYes: true,
					YesProvided: yes,
				})
			}
			if err := client.RepoUpdateTopics(ctx, ref.Owner, ref.Repo, &forgejo.RepoTopicOptions{Topics: splitCSV(topics)}, forgejo.RequestOptions{}); err != nil {
				return err
			}
		}
		out, err := client.RepoListTopics(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeJSON(stdout, out)
	case "avatar":
		ref, rest, err := repoFromArgs(cfg, args[1:])
		if err != nil {
			return errors.New("usage: fjgo repo avatar <owner/repo> <png> --yes")
		}
		rest, yes := takeYesFlag(rest)
		rest, dryRun := takeDryRunFlag(rest)
		if len(rest) != 1 {
			return errors.New("usage: fjgo repo avatar <owner/repo> <png> --yes")
		}
		if !yes {
			return errors.New("repo avatar requires --yes")
		}
		if dryRun {
			op, _ := forgejo.OperationByID("repoUpdateAvatar")
			return writeRequestPreview(stdout, requestPreview{
				Operation:   op.ID,
				Method:      op.Method,
				Path:        previewPath(op, map[string]string{"owner": ref.Owner, "repo": ref.Repo}),
				Body:        map[string]string{"image_file": rest[0], "image_base64": "omitted"},
				AuthPresent: clientHasAuth(client),
				RequiresYes: true,
				YesProvided: yes,
			})
		}
		b, err := os.ReadFile(rest[0])
		if err != nil {
			return err
		}
		return client.RepoUpdateAvatar(ctx, ref.Owner, ref.Repo, &forgejo.UpdateRepoAvatarOption{Image: base64.StdEncoding.EncodeToString(b)}, forgejo.RequestOptions{})
	case "issue":
		return runRepoIssue(ctx, client, cfg, args[1:], stdout)
	default:
		return runAlias(ctx, client, cfg, append([]string{"repo"}, args...), stdout)
	}
}

func runRelease(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo release <list|create|upload>")
	}
	switch args[0] {
	case "list":
		ref, rest, err := repoFromArgs(cfg, args[1:])
		if err != nil || len(rest) != 0 {
			return errors.New("usage: fjgo release list [owner/repo]")
		}
		out, err := client.RepoListReleases(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeJSON(stdout, out)
	case "create":
		return runReleaseCreate(ctx, client, cfg, args[1:], stdout)
	case "upload":
		return runReleaseUpload(ctx, client, cfg, args[1:], stdout)
	default:
		return runAlias(ctx, client, cfg, append([]string{"repo", "releases"}, args...), stdout)
	}
}

func runReleaseCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) < 1 {
		return errors.New("usage: fjgo release create [owner/repo] <tag> [name=value ...] --yes")
	}
	if !yes {
		return errors.New("release create requires --yes")
	}
	op, ok := forgejo.OperationByID("repoCreateRelease")
	if !ok {
		return errors.New("missing repoCreateRelease operation")
	}
	body := map[string]any{"tag_name": rest[0], "name": rest[0]}
	for _, arg := range rest[1:] {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return fmt.Errorf("expected name=value, got %q", arg)
		}
		switch name {
		case "body", "name", "target_commitish":
			body[name] = value
		case "draft", "hide_archive_links", "prerelease":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s expects bool: %w", name, err)
			}
			body[name] = parsed
		default:
			return fmt.Errorf("unknown release create option %q", name)
		}
	}
	pathValues := map[string]string{"owner": ref.Owner, "repo": ref.Repo}
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Body:        body,
			AuthPresent: clientHasAuth(client),
			RequiresYes: true,
			YesProvided: yes,
		})
	}
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	_, err = stdout.Write(out)
	return err
}

func runReleaseUpload(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) < 2 {
		return errors.New("usage: fjgo release upload [owner/repo] <release-id> <file> [name=value ...] --yes")
	}
	if !yes {
		return errors.New("release upload requires --yes")
	}
	op, ok := forgejo.OperationByID("repoCreateReleaseAttachment")
	if !ok {
		return errors.New("missing repoCreateReleaseAttachment operation")
	}
	pathValues := map[string]string{"owner": ref.Owner, "repo": ref.Repo, "id": rest[0]}
	filePath := rest[1]
	fields := map[string]string{}
	query := url.Values{}
	for _, arg := range rest[2:] {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return fmt.Errorf("expected name=value, got %q", arg)
		}
		if isFormParam(op, name) && name != "attachment" {
			fields[name] = value
			continue
		}
		query.Add(name, value)
	}
	files := []forgejo.UploadPart{{FieldName: "attachment", FilePath: filePath}}
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Form:        fields,
			Files:       files,
			Upload:      true,
			AuthPresent: clientHasAuth(client),
			RequiresYes: true,
			YesProvided: yes,
		})
	}
	out, err := client.DoOperationMultipart(ctx, op, pathValues, query, fields, files, nil)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	_, err = stdout.Write(out)
	return err
}

func runRepoIssue(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo repo issue <close|comment>")
	}
	switch args[0] {
	case "close":
		args, yes := takeYesFlag(args[1:])
		args, dryRun := takeDryRunFlag(args)
		ref, rest, err := repoFromArgs(cfg, args)
		if err != nil || len(rest) != 1 {
			return errors.New("usage: fjgo repo issue close [owner/repo] <index> --yes")
		}
		if !yes {
			return errors.New("repo issue close requires --yes")
		}
		op, _ := forgejo.OperationByID("issueEditIssue")
		pathValues := map[string]string{"owner": ref.Owner, "repo": ref.Repo, "index": rest[0]}
		body := map[string]string{"state": "closed"}
		if dryRun {
			return writeRequestPreview(stdout, requestPreview{Operation: op.ID, Method: op.Method, Path: previewPath(op, pathValues), Body: body, AuthPresent: clientHasAuth(client), RequiresYes: true, YesProvided: yes})
		}
		out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
		if err != nil {
			return err
		}
		_, err = stdout.Write(out)
		return err
	case "comment":
		args, body, err := takeBodyFlag(args[1:])
		if err != nil {
			return err
		}
		args, yes := takeYesFlag(args)
		args, dryRun := takeDryRunFlag(args)
		ref, rest, err := repoFromArgs(cfg, args)
		if err != nil || len(rest) != 1 || body == "" {
			return errors.New("usage: fjgo repo issue comment [owner/repo] <index> -body '{\"body\":\"...\"}' --yes")
		}
		if !yes {
			return errors.New("repo issue comment requires --yes")
		}
		bodyValue, err := readJSONBody(body)
		if err != nil {
			return err
		}
		op, _ := forgejo.OperationByID("issueCreateComment")
		pathValues := map[string]string{"owner": ref.Owner, "repo": ref.Repo, "index": rest[0]}
		if dryRun {
			return writeRequestPreview(stdout, requestPreview{Operation: op.ID, Method: op.Method, Path: previewPath(op, pathValues), Body: bodyValue, AuthPresent: clientHasAuth(client), RequiresYes: true, YesProvided: yes})
		}
		out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: bodyValue})
		if err != nil {
			return err
		}
		_, err = stdout.Write(out)
		return err
	default:
		return fmt.Errorf("unknown repo issue command %q", args[0])
	}
}

func runDoctor(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	args, _ = takeJSONFlag(args)
	if len(args) > 1 {
		return errors.New("usage: fjgo doctor [owner/repo] [--json]")
	}
	var ref *repoRef
	if len(args) == 1 {
		owner, repo, err := splitRepo(args[0])
		if err != nil {
			return err
		}
		ref = &repoRef{Owner: owner, Repo: repo}
	} else {
		ref = cfg.RemoteRepo
	}
	exe, _ := os.Executable()
	report := map[string]any{
		"tool": map[string]string{
			"name":    "fjgo",
			"version": version,
			"commit":  commit,
			"date":    date,
		},
		"runtime": map[string]string{
			"goos":       runtime.GOOS,
			"goarch":     runtime.GOARCH,
			"go_version": runtime.Version(),
		},
		"executable":          exe,
		"base_url":            cfg.BaseURL,
		"base_url_configured": cfg.BaseURL != defaultBaseURL,
		"token_present":       cfg.Token != "",
		"field_feedback":      "Share this redacted JSON with failures; it does not include token values.",
		"commands": []string{
			"fjgo -R origin doctor --json",
			"fjgo -R origin auth status",
			"fjgo alias inspect repo issues create",
			"fjgo -R origin repo issues list state=open",
			"fjgo -R origin release list",
		},
		"skill": map[string]any{
			"default_install_path": fjgoskill.DefaultInstallDir(),
			"install_command":      "fjgo skill install",
			"status":               fjgoskill.Check(""),
		},
	}
	if cfg.RemoteName != "" || cfg.RemoteURL != "" {
		report["git"] = map[string]any{
			"remote":     cfg.RemoteName,
			"remote_url": redactRemoteURL(cfg.RemoteURL),
			"repo":       cfg.RemoteRepo,
		}
	}
	auth := map[string]any{"authenticated": false}
	if cfg.Token != "" {
		user, err := client.Me(ctx)
		if err != nil {
			auth["error"] = errorMessage(err)
		} else {
			auth["authenticated"] = true
			auth["user"] = userSummary(user)
		}
	}
	report["auth"] = auth
	if ref != nil {
		report["repo"] = ref
		repo, err := client.RepoGet(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
		if err != nil {
			report["repo_error"] = errorMessage(err)
		} else {
			report["repository"] = map[string]any{
				"full_name":         repo.FullName,
				"html_url":          repo.HTMLURL,
				"default_branch":    repo.DefaultBranch,
				"private":           repo.Private,
				"archived":          repo.Archived,
				"open_issues_count": repo.OpenIssues,
				"open_pr_count":     repo.OpenPulls,
				"topics":            repo.Topics,
			}
			releases, err := client.RepoListReleases(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{Query: url.Values{"limit": {"5"}}})
			if err != nil {
				report["releases_error"] = errorMessage(err)
			} else {
				var out []doctorRelease
				for _, release := range releases {
					if release == nil {
						continue
					}
					out = append(out, doctorRelease{
						ID:      release.ID,
						TagName: release.TagName,
						Name:    release.Title,
						HTMLURL: release.HTMLURL,
						Draft:   release.IsDraft,
					})
				}
				report["recent_releases"] = out
				if len(out) > 0 {
					report["latest_release"] = out[0]
				}
			}
		}
	}
	return writeJSON(stdout, report)
}

func runSkill(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo skill <install|status>")
	}
	switch args[0] {
	case "install":
		return runSkillInstall(args[1:], stdout, stderr)
	case "status":
		return runSkillStatus(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown skill command %q", args[0])
	}
}

func runInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	skills := fs.Bool("skills", false, "install embedded Codex skill")
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	force := fs.Bool("force", false, "overwrite existing embedded skill files")
	check := fs.Bool("check", false, "check embedded skill install status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*skills || fs.NArg() != 0 {
		return errors.New("usage: fjgo install --skills [--dir path] [--force|--check]")
	}
	if *check {
		return writeJSON(stdout, fjgoskill.Check(*dir))
	}
	return installSkill(*dir, *force, stdout)
}

func runSkillInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo skill install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	force := fs.Bool("force", false, "overwrite existing embedded skill files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: fjgo skill install [--dir path] [--force]")
	}
	return installSkill(*dir, *force, stdout)
}

func runSkillStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo skill status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: fjgo skill status [--dir path]")
	}
	return writeJSON(stdout, fjgoskill.Check(*dir))
}

func installSkill(dir string, force bool, stdout io.Writer) error {
	result, err := fjgoskill.Install(dir, force)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

func repoFromArgs(cfg runConfig, args []string) (repoRef, []string, error) {
	if len(args) > 0 && strings.Contains(args[0], "/") {
		owner, repo, err := splitRepo(args[0])
		return repoRef{Owner: owner, Repo: repo}, args[1:], err
	}
	if cfg.RemoteRepo != nil {
		return *cfg.RemoteRepo, args, nil
	}
	return repoRef{}, args, errors.New("missing owner/repo")
}

func runAliasInfo(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo alias <list|inspect>")
	}
	args, jsonOut := takeJSONFlag(args)
	switch args[0] {
	case "list":
		if jsonOut {
			return writeJSON(stdout, forgejo.Aliases())
		}
		for _, alias := range forgejo.Aliases() {
			op, _ := forgejo.OperationByID(alias.Operation)
			fmt.Fprintf(stdout, "%-45s %-30s %s %s\n", strings.Join(alias.Command, " "), alias.Operation, op.Method, op.Path)
		}
		return nil
	case "collisions":
		if jsonOut {
			return writeJSON(stdout, forgejo.AliasCollisions())
		}
		for _, collision := range forgejo.AliasCollisions() {
			fmt.Fprintf(stdout, "%-45s kept=%s skipped=%s reason=%s\n", strings.Join(collision.Command, " "), collision.Kept, collision.Skipped, collision.Reason)
		}
		return nil
	case "inspect":
		if len(args) < 2 {
			return errors.New("usage: fjgo alias inspect <command...>")
		}
		alias, ok := aliasByCommand(args[1:])
		if !ok {
			return fmt.Errorf("unknown alias %q", strings.Join(args[1:], " "))
		}
		if jsonOut {
			return writeJSON(stdout, aliasView(alias))
		}
		writeAlias(stdout, alias)
		return nil
	default:
		return fmt.Errorf("unknown alias command %q", args[0])
	}
}

func runModel(args []string, stdout io.Writer) error {
	args, jsonOut := takeJSONFlag(args)
	if len(args) != 2 || args[0] != "inspect" {
		return errors.New("usage: fjgo model inspect <Model>")
	}
	model, ok := forgejo.ModelByName(strings.TrimPrefix(args[1], "*"))
	if !ok {
		return fmt.Errorf("unknown model %q", args[1])
	}
	if jsonOut {
		return writeJSON(stdout, model)
	}
	fmt.Fprintf(stdout, "model: %s\n", model.Name)
	for _, field := range model.Fields {
		writeModelField(stdout, field, "")
	}
	return nil
}

func runAlias(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	alias, rest, ok := matchAlias(args)
	if !ok {
		return fmt.Errorf("unknown command %q", args[0])
	}
	rest, body, err := takeBodyFlag(rest)
	if err != nil {
		return err
	}
	rest, yes := takeYesFlag(rest)
	rest, dryRun := takeDryRunFlag(rest)
	if alias.Unsafe && !yes {
		return fmt.Errorf("%s requires --yes", strings.Join(alias.Command, " "))
	}
	pathValues := map[string]string{}
	argOffset := 0
	if len(alias.Args) > 0 && alias.Args[0] == "owner/repo" && cfg.RemoteRepo != nil && (len(rest) < len(alias.Args) || !strings.Contains(rest[0], "/")) {
		pathValues["owner"] = cfg.RemoteRepo.Owner
		pathValues["repo"] = cfg.RemoteRepo.Repo
		argOffset = 1
	}
	if len(rest)+argOffset < len(alias.Args) {
		return fmt.Errorf("usage: fjgo %s %s", strings.Join(alias.Command, " "), strings.Join(alias.Args, " "))
	}
	for i := argOffset; i < len(alias.Args); i++ {
		name := alias.Args[i]
		value := rest[i-argOffset]
		if name == "owner/repo" {
			owner, repo, err := splitRepo(value)
			if err != nil {
				return err
			}
			pathValues["owner"] = owner
			pathValues["repo"] = repo
			continue
		}
		pathValues[name] = value
	}
	op, ok := forgejo.OperationByID(alias.Operation)
	if !ok {
		return fmt.Errorf("unknown operation %q", alias.Operation)
	}
	_, query, err := splitArgs(op, rest[len(alias.Args)-argOffset:])
	if err != nil {
		return err
	}
	var bodyValue any
	if body != "" {
		bodyValue, err = readJSONBody(body)
		if err != nil {
			return err
		}
	} else if op.BodyType != "" {
		return missingBodyError(op)
	}
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Body:        bodyValue,
			AuthPresent: clientHasAuth(client),
			RequiresYes: alias.Unsafe,
			YesProvided: yes,
		})
	}
	out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{
		Query: query,
		Body:  bodyValue,
	})
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return nil
	}
	_, err = stdout.Write(out)
	return err
}

func writeAlias(w io.Writer, alias forgejo.Alias) {
	op, _ := forgejo.OperationByID(alias.Operation)
	fmt.Fprintf(w, "command: %s\n", strings.Join(alias.Command, " "))
	fmt.Fprintf(w, "operation: %s\n", alias.Operation)
	fmt.Fprintf(w, "method: %s\n", op.Method)
	fmt.Fprintf(w, "path: %s\n", op.Path)
	if len(alias.Args) != 0 {
		fmt.Fprintf(w, "args: %s\n", strings.Join(alias.Args, ", "))
	}
	writeOperationParams(w, "query_params", op.QueryParams)
	if op.BodyType != "" {
		fmt.Fprintf(w, "body: %s\n", op.BodyType)
		writeModelFields(w, "body_fields", op.BodyType)
	}
	writeOperationParams(w, "form_params", op.FormParams)
	if op.ReturnType != "" {
		fmt.Fprintf(w, "returns: %s\n", op.ReturnType)
	}
	if op.Upload {
		fmt.Fprintln(w, "upload: multipart/form-data")
	}
	if alias.Unsafe {
		fmt.Fprintln(w, "requires: --yes")
	}
}

func aliasView(alias forgejo.Alias) aliasInspect {
	op, _ := forgejo.OperationByID(alias.Operation)
	view := aliasInspect{
		Command:       alias.Command,
		Operation:     alias.Operation,
		Args:          alias.Args,
		OperationInfo: operationView(op),
	}
	if alias.Unsafe {
		view.Requires = []string{"--yes"}
	}
	return view
}

func splitRepo(full string) (string, string, error) {
	owner, repo, ok := strings.Cut(full, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("expected owner/repo, got %q", full)
	}
	return owner, repo, nil
}

func resolveRemoteRepo(remote, baseURL string) (*repoRef, string, error) {
	out, err := exec.Command("git", "remote", "get-url", remote).Output()
	if err != nil {
		return nil, "", fmt.Errorf("resolve git remote %q: %w", remote, err)
	}
	remoteURL := strings.TrimSpace(string(out))
	ref, err := parseRemoteRepo(remoteURL, baseURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse git remote %q: %w", remote, err)
	}
	return ref, remoteURL, nil
}

func parseRemoteRepo(remoteURL, baseURL string) (*repoRef, error) {
	base, _ := url.Parse(baseURL)
	host := ""
	if base != nil {
		host = base.Host
	}
	var pathPart string
	if u, err := url.Parse(remoteURL); err == nil && u.Scheme != "" {
		if host != "" && u.Host != host {
			return nil, fmt.Errorf("remote host %q does not match base host %q", u.Host, host)
		}
		pathPart = strings.TrimPrefix(u.Path, "/")
	} else if before, after, ok := strings.Cut(remoteURL, ":"); ok && strings.Contains(before, "@") {
		remoteHost := strings.TrimPrefix(before[strings.LastIndex(before, "@")+1:], "[")
		remoteHost = strings.TrimSuffix(remoteHost, "]")
		if host != "" && remoteHost != host {
			return nil, fmt.Errorf("remote host %q does not match base host %q", remoteHost, host)
		}
		pathPart = after
	} else {
		return nil, fmt.Errorf("unsupported remote URL %q", remoteURL)
	}
	pathPart = strings.TrimSuffix(pathPart, ".git")
	parts := strings.Split(pathPart, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("remote URL does not contain owner/repo")
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("remote URL does not contain owner/repo")
	}
	return &repoRef{Owner: owner, Repo: repo}, nil
}

func runAuth(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "status") {
		return errors.New("usage: fjgo auth status")
	}
	status := map[string]any{
		"base_url":      cfg.BaseURL,
		"token_present": cfg.Token != "",
	}
	if cfg.RemoteRepo != nil {
		status["repo"] = cfg.RemoteRepo
	}
	if cfg.Token != "" {
		user, err := client.Me(ctx)
		if err != nil {
			status["authenticated"] = false
			status["error"] = errorMessage(err)
			return writeJSON(stdout, status)
		}
		status["authenticated"] = true
		status["user"] = userSummary(user)
	}
	return writeJSON(stdout, status)
}

func userSummary(user forgejo.User) map[string]any {
	return map[string]any{
		"id":       user.ID,
		"login":    user.UserName,
		"html_url": user.HTMLURL,
		"is_admin": user.IsAdmin,
	}
}

func takeSetFlag(args []string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != "--set" {
			out = append(out, args[i])
			continue
		}
		if i+1 == len(args) {
			return nil, "", errors.New("--set requires a value")
		}
		return append(out, args[i+2:]...), args[i+1], nil
	}
	return out, "", nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func missingPathParams(op forgejo.Operation, values map[string]string) []string {
	var missing []string
	for _, name := range op.PathParams {
		if _, ok := values[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func callOperationRaw(ctx context.Context, client *forgejo.Client, id string, pathValues map[string]string, query url.Values) ([]byte, error) {
	op, ok := forgejo.OperationByID(id)
	if !ok {
		return nil, fmt.Errorf("unknown operation %q", id)
	}
	return client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Query: query})
}

func takeBodyFlag(args []string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] != "-body" {
			out = append(out, args[i])
			continue
		}
		if i+1 == len(args) {
			return nil, "", errors.New("-body requires a value")
		}
		return append(out, args[i+2:]...), args[i+1], nil
	}
	return out, "", nil
}

func takeYesFlag(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	yes := false
	for _, arg := range args {
		if arg == "--yes" {
			yes = true
			continue
		}
		out = append(out, arg)
	}
	return out, yes
}

func takeJSONFlag(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	jsonOut := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOut = true
			continue
		}
		out = append(out, arg)
	}
	return out, jsonOut
}

func takeDryRunFlag(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	dryRun := false
	for _, arg := range args {
		if arg == "--dry-run" || arg == "--print-request" {
			dryRun = true
			continue
		}
		out = append(out, arg)
	}
	return out, dryRun
}

func matchAlias(args []string) (forgejo.Alias, []string, bool) {
	for _, alias := range forgejo.Aliases() {
		if len(args) < len(alias.Command) {
			continue
		}
		if slices.Equal(args[:len(alias.Command)], alias.Command) {
			return alias, args[len(alias.Command):], true
		}
	}
	return forgejo.Alias{}, nil, false
}

func aliasByCommand(command []string) (forgejo.Alias, bool) {
	for _, alias := range forgejo.Aliases() {
		if slices.Equal(alias.Command, command) {
			return alias, true
		}
	}
	return forgejo.Alias{}, false
}

func splitArgs(op forgejo.Operation, args []string) (map[string]string, url.Values, error) {
	pathValues := map[string]string{}
	query := url.Values{}
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return nil, nil, fmt.Errorf("expected name=value, got %q", arg)
		}
		if slices.Contains(op.PathParams, name) {
			pathValues[name] = value
		} else {
			query.Add(name, value)
		}
	}
	return pathValues, query, nil
}

func splitUploadArgs(op forgejo.Operation, args []string) (map[string]string, url.Values, map[string]string, []forgejo.UploadPart, error) {
	pathValues := map[string]string{}
	query := url.Values{}
	fields := map[string]string{}
	var files []forgejo.UploadPart
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return nil, nil, nil, nil, fmt.Errorf("expected name=value, got %q", arg)
		}
		switch {
		case slices.Contains(op.PathParams, name):
			pathValues[name] = value
		case isFormParam(op, name):
			if strings.HasPrefix(value, "@") {
				files = append(files, forgejo.UploadPart{FieldName: name, FilePath: strings.TrimPrefix(value, "@")})
			} else {
				fields[name] = value
			}
		default:
			query.Add(name, value)
		}
	}
	return pathValues, query, fields, files, nil
}

func isFormParam(op forgejo.Operation, name string) bool {
	for _, p := range op.FormParams {
		if p.Name == name {
			return true
		}
	}
	return false
}

func previewPath(op forgejo.Operation, pathValues map[string]string) string {
	apiPath := op.Path
	for _, name := range op.PathParams {
		if value, ok := pathValues[name]; ok {
			apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
		}
	}
	return apiPath
}

func writeRequestPreview(w io.Writer, preview requestPreview) error {
	return writeJSON(w, preview)
}

func clientHasAuth(client *forgejo.Client) bool {
	return client.HasToken()
}

func readJSONBody(value string) (any, error) {
	var b []byte
	var err error
	switch {
	case value == "-":
		b, err = io.ReadAll(os.Stdin)
	case strings.HasPrefix(value, "@"):
		b, err = os.ReadFile(strings.TrimPrefix(value, "@"))
	default:
		b = []byte(value)
	}
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func missingBodyError(op forgejo.Operation) error {
	return fmt.Errorf("%s requires -body %s; run `fjgo api inspect %s`", op.ID, op.BodyType, op.ID)
}

func writeModelFields(w io.Writer, label, name string) {
	model, ok := forgejo.ModelByName(strings.TrimPrefix(name, "*"))
	if !ok || len(model.Fields) == 0 {
		return
	}
	fmt.Fprintf(w, "%s:\n", label)
	for _, field := range model.Fields {
		writeModelField(w, field, "  ")
	}
}

func writeModelField(w io.Writer, field forgejo.ModelField, prefix string) {
	required := ""
	if field.Required {
		required = " required"
	}
	fmt.Fprintf(w, "%s%s: %s%s\n", prefix, field.Name, field.Type, required)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func redactRemoteURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = url.User("redacted")
	return u.String()
}

func jsonErrorRequested(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return false
		}
		if arg == "--json" || arg == "-json" {
			return true
		}
		if !strings.HasPrefix(arg, "-") {
			return false
		}
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		} else if rootFlagTakesValue(name) {
			i++
		}
	}
	return false
}

func rootFlagTakesValue(name string) bool {
	switch name {
	case "base-url", "token", "timeout", "R", "repo-from-remote":
		return true
	default:
		return false
	}
}

func errorView(args []string, err error) map[string]any {
	out := map[string]any{
		"error":   errorMessage(err),
		"command": redactArgs(args),
	}
	var httpErr forgejo.HTTPError
	if errors.As(err, &httpErr) {
		out["status"] = httpErr.StatusCode
		out["kind"] = "forgejo_api"
	} else {
		out["kind"] = "cli"
	}
	return out
}

func redactArgs(args []string) []string {
	out := slices.Clone(args)
	for i, arg := range out {
		if arg == "-token" || arg == "--token" {
			if i+1 < len(out) {
				out[i+1] = "redacted"
			}
			continue
		}
		if strings.HasPrefix(arg, "-token=") {
			out[i] = "-token=redacted"
		}
		if strings.HasPrefix(arg, "--token=") {
			out[i] = "--token=redacted"
		}
	}
	return out
}

func errorMessage(err error) string {
	var httpErr forgejo.HTTPError
	if errors.As(err, &httpErr) {
		var payload struct {
			Message string `json:"message"`
			URL     string `json:"url"`
		}
		if json.Unmarshal([]byte(httpErr.Body), &payload) == nil && payload.Message != "" {
			if payload.URL != "" {
				return fmt.Sprintf("forgejo api: status %d: %s (%s)", httpErr.StatusCode, payload.Message, payload.URL)
			}
			return fmt.Sprintf("forgejo api: status %d: %s", httpErr.StatusCode, payload.Message)
		}
	}
	return err.Error()
}

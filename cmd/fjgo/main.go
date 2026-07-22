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
	version = "v1.2.0"
	commit  = "none"
	date    = "unknown"
)

type repoRef struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type runConfig struct {
	BaseURL    string
	Host       string
	Token      string
	BasicAuth  bool
	OTP        bool
	Sudo       bool
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
		code := 1
		if isUsage(err) {
			code = 2
		}
		if jsonErrorRequested(args) {
			_ = writeJSON(stdout, errorView(args, err))
		} else {
			_ = writeTOON(stdout, errorView(args, err))
		}
		return code
	}
	return 0
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	args, contextFlags, err := peelContextFlags(args)
	if err != nil {
		return err
	}
	if err := rejectUnknownRootFlags(args); err != nil {
		return err
	}
	fs := flag.NewFlagSet("fjgo", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	envHost := os.Getenv("FJGO_HOST")
	baseURL := fs.String("base-url", defaultBaseURL, "Forgejo API base URL")
	host := fs.String("host", envHost, "Forgejo host; derives https://host/api/v1")
	jsonErrors := fs.Bool("json", false, "print errors as JSON when used before the command")
	token := fs.String("token", "", "Forgejo access token (or FJGO_TOKEN)")
	username := fs.String("username", os.Getenv("FJGO_USERNAME"), "Basic Auth username (or FJGO_USERNAME)")
	password := fs.String("password", os.Getenv("FJGO_PASSWORD"), "Basic Auth password (or FJGO_PASSWORD)")
	otp := fs.String("otp", os.Getenv("FJGO_OTP"), "Basic Auth TOTP code (or FJGO_OTP)")
	sudo := fs.String("sudo", os.Getenv("FJGO_SUDO"), "act as another user (or FJGO_SUDO)")
	timeout := fs.Duration("timeout", 15*time.Second, "per-request HTTP timeout")
	showVersion := fs.Bool("version", false, "print fjgo version")
	repoContext := fs.String("repo", getenv("FJGO_REPO", ""), "explicit Forgejo owner/repo context (or FJGO_REPO)")
	remote := fs.String("R", "", "resolve owner/repo from git remote")
	remoteLong := fs.String("repo-from-remote", "", "resolve owner/repo from git remote")
	fs.Usage = func() {
		_ = writeHelp(stdout, rootHelp())
	}
	if err := fs.Parse(args); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return newUsageError(errorMessage(err), "Run `fjgo --help` for valid global flags")
	}
	if *showVersion {
		return writeTOON(stdout, map[string]any{
			"version": version,
			"commit":  commit,
			"date":    date,
		})
	}
	_ = *jsonErrors
	baseURLConfigured := false
	hostConfigured := envHost != ""
	hostFlagConfigured := false
	authCredentialFlagConfigured := false
	sudoFlagConfigured := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "base-url" {
			baseURLConfigured = true
		}
		if f.Name == "host" {
			hostConfigured = true
			hostFlagConfigured = true
		}
		if f.Name == "username" || f.Name == "password" || f.Name == "otp" {
			authCredentialFlagConfigured = true
		}
		if f.Name == "sudo" {
			sudoFlagConfigured = true
		}
	})
	if contextFlags.Host != "" {
		*host = contextFlags.Host
		hostConfigured = true
		hostFlagConfigured = true
	}
	if *host != "" && (!baseURLConfigured || hostFlagConfigured) {
		derived, err := apiBaseURLFromHost(*host)
		if err != nil {
			return err
		}
		*baseURL = derived
	}
	if *token == "" && (*baseURL != defaultBaseURL || baseURLConfigured) {
		*token = os.Getenv("FJGO_TOKEN")
	}
	if *token == "" && hostConfigured {
		*token = os.Getenv("FJGO_TOKEN")
	}
	if *baseURL == defaultBaseURL && !baseURLConfigured && !hostConfigured {
		if !authCredentialFlagConfigured {
			*username, *password, *otp = "", "", ""
		}
		if !sudoFlagConfigured {
			*sudo = ""
		}
	}
	if *remoteLong != "" {
		*remote = *remoteLong
	}
	if contextFlags.Remote != "" {
		*remote = contextFlags.Remote
	}
	if contextFlags.Repo != "" {
		*repoContext = contextFlags.Repo
	}

	httpClient := *http.DefaultClient
	httpClient.Timeout = *timeout
	client, err := forgejo.NewClientWithAuth(*baseURL, forgejo.AuthConfig{
		Token: *token, Username: *username, Password: *password, OTP: *otp, Sudo: *sudo,
	}, &httpClient)
	if err != nil {
		return err
	}
	cfg := runConfig{BaseURL: *baseURL, Host: *host, Token: *token, BasicAuth: *username != "", OTP: *otp != "", Sudo: *sudo != ""}
	repoContextSource := ""
	if os.Getenv("FJGO_REPO") != "" {
		repoContextSource = "FJGO_REPO"
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "repo" {
			repoContextSource = "--repo"
		}
	})
	if contextFlags.Repo != "" {
		repoContextSource = contextFlags.RepoSource
	}
	if *repoContext != "" {
		owner, repo, err := splitRepo(*repoContext)
		if err != nil {
			return err
		}
		cfg.RemoteRepo = &repoRef{Owner: owner, Repo: repo}
		cfg.RemoteName = repoContextSource
	} else if *remote != "" {
		ref, remoteURL, err := resolveRemoteRepo(*remote, *baseURL)
		if err != nil {
			return err
		}
		cfg.RemoteRepo = ref
		cfg.RemoteName = *remote
		cfg.RemoteURL = remoteURL
	}
	if fs.NArg() == 0 {
		return writeHome(ctx, client, cfg, stdout)
	}

	switch fs.Arg(0) {
	case "setup":
		return runSetup(fs.Args()[1:], stdout)
	case "hook":
		return runHook(fs.Args()[1:], stdout)
	case "doctor", "context":
		return runDoctor(ctx, client, cfg, fs.Args()[1:], stdout)
	case "version":
		if hasHelp(fs.Args()[1:]) {
			return writeHelp(stdout, "usage: fjgo version\nShow the Forgejo server version.\n\nexamples:\n  fjgo version\n  fjgo --host forgejo.example.com version")
		}
		if err := rejectUnknownFlags(fs.Args()[1:], "version", nil, nil); err != nil {
			return err
		}
		if len(fs.Args()) != 1 {
			return newUsageError("usage: fjgo version")
		}
		body, err := callOperationRaw(ctx, client, "getVersion", nil, nil)
		if err != nil {
			return err
		}
		return writeJSONAsTOON(stdout, body, "version", false)
	case "me":
		if hasHelp(fs.Args()[1:]) {
			return writeHelp(stdout, "usage: fjgo me\nShow the authenticated Forgejo user summary.\n\nexamples:\n  fjgo me\n  fjgo --host forgejo.example.com me")
		}
		if err := rejectUnknownFlags(fs.Args()[1:], "me", nil, nil); err != nil {
			return err
		}
		if len(fs.Args()) != 1 {
			return newUsageError("usage: fjgo me")
		}
		user, err := client.Me(ctx)
		if err != nil {
			return err
		}
		return writeTOON(stdout, userSummary(user))
	case "auth", "whoami":
		return runAuth(ctx, client, cfg, fs.Args()[1:], stdout)
	case "get":
		cmdArgs := fs.Args()[1:]
		if hasHelp(cmdArgs) {
			return writeHelp(stdout, "usage: fjgo get <path> [--json] [--full]\nexamples:\n  fjgo get /version\n  fjgo get /repos/OWNER/REPO --json")
		}
		if err := rejectUnknownFlags(cmdArgs, "get", []string{"--json", "--full"}, nil); err != nil {
			return err
		}
		cmdArgs, jsonOut := takeJSONFlag(cmdArgs)
		cmdArgs, full := takeFullFlag(cmdArgs)
		if len(cmdArgs) != 1 {
			return newUsageError("usage: fjgo get <path>", "Run `fjgo get /version`")
		}
		body, err := client.GetRaw(ctx, cmdArgs[0])
		if err != nil {
			return err
		}
		if len(body) == 0 {
			return writeAPIOK(stdout, jsonOut)
		}
		return writeAPIBody(stdout, body, "", jsonOut, full)
	case "api":
		return runAPI(ctx, client, cfg, fs.Args()[1:], stdout, stderr)
	case "alias":
		return runAliasInfo(fs.Args()[1:], stdout)
	case "model":
		return runModel(fs.Args()[1:], stdout)
	case "repo":
		return runRepo(ctx, client, cfg, fs.Args()[1:], stdout)
	case "issue":
		return runIssue(ctx, client, cfg, fs.Args()[1:], stdout)
	case "pr", "pull":
		return runPR(ctx, client, cfg, fs.Args()[1:], stdout)
	case "run":
		return runActions(ctx, client, cfg, fs.Args()[1:], stdout)
	case "workflow":
		return runWorkflow(ctx, client, cfg, fs.Args()[1:], stdout)
	case "search":
		return runSearch(ctx, client, cfg, fs.Args()[1:], stdout)
	case "label":
		return runLabel(ctx, client, cfg, fs.Args()[1:], stdout)
	case "secret":
		return runSecret(ctx, client, cfg, fs.Args()[1:], stdout)
	case "variable":
		return runVariable(ctx, client, cfg, fs.Args()[1:], stdout)
	case "release":
		return runRelease(ctx, client, cfg, fs.Args()[1:], stdout)
	case "update":
		return runUpdate(ctx, &httpClient, fs.Args()[1:], stdout)
	case "skill":
		return runSkill(fs.Args()[1:], stdout, stderr)
	case "install":
		return runInstall(fs.Args()[1:], stdout, stderr)
	default:
		return runAlias(ctx, client, cfg, fs.Args(), stdout)
	}
}

func runAPI(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout, stderr io.Writer) error {
	if help, ok := subcommandHelp(args, apiHelp(), apiCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	args, jsonOut := takeJSONFlag(args)
	switch args[0] {
	case "list":
		if err := rejectUnknownFlags(args[1:], "api list", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
			return err
		}
		var err error
		args, fields, err := takeFieldsFlag(args)
		if err != nil {
			return err
		}
		fields, err = validatedFields(fields, []string{"id", "method", "path"}, []string{"id", "method", "path", "summary", "description", "returns", "body", "upload", "tags", "deprecated"}, "api list")
		if err != nil {
			return err
		}
		filter := ""
		if len(args) > 1 {
			filter = strings.ToLower(args[1])
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
	case "inspect":
		if err := rejectUnknownFlags(args[1:], "api inspect", []string{"--json"}, nil); err != nil {
			return err
		}
		if len(args) != 2 {
			return newUsageError("usage: fjgo api inspect <operationId>", "Run `fjgo api list <filter>` to find operation IDs")
		}
		op, ok := forgejo.OperationByID(args[1])
		if !ok {
			return fmt.Errorf("unknown operation %q", args[1])
		}
		if jsonOut {
			return writeJSON(stdout, operationView(op))
		}
		return writeTOON(stdout, operationBlocks(op))
	case "call":
		callArgs, full := takeFullFlag(args[1:])
		return callOperation(ctx, client, cfg, callArgs, stdout, stderr, jsonOut, full)
	case "upload":
		uploadArgs, full := takeFullFlag(args[1:])
		return uploadOperation(ctx, client, cfg, uploadArgs, stdout, jsonOut, full)
	case "raw", "request":
		rawArgs, full := takeFullFlag(args[1:])
		return rawRequest(ctx, client, rawArgs, stdout, jsonOut, full)
	default:
		return unknownSubcommandError("api", args[0], []string{"list", "inspect", "call", "upload", "raw"})
	}
}

func apiCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":    {Usage: "fjgo api list [filter] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo api list repo", "fjgo api list release --fields id,method,path"}},
		"inspect": {Usage: "fjgo api inspect <operationId> [--json]", Examples: []string{"fjgo api inspect createCurrentUserRepo", "fjgo api inspect repoSearch --json"}},
		"call":    {Usage: "fjgo api call <operationId> [name=value ...] [flags]", Flags: []string{"-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>, --accept <type>", "--output <path>, --raw", "--include-response", "--yes, --dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api call repoSearch q=forgejo", "fjgo api call getVersion --include-response", "fjgo api call repoGetArchive owner=OWNER repo=REPO archive=main.zip --output repo.zip"}},
		"upload":  {Usage: "fjgo api upload <operationId> [name=value ...] attachment=@file [flags] --yes", Flags: []string{"--include-response", "--dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api upload repoCreateReleaseAttachment owner=OWNER repo=REPO id=1 attachment=@dist.tgz --dry-run --yes", "fjgo api upload issueCreateIssueAttachment owner=OWNER repo=REPO index=1 attachment=@file.txt --include-response --yes"}},
		"raw":     {Usage: "fjgo api raw <METHOD> <path> [name=value ...] [flags]", Flags: []string{"-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>, --accept <type>", "--output <path>, --raw", "--include-response", "--yes, --dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api raw GET /version --include-response", "fjgo api raw POST /markdown/raw -body-raw @README.md --content-type text/plain --yes", "fjgo api raw GET /repos/OWNER/REPO/archive/main.zip --output repo.zip"}},
		"request": {Usage: "fjgo api request <METHOD> <path> [name=value ...] [flags]", Flags: []string{"-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>, --accept <type>", "--output <path>, --raw", "--include-response", "--yes, --dry-run, --print-request", "--full, --json"}, Examples: []string{"fjgo api request GET /version --include-response", "fjgo api request POST /markdown/raw -body-raw @README.md --content-type text/plain --yes", "fjgo api request GET /repos/OWNER/REPO/archive/main.zip --output repo.zip"}},
	}
}

func writeOperation(w io.Writer, op forgejo.Operation) {
	fmt.Fprintf(w, "id: %s\n", op.ID)
	fmt.Fprintf(w, "method: %s\n", op.Method)
	fmt.Fprintf(w, "path: %s\n", op.Path)
	if op.Summary != "" {
		fmt.Fprintf(w, "summary: %s\n", op.Summary)
	}
	if op.Description != "" {
		fmt.Fprintf(w, "description: %s\n", op.Description)
	}
	if len(op.PathParams) != 0 {
		writeOperationParams(w, "path_params", op.PathParamInfo)
	}
	writeOperationParams(w, "query_params", op.QueryParams)
	if op.BodyType != "" {
		fmt.Fprintf(w, "body: %s\n", op.BodyType)
		if op.BodyParam != nil {
			writeOperationParams(w, "body_param", []forgejo.OperationParam{*op.BodyParam})
		}
		writeModelFields(w, "body_fields", op.BodyType)
	}
	writeOperationParams(w, "form_params", op.FormParams)
	if op.ReturnType != "" {
		fmt.Fprintf(w, "returns: %s\n", op.ReturnType)
	}
	if op.Upload {
		fmt.Fprintln(w, "upload: multipart/form-data")
	}
	if len(op.Consumes) != 0 {
		fmt.Fprintf(w, "consumes: %s\n", strings.Join(op.Consumes, ", "))
	}
	if len(op.Produces) != 0 {
		fmt.Fprintf(w, "produces: %s\n", strings.Join(op.Produces, ", "))
	}
	if len(op.Tags) != 0 {
		fmt.Fprintf(w, "tags: %s\n", strings.Join(op.Tags, ", "))
	}
	if op.Deprecated {
		fmt.Fprintln(w, "deprecated: true")
	}
	for _, response := range op.Responses {
		headerNames := make([]string, 0, len(response.Headers))
		for _, header := range response.Headers {
			headerNames = append(headerNames, header.Name)
		}
		fmt.Fprintf(w, "response %s: %s headers=%s - %s\n", response.Code, response.Type, strings.Join(headerNames, ","), response.Description)
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
	ContentType string               `json:"content_type,omitempty"`
	Accept      string               `json:"accept,omitempty"`
	Output      string               `json:"output,omitempty"`
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
		extra := ""
		if len(p.Enum) != 0 {
			extra += " enum=" + strings.Join(p.Enum, "|")
		}
		if p.HasDefault {
			extra += " default=" + fmt.Sprint(p.Default)
		}
		if p.CollectionFormat != "" {
			extra += " collection=" + p.CollectionFormat
		}
		if p.HasMinimum {
			extra += " minimum=" + strconv.FormatFloat(p.Minimum, 'g', -1, 64)
		}
		fmt.Fprintf(w, "  %s: %s%s%s%s\n", p.Name, p.Type, required, extra, desc)
	}
}

func callOperation(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout, stderr io.Writer, jsonOut, full bool) error {
	if hasHelp(args) {
		return writeHelp(stdout, apiHelp())
	}
	if err := rejectUnknownFlags(args, "api call", []string{"-body", "-body-raw", "--content-type", "--accept", "--output", "--raw", "--include-response", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"-body", "-body-raw", "--content-type", "--accept", "--output"}); err != nil {
		return err
	}
	args, input, err := takeAPIBodyInput(args)
	if err != nil {
		return err
	}
	args, output, err := takeAPIOutput(args)
	if err != nil {
		return err
	}
	if output != "" && jsonOut {
		return newUsageError("use --output or --raw instead of --json for raw response bytes")
	}
	args, includeResponse := takeIncludeResponse(args)
	if output != "" && includeResponse {
		return newUsageError("use --include-response only with buffered output; --output and --raw already report transfer status")
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	if len(args) == 0 {
		return newUsageError("usage: fjgo api call <operationId> [name=value ...]", "Run `fjgo api inspect <operationId>` before calling unfamiliar operations")
	}
	op, ok := forgejo.OperationByID(args[0])
	if !ok {
		return fmt.Errorf("unknown operation %q", args[0])
	}
	if op.Upload && (input.Raw == nil || operationNonMultipartContentType(op) == "") {
		return fmt.Errorf("%s uses multipart/form-data upload; use `fjgo api upload %s ... attachment=@file --yes`", op.ID, op.ID)
	}
	if op.Method != http.MethodGet && !yes {
		return fmt.Errorf("%s requires --yes", op.ID)
	}
	pathValues, query, err := splitArgs(op, args[1:])
	if err != nil {
		return err
	}
	fillRepoPathValues(pathValues, op, cfg)
	if missing := missingPathParams(op, pathValues); len(missing) != 0 {
		return fmt.Errorf("missing path parameter %q for %s; run `fjgo api inspect %s`", missing[0], op.ID, op.ID)
	}
	if err := validateOperationPath(op, pathValues); err != nil {
		return err
	}
	if err := validateOperationQuery(op, query); err != nil {
		return err
	}
	if input.JSON == nil && input.Raw == nil && op.BodyParam != nil && op.BodyParam.Required {
		return missingBodyError(op)
	}
	if err := validateOperationBody(op, input.JSON); err != nil {
		return err
	}
	if err := normalizeOperationBody(op, &input); err != nil {
		return err
	}
	if input.ContentType == "" {
		if input.Raw != nil && op.Upload {
			input.ContentType = operationNonMultipartContentType(op)
		} else if input.Raw != nil && len(op.Consumes) != 0 {
			input.ContentType = op.Consumes[0]
		} else if input.JSON != nil {
			input.ContentType = "application/json"
		}
	}
	if input.Accept == "" && len(op.Produces) != 0 {
		input.Accept = strings.Join(op.Produces, ", ")
	}
	previewBody := apiBodyPreview(input)
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Body:        previewBody,
			ContentType: input.ContentType,
			Accept:      input.Accept,
			Output:      output,
			AuthPresent: clientHasAuth(client),
			RequiresYes: op.Method != http.MethodGet,
			YesProvided: yes,
		})
	}
	opts := forgejo.RequestOptions{
		Query:       query,
		Body:        input.JSON,
		RawBody:     input.Raw,
		ContentType: input.ContentType,
		Accept:      input.Accept,
	}
	if output != "" {
		return streamAPIDestination(stdout, output, func(dst io.Writer) (forgejo.StreamResponse, error) {
			return client.DoOperationStream(ctx, op, pathValues, opts, dst)
		})
	}
	response, err := client.DoOperationRawResponse(ctx, op, pathValues, opts)
	if err != nil {
		return err
	}
	if includeResponse {
		return writeAPIResponse(stdout, response, strings.Join(op.Produces, ","), jsonOut, full)
	}
	if len(response.Body) == 0 {
		return writeAPIOK(stdout, jsonOut)
	}
	return writeAPIBody(stdout, response.Body, strings.Join(op.Produces, ","), jsonOut, full)
}

func operationNonMultipartContentType(op forgejo.Operation) string {
	for _, mediaType := range op.Consumes {
		if !strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
			return mediaType
		}
	}
	return ""
}

func uploadOperation(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if hasHelp(args) {
		return writeHelp(stdout, apiHelp())
	}
	if err := rejectUnknownFlags(args, "api upload", []string{"--include-response", "--yes", "--dry-run", "--print-request", "--json", "--full"}, nil); err != nil {
		return err
	}
	args, includeResponse := takeIncludeResponse(args)
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	if len(args) == 0 {
		return newUsageError("usage: fjgo api upload <operationId> [name=value ...] attachment=@file --yes", "Run `fjgo api inspect <operationId>` before uploading")
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
	fillRepoPathValues(pathValues, op, cfg)
	if missing := missingPathParams(op, pathValues); len(missing) != 0 {
		return fmt.Errorf("missing path parameter %q for %s; run `fjgo api inspect %s`", missing[0], op.ID, op.ID)
	}
	if err := validateOperationPath(op, pathValues); err != nil {
		return err
	}
	if err := validateOperationUpload(op, query, fields, files); err != nil {
		return err
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
	response, err := client.DoOperationMultipartResponse(ctx, op, pathValues, query, fields, files)
	if err != nil {
		return err
	}
	if includeResponse {
		return writeAPIResponse(stdout, response, strings.Join(op.Produces, ","), jsonOut, full)
	}
	if len(response.Body) == 0 {
		return writeAPIOK(stdout, jsonOut)
	}
	return writeAPIBody(stdout, response.Body, strings.Join(op.Produces, ","), jsonOut, full)
}

func rawRequest(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer, jsonOut, full bool) error {
	if hasHelp(args) {
		return writeHelp(stdout, apiHelp())
	}
	if err := rejectUnknownFlags(args, "api raw", []string{"-body", "-body-raw", "--content-type", "--accept", "--output", "--raw", "--include-response", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"-body", "-body-raw", "--content-type", "--accept", "--output"}); err != nil {
		return err
	}
	args, input, err := takeAPIBodyInput(args)
	if err != nil {
		return err
	}
	args, output, err := takeAPIOutput(args)
	if err != nil {
		return err
	}
	if output != "" && jsonOut {
		return newUsageError("use --output or --raw instead of --json for raw response bytes")
	}
	args, includeResponse := takeIncludeResponse(args)
	if output != "" && includeResponse {
		return newUsageError("use --include-response only with buffered output; --output and --raw already report transfer status")
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	if len(args) < 2 {
		return newUsageError("usage: fjgo api raw <METHOD> <path> [name=value ...]", "Run `fjgo api raw GET /version`")
	}
	method := strings.ToUpper(args[0])
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return newUsageError("api raw method must be GET, POST, PUT, PATCH, or DELETE")
	}
	apiPath := args[1]
	if !strings.HasPrefix(apiPath, "/") {
		return newUsageError("api raw path must start with /", "Run `fjgo api raw GET /repos/OWNER/REPO`")
	}
	if method != http.MethodGet && !yes {
		return newUsageError("api raw " + method + " requires --yes")
	}
	query, err := queryFromPairs(args[2:])
	if err != nil {
		return err
	}
	if input.Raw != nil && input.ContentType == "" {
		input.ContentType = "text/plain; charset=utf-8"
	}
	previewBody := apiBodyPreview(input)
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Method:      method,
			Path:        apiPath,
			Query:       query,
			Body:        previewBody,
			ContentType: input.ContentType,
			Accept:      input.Accept,
			Output:      output,
			AuthPresent: clientHasAuth(client),
			RequiresYes: method != http.MethodGet,
			YesProvided: yes,
		})
	}
	opts := forgejo.RequestOptions{Query: query, Body: input.JSON, RawBody: input.Raw, ContentType: input.ContentType, Accept: input.Accept}
	if output != "" {
		return streamAPIDestination(stdout, output, func(dst io.Writer) (forgejo.StreamResponse, error) {
			return client.DoRawStream(ctx, method, apiPath, opts, dst)
		})
	}
	resp, err := client.DoRawResponse(ctx, method, apiPath, opts)
	if err != nil {
		return err
	}
	if includeResponse {
		return writeAPIResponse(stdout, resp, "", jsonOut, full)
	}
	if len(resp.Body) == 0 {
		return writeAPIOK(stdout, jsonOut)
	}
	return writeAPIBody(stdout, resp.Body, "", jsonOut, full)
}

func runRepo(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, repoHelp(), repoCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	switch args[0] {
	case "list":
		return runRepoList(ctx, client, args[1:], stdout, jsonOut)
	case "create":
		return runRepoCreate(ctx, client, args[1:], stdout, jsonOut, full)
	case "get":
		if err := rejectUnknownFlags(args[1:], "repo get", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
			return err
		}
		restArgs, fieldsArg, err := takeFieldsFlag(args[1:])
		if err != nil {
			return err
		}
		fields, err := validatedFields(fieldsArg, repoViewDefaultFields, repoViewFields, "repo get")
		if err != nil {
			return err
		}
		ref, rest, err := repoFromArgs(cfg, restArgs)
		if err != nil || len(rest) != 0 {
			return newUsageError("usage: fjgo repo get [owner/repo]", "Pass `owner/repo`, root `--repo OWNER/REPO`, set FJGO_REPO=OWNER/REPO, or use `-R origin`")
		}
		out, err := client.RepoGet(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeRepository(stdout, out, jsonOut, fields)
	case "edit":
		return runRepoEdit(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "fork":
		return runRepoFork(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "branches":
		return runRepoBranches(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "collaborators":
		return runRepoCollaborators(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "branch-protection", "branch-protections":
		return runRepoBranchProtection(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "topics":
		ref, rest, err := repoFromArgs(cfg, args[1:])
		if err != nil {
			return newUsageError("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]", "Pass `owner/repo` or use `-R origin`")
		}
		if err := rejectUnknownFlags(rest, "repo topics", []string{"--set", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--set"}); err != nil {
			return err
		}
		args, yes := takeYesFlag(rest)
		args, dryRun := takeDryRunFlag(args)
		args, topics, err := takeSetFlag(args)
		if err != nil {
			return err
		}
		if len(args) != 0 {
			return newUsageError("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]")
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
		if out == nil {
			return writeTopics(stdout, ref, nil, jsonOut)
		}
		return writeTopics(stdout, ref, out.TopicNames, jsonOut)
	case "avatar":
		ref := repoRef{}
		rest := args[1:]
		if cfg.RemoteRepo != nil {
			ref = *cfg.RemoteRepo
		} else {
			var err error
			ref, rest, err = repoFromArgs(cfg, rest)
			if err != nil {
				return newUsageError("usage: fjgo repo avatar <owner/repo> <png> --yes", "Pass `owner/repo` or use `-R origin`")
			}
		}
		if err := rejectUnknownFlags(rest, "repo avatar", []string{"--yes", "--dry-run", "--print-request"}, nil); err != nil {
			return err
		}
		rest, yes := takeYesFlag(rest)
		rest, dryRun := takeDryRunFlag(rest)
		if len(rest) != 1 {
			return newUsageError("usage: fjgo repo avatar <owner/repo> <png> --yes")
		}
		if !yes {
			return newUsageError("repo avatar requires --yes")
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
		if err := client.RepoUpdateAvatar(ctx, ref.Owner, ref.Repo, &forgejo.UpdateRepoAvatarOption{Image: base64.StdEncoding.EncodeToString(b)}, forgejo.RequestOptions{}); err != nil {
			return err
		}
		return writeTOON(stdout, map[string]any{"avatar": "updated"})
	case "issue":
		return runRepoIssue(ctx, client, cfg, args[1:], stdout)
	default:
		aliasArgs := append([]string{"repo"}, args...)
		if jsonOut {
			aliasArgs = append(aliasArgs, "--json")
		}
		if full {
			aliasArgs = append(aliasArgs, "--full")
		}
		return runAlias(ctx, client, cfg, aliasArgs, stdout)
	}
}

func repoCommandHelps() map[string]commandHelpSpec {
	helps := map[string]commandHelpSpec{
		"list":               {Usage: "fjgo repo list [flags]", Flags: []string{"--user <user>, --org <org>, --q <text>", "--limit <n> (default " + defaultListLimit + "), --page <n>, --sort <key>, --private <bool>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo repo list --org OWNER --fields name,private,archived", "fjgo repo list --q \"forgejo cli\" --limit 20"}},
		"create":             {Usage: "fjgo repo create <name> [flags] --yes", Flags: []string{"--org <org>, --description <text>, --private, --auto-init", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo repo create demo --private --dry-run --yes", "fjgo repo create demo --org OWNER --yes"}},
		"get":                {Usage: "fjgo repo get [owner/repo] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo --repo OWNER/REPO repo get", "fjgo repo get OWNER/REPO --fields full_name,default_branch,open_issues"}},
		"edit":               {Usage: "fjgo repo edit [owner/repo] [flags] --yes", Flags: []string{"--description <text>", "--private <bool>, --archived <bool>, --has-issues <bool>, --has-pulls <bool>, --has-wiki <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO repo edit --description \"CLI\" --dry-run --yes", "fjgo repo edit OWNER/REPO --archived false --yes"}},
		"fork":               {Usage: "fjgo repo fork [owner/repo] [flags] --yes", Flags: []string{"--name <name>, --org <org>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO repo fork --dry-run --yes", "fjgo repo fork OWNER/REPO --name fork --yes"}},
		"branches":           {Usage: "fjgo repo branches <list|get|create|delete> [owner/repo] [flags]", Flags: []string{"--fields <a,b,c>, --json", "--name <branch>, --from <branch>", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo branches list", "fjgo --repo OWNER/REPO repo branches create --name feature --from main --dry-run --yes"}},
		"collaborators":      {Usage: "fjgo repo collaborators <list|check|permission|add|remove> [owner/repo] [flags]", Flags: []string{"--permission <read|write|admin>", "--fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo collaborators list", "fjgo --repo OWNER/REPO repo collaborators add alice --permission write --dry-run --yes"}},
		"branch-protection":  {Usage: "fjgo repo branch-protection <list|get|create|edit|delete> [owner/repo] [flags]", Flags: []string{"--name <branch>, --required-approvals <n>", "--fields <a,b,c>, --json, --full", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo branch-protection list", "fjgo --repo OWNER/REPO repo branch-protection create --name main --required-approvals 1 --dry-run --yes"}},
		"branch-protections": {Usage: "fjgo repo branch-protections <list|get|create|edit|delete> [owner/repo] [flags]", Flags: []string{"--name <branch>, --required-approvals <n>", "--fields <a,b,c>, --json, --full", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo branch-protections list", "fjgo --repo OWNER/REPO repo branch-protections create --name main --required-approvals 1 --dry-run --yes"}},
		"topics":             {Usage: "fjgo repo topics [owner/repo] [--set comma,separated,topics] [flags]", Flags: []string{"--set <topic,topic>", "--yes, --dry-run, --print-request, --json"}, Examples: []string{"fjgo repo topics OWNER/REPO", "fjgo repo topics OWNER/REPO --set forgejo,go --dry-run --yes"}},
		"avatar":             {Usage: "fjgo repo avatar [owner/repo] <png> --yes", Flags: []string{"--dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo avatar icon.png --dry-run --yes", "fjgo repo avatar OWNER/REPO icon.png --yes"}},
		"issue":              {Usage: "fjgo repo issue <close|comment> [owner/repo] <index> [flags]", Flags: []string{"-body <json|@file|->", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue close 42 --dry-run --yes", "fjgo -R origin repo issue comment 42 -body '{\"body\":\"note\"}' --dry-run --yes"}},
	}
	for command, spec := range repoNestedCommandHelps() {
		helps[command] = spec
	}
	return helps
}

func repoNestedCommandHelps() map[string]commandHelpSpec {
	branchDelete := commandHelpSpec{Usage: "fjgo repo branches delete [owner/repo] <branch> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branches delete feature --dry-run --yes", "fjgo repo branches delete OWNER/REPO feature --yes"}}
	collaboratorRemove := commandHelpSpec{Usage: "fjgo repo collaborators remove [owner/repo] <user> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo collaborators remove alice --dry-run --yes", "fjgo repo collaborators remove OWNER/REPO alice --yes"}}
	protectionMutate := func(action string) commandHelpSpec {
		return commandHelpSpec{Usage: "fjgo repo branch-protection " + action + " [owner/repo] <name> [flags] --yes", Flags: []string{"--required-approvals <n>, --status-check <context> (repeatable)", "--enable-status-check <bool>, --require-signed-commits <bool>, --enable-push <bool>, --apply-to-admins <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo -R origin repo branch-protection " + action + " main --required-approvals 1 --dry-run --yes", "fjgo repo branch-protection " + action + " OWNER/REPO main --require-signed-commits true --yes"}}
	}
	protectionDelete := commandHelpSpec{Usage: "fjgo repo branch-protection delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branch-protection delete main --dry-run --yes", "fjgo repo branch-protection delete OWNER/REPO main --yes"}}
	helps := map[string]commandHelpSpec{
		"branches list":            {Usage: "fjgo repo branches list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin repo branches list", "fjgo repo branches list OWNER/REPO --fields name,protected"}},
		"branches get":             {Usage: "fjgo repo branches get [owner/repo] <branch> [--json]", Examples: []string{"fjgo -R origin repo branches get main", "fjgo repo branches get OWNER/REPO main --json"}},
		"branches create":          {Usage: "fjgo repo branches create [owner/repo] --name <branch> [--from <ref>] --yes", Flags: []string{"--from <ref>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branches create --name feature --from main --dry-run --yes", "fjgo repo branches create OWNER/REPO --name feature --yes"}},
		"branches delete":          branchDelete,
		"branches remove":          renamedCommandHelp(branchDelete, "branches delete", "branches remove"),
		"collaborators list":       {Usage: "fjgo repo collaborators list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin repo collaborators list", "fjgo repo collaborators list OWNER/REPO --fields login,active"}},
		"collaborators check":      {Usage: "fjgo repo collaborators check [owner/repo] <user> [--json]", Examples: []string{"fjgo -R origin repo collaborators check alice", "fjgo repo collaborators check OWNER/REPO alice --json"}},
		"collaborators get":        {Usage: "fjgo repo collaborators get [owner/repo] <user> [--json]", Examples: []string{"fjgo -R origin repo collaborators get alice", "fjgo repo collaborators get OWNER/REPO alice --json"}},
		"collaborators permission": {Usage: "fjgo repo collaborators permission [owner/repo] <user> [--json]", Examples: []string{"fjgo -R origin repo collaborators permission alice", "fjgo repo collaborators permission OWNER/REPO alice --json"}},
		"collaborators add":        {Usage: "fjgo repo collaborators add [owner/repo] <user> [--permission <level>] --yes", Flags: []string{"--permission <read|write|admin> (default write)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo collaborators add alice --permission write --dry-run --yes", "fjgo repo collaborators add OWNER/REPO alice --yes"}},
		"collaborators remove":     collaboratorRemove,
		"collaborators delete":     renamedCommandHelp(collaboratorRemove, "collaborators remove", "collaborators delete"),
		"branch-protection list":   {Usage: "fjgo repo branch-protection list [owner/repo] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin repo branch-protection list", "fjgo repo branch-protection list OWNER/REPO --fields name,required_approvals"}},
		"branch-protection get":    {Usage: "fjgo repo branch-protection get [owner/repo] <name> [--json]", Examples: []string{"fjgo -R origin repo branch-protection get main", "fjgo repo branch-protection get OWNER/REPO main --json"}},
		"branch-protection create": protectionMutate("create"),
		"branch-protection edit":   protectionMutate("edit"),
		"branch-protection delete": protectionDelete,
		"branch-protection remove": renamedCommandHelp(protectionDelete, "branch-protection delete", "branch-protection remove"),
		"issue close":              {Usage: "fjgo repo issue close [owner/repo] <index> --yes", Flags: []string{"--dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue close 42 --dry-run --yes", "fjgo repo issue close OWNER/REPO 42 --yes"}},
		"issue comment":            {Usage: "fjgo repo issue comment [owner/repo] <index> -body <json|@file|-> --yes", Flags: []string{"-body <json|@file|->", "--dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue comment 42 -body '{\"body\":\"note\"}' --dry-run --yes", "fjgo repo issue comment OWNER/REPO 42 -body @comment.json --yes"}},
	}
	for command, spec := range mapsCloneWithPrefix(helps, "branch-protection", "branch-protections") {
		helps[command] = spec
	}
	return helps
}

func mapsCloneWithPrefix(source map[string]commandHelpSpec, from, to string) map[string]commandHelpSpec {
	out := map[string]commandHelpSpec{}
	for command, spec := range source {
		if strings.HasPrefix(command, from+" ") {
			spec = renamedCommandHelp(spec, from, to)
			out[to+strings.TrimPrefix(command, from)] = spec
		}
	}
	return out
}

func runRelease(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if help, ok := subcommandHelp(args, releaseHelp(), releaseCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	args, jsonOut := takeJSONFlag(args)
	args, full := takeFullFlag(args)
	_ = full
	switch args[0] {
	case "list":
		return runReleaseList(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "view":
		return runReleaseView(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "latest":
		return runReleaseLatest(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "create":
		return runReleaseCreate(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "edit":
		return runReleaseEdit(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "delete":
		return runReleaseDelete(ctx, client, cfg, args[1:], stdout, jsonOut)
	case "upload":
		return runReleaseUpload(ctx, client, cfg, args[1:], stdout, jsonOut, full)
	case "assets", "asset":
		return runReleaseAssets(ctx, client, cfg, args[1:], stdout, jsonOut)
	default:
		aliasArgs := append([]string{"repo", "releases"}, args...)
		if jsonOut {
			aliasArgs = append(aliasArgs, "--json")
		}
		if full {
			aliasArgs = append(aliasArgs, "--full")
		}
		return runAlias(ctx, client, cfg, aliasArgs, stdout)
	}
}

func releaseCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":          {Usage: "fjgo release list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--draft <bool>, --prerelease <bool>, --q <text>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release list", "fjgo release list OWNER/REPO --fields id,tag,title"}},
		"view":          {Usage: "fjgo release view [owner/repo] <id|tag> [flags]", Flags: []string{"--tag", "--full", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release view v1.2.3", "fjgo release view OWNER/REPO 123 --full"}},
		"latest":        {Usage: "fjgo release latest [owner/repo] [--full] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo --repo OWNER/REPO release latest", "fjgo release latest OWNER/REPO --fields id,tag,title"}},
		"create":        {Usage: "fjgo release create [owner/repo] <tag> [flags] --yes", Flags: []string{"--name <text>, --target <ref>", "--body|--notes <text>, --body-file|--notes-file <path>", "--draft, --prerelease, --hide-archive-links", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release create v1.2.3 --body-file notes.md --dry-run --yes", "fjgo release create OWNER/REPO v1.2.3 --yes"}},
		"edit":          {Usage: "fjgo release edit [owner/repo] <id> [flags] --yes", Flags: []string{"--name <text>, --target <ref>", "--body|--notes <text>, --body-file|--notes-file <path>", "--draft <bool>, --prerelease <bool>, --hide-archive-links <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release edit 123 --prerelease false --dry-run --yes", "fjgo release edit OWNER/REPO 123 --name \"v1.2.3\" --yes"}},
		"delete":        {Usage: "fjgo release delete [owner/repo] <id|tag> --yes", Flags: []string{"--tag", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo --repo OWNER/REPO release delete 123 --dry-run --yes", "fjgo release delete OWNER/REPO v1.2.3 --tag --yes"}},
		"upload":        {Usage: "fjgo release upload [owner/repo] <release-id> <file> [name=value ...] --yes", Flags: []string{"--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO release upload 123 dist/app.tar.gz name=app.tar.gz --dry-run --yes", "fjgo release upload OWNER/REPO 123 dist/app.tar.gz --yes"}},
		"assets":        {Usage: "fjgo release assets <list|delete> [owner/repo] <release-id> [asset-id] [flags]", Flags: []string{"--fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO release assets list 123", "fjgo --repo OWNER/REPO release assets delete 123 456 --dry-run --yes"}},
		"asset":         {Usage: "fjgo release asset <list|delete> [owner/repo] <release-id> [asset-id] [flags]", Flags: []string{"--fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO release asset list 123", "fjgo --repo OWNER/REPO release asset delete 123 456 --dry-run --yes"}},
		"assets list":   {Usage: "fjgo release assets list [owner/repo] <release-id> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin release assets list 123", "fjgo release assets list OWNER/REPO 123 --fields id,name,size"}},
		"assets delete": {Usage: "fjgo release assets delete [owner/repo] <release-id> <asset-id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin release assets delete 123 456 --dry-run --yes", "fjgo release assets delete OWNER/REPO 123 456 --yes"}},
		"assets remove": {Usage: "fjgo release assets remove [owner/repo] <release-id> <asset-id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin release assets remove 123 456 --dry-run --yes", "fjgo release assets remove OWNER/REPO 123 456 --yes"}},
		"asset list":    {Usage: "fjgo release asset list [owner/repo] <release-id> [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin release asset list 123", "fjgo release asset list OWNER/REPO 123 --fields id,name,size"}},
		"asset delete":  {Usage: "fjgo release asset delete [owner/repo] <release-id> <asset-id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin release asset delete 123 456 --dry-run --yes", "fjgo release asset delete OWNER/REPO 123 456 --yes"}},
		"asset remove":  {Usage: "fjgo release asset remove [owner/repo] <release-id> <asset-id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin release asset remove 123 456 --dry-run --yes", "fjgo release asset remove OWNER/REPO 123 456 --yes"}},
	}
}

func runReleaseCreate(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "release create", []string{"--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body", "--draft", "--prerelease", "--hide-archive-links", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--target", "--body", "--body-file", "--notes", "--notes-file", "-body"}); err != nil {
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
	var title, target string
	var ok bool
	args, title, ok, err = takeValueFlag(args, "--name")
	if err != nil {
		return err
	}
	args, target, _, err = takeValueFlag(args, "--target")
	if err != nil {
		return err
	}
	args, draft := boolFlag(args, "--draft")
	args, prerelease := boolFlag(args, "--prerelease")
	args, hideArchiveLinks := boolFlag(args, "--hide-archive-links")
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) < 1 {
		return newUsageError("usage: fjgo release create [owner/repo] <tag> [name=value ...] --yes", "Run `fjgo release create <tag> --dry-run --yes` first")
	}
	if !yes {
		return newUsageError("release create requires --yes")
	}
	op, ok := forgejo.OperationByID("repoCreateRelease")
	if !ok {
		return errors.New("missing repoCreateRelease operation")
	}
	body := map[string]any{"tag_name": rest[0], "name": rest[0]}
	if ok {
		body["name"] = title
	}
	if target != "" {
		body["target_commitish"] = target
	}
	if bodySet {
		body["body"] = bodyText
	}
	if draft {
		body["draft"] = true
	}
	if prerelease {
		body["prerelease"] = true
	}
	if hideArchiveLinks {
		body["hide_archive_links"] = true
	}
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
		return writeTOON(stdout, map[string]any{"release": "created"})
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "release", full)
}

func runReleaseUpload(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, jsonOut, full bool) error {
	if err := rejectUnknownFlags(args, "release upload", []string{"--yes", "--dry-run", "--print-request", "--json", "--full"}, nil); err != nil {
		return err
	}
	args, yes := takeYesFlag(args)
	args, dryRun := takeDryRunFlag(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil || len(rest) < 2 {
		return newUsageError("usage: fjgo release upload [owner/repo] <release-id> <file> [name=value ...] --yes", "Run `fjgo release upload <release-id> <file> --dry-run --yes` first")
	}
	if !yes {
		return newUsageError("release upload requires --yes")
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
		return writeTOON(stdout, map[string]any{"release_asset": "uploaded"})
	}
	if jsonOut {
		return writeAPIBody(stdout, out, "application/json", true, true)
	}
	return writeJSONAsTOON(stdout, out, "release_asset", full)
}

func runRepoIssue(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return newUsageError("usage: fjgo repo issue <close|comment>", "Run `fjgo repo issue close --help` or `fjgo repo issue comment --help`")
	}
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo repo issue <close|comment> [owner/repo] <index> [flags]\nexamples:\n  fjgo -R origin repo issue close 42 --dry-run --yes\n  fjgo -R origin repo issue comment 42 -body '{\"body\":\"note\"}' --dry-run --yes")
	}
	switch args[0] {
	case "close":
		if err := rejectUnknownFlags(args[1:], "repo issue close", []string{"--yes", "--dry-run", "--print-request"}, nil); err != nil {
			return err
		}
		args, yes := takeYesFlag(args[1:])
		args, dryRun := takeDryRunFlag(args)
		ref, rest, err := repoFromArgs(cfg, args)
		if err != nil || len(rest) != 1 {
			return newUsageError("usage: fjgo repo issue close [owner/repo] <index> --yes", "Pass `owner/repo` or use `-R origin`")
		}
		if !yes {
			return newUsageError("repo issue close requires --yes")
		}
		op, _ := forgejo.OperationByID("issueEditIssue")
		pathValues := map[string]string{"owner": ref.Owner, "repo": ref.Repo, "index": rest[0]}
		body := map[string]string{"state": "closed"}
		if dryRun {
			return writeRequestPreview(stdout, requestPreview{Operation: op.ID, Method: op.Method, Path: previewPath(op, pathValues), Body: body, AuthPresent: clientHasAuth(client), RequiresYes: true, YesProvided: yes})
		}
		current, err := client.IssueGetIssue(ctx, ref.Owner, ref.Repo, rest[0], forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		if current != nil && current.State != nil && strings.EqualFold(string(*current.State), "closed") {
			return writeTOON(stdout, map[string]any{"issue": fmt.Sprintf("#%s already closed (no-op)", rest[0])})
		}
		out, err := client.DoOperationRaw(ctx, op, pathValues, forgejo.RequestOptions{Body: body})
		if err != nil {
			return err
		}
		return writeJSONAsTOON(stdout, out, "issue", false)
	case "comment":
		if err := rejectUnknownFlags(args[1:], "repo issue comment", []string{"-body", "--yes", "--dry-run", "--print-request"}, []string{"-body"}); err != nil {
			return err
		}
		args, body, err := takeBodyFlag(args[1:])
		if err != nil {
			return err
		}
		args, yes := takeYesFlag(args)
		args, dryRun := takeDryRunFlag(args)
		ref, rest, err := repoFromArgs(cfg, args)
		if err != nil || len(rest) != 1 || body == "" {
			return newUsageError("usage: fjgo repo issue comment [owner/repo] <index> -body '{\"body\":\"...\"}' --yes", "Pass `owner/repo` or use `-R origin`")
		}
		if !yes {
			return newUsageError("repo issue comment requires --yes")
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
		return writeJSONAsTOON(stdout, out, "comment", false)
	default:
		return unknownSubcommandError("repo issue", args[0], []string{"close", "comment"})
	}
}

func runDoctor(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo doctor [owner/repo] [--json]\n\nflags:\n  --json  output deterministic redacted JSON instead of TOON\n\nexamples:\n  fjgo -R origin doctor\n  fjgo doctor OWNER/REPO --json")
	}
	if err := rejectUnknownFlags(args, "doctor", []string{"--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	if len(args) > 1 {
		return newUsageError("usage: fjgo doctor [owner/repo] [--json]", "Pass `owner/repo` or use `-R origin`")
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
		"basic_auth_present":  cfg.BasicAuth,
		"otp_present":         cfg.OTP,
		"sudo_present":        cfg.Sudo,
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
	if cfg.Token != "" || cfg.BasicAuth {
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
	if jsonOut {
		return writeJSON(stdout, report)
	}
	return writeTOON(stdout, report)
}

func runSkill(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return writeHelp(stdout, "usage: fjgo skill <install|status|generate> [flags]\nexamples:\n  fjgo skill install --force\n  fjgo skill status --json\n  fjgo skill generate --check")
	}
	if help, ok := subcommandHelp(args, "usage: fjgo skill <install|status|generate> [flags]\nflags:\n  --dir <path>, --force, --check, --json\nexamples:\n  fjgo skill install --force\n  fjgo skill status --json\n  fjgo skill generate --check", skillCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	switch args[0] {
	case "install":
		return runSkillInstall(args[1:], stdout, stderr)
	case "status":
		return runSkillStatus(args[1:], stdout, stderr)
	case "generate":
		return runSkillGenerate(args[1:], stdout)
	default:
		return unknownSubcommandError("skill", args[0], []string{"install", "status", "generate"})
	}
}

func skillCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"install":  {Usage: "fjgo skill install [--dir path] [--force] [--json]", Examples: []string{"fjgo skill install --force", "fjgo skill install --dir .agents/skills/fjgo"}},
		"status":   {Usage: "fjgo skill status [--dir path] [--json]", Examples: []string{"fjgo skill status", "fjgo skill status --dir .agents/skills/fjgo --json"}},
		"generate": {Usage: "fjgo skill generate [--check] [--json]", Examples: []string{"fjgo skill generate --check", "fjgo skill generate > internal/fjgoskill/skill/fjgo/SKILL.md"}},
	}
}

func runInstall(args []string, stdout, stderr io.Writer) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo install --skills [--dir path] [--force|--check] [--json]\nexamples:\n  fjgo install --skills --check\n  fjgo install --skills --force")
	}
	fs := flag.NewFlagSet("fjgo install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	skills := fs.Bool("skills", false, "install embedded Codex skill")
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	force := fs.Bool("force", false, "overwrite existing embedded skill files")
	check := fs.Bool("check", false, "check embedded skill install status")
	jsonOut := fs.Bool("json", false, "output JSON instead of TOON")
	if err := parseKnownFlagSet(fs, args, "install", []string{"--skills", "--dir", "--force", "--check", "--json"}, []string{"--dir"}); err != nil {
		return err
	}
	if !*skills || fs.NArg() != 0 {
		return newUsageError("usage: fjgo install --skills [--dir path] [--force|--check]", "Run `fjgo skill install --force`")
	}
	if *check {
		status := fjgoskill.Check(*dir)
		if *jsonOut {
			return writeJSON(stdout, status)
		}
		return writeTOON(stdout, status)
	}
	return installSkill(*dir, *force, *jsonOut, stdout)
}

func runSkillInstall(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo skill install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	force := fs.Bool("force", false, "overwrite existing embedded skill files")
	jsonOut := fs.Bool("json", false, "output JSON instead of TOON")
	if err := parseKnownFlagSet(fs, args, "skill install", []string{"--dir", "--force", "--json"}, []string{"--dir"}); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return newUsageError("usage: fjgo skill install [--dir path] [--force]")
	}
	return installSkill(*dir, *force, *jsonOut, stdout)
}

func runSkillStatus(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo skill status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", fjgoskill.DefaultInstallDir(), "skill install directory")
	jsonOut := fs.Bool("json", false, "output JSON instead of TOON")
	if err := parseKnownFlagSet(fs, args, "skill status", []string{"--dir", "--json"}, []string{"--dir"}); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return newUsageError("usage: fjgo skill status [--dir path]")
	}
	status := fjgoskill.Check(*dir)
	if *jsonOut {
		return writeJSON(stdout, status)
	}
	return writeTOON(stdout, status)
}

func runSkillGenerate(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("fjgo skill generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	check := fs.Bool("check", false, "check embedded SKILL.md against generated content")
	jsonOut := fs.Bool("json", false, "output JSON instead of TOON")
	if err := parseKnownFlagSet(fs, args, "skill generate", []string{"--check", "--json"}, nil); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return newUsageError("usage: fjgo skill generate [--check]")
	}
	current, want, err := fjgoskill.EmbeddedSkillCurrent()
	if err != nil {
		return err
	}
	if *check {
		status := map[string]any{"current": current}
		if *jsonOut {
			if !current {
				return newUsageError("embedded SKILL.md is stale", "Run `fjgo skill generate > internal/fjgoskill/skill/fjgo/SKILL.md`")
			}
			return writeJSON(stdout, status)
		}
		if !current {
			return newUsageError("embedded SKILL.md is stale", "Run `fjgo skill generate > internal/fjgoskill/skill/fjgo/SKILL.md`")
		}
		return writeTOON(stdout, status)
	}
	_, err = io.WriteString(stdout, want)
	return err
}

func installSkill(dir string, force, jsonOut bool, stdout io.Writer) error {
	result, err := fjgoskill.Install(dir, force)
	if err != nil {
		return err
	}
	if jsonOut {
		return writeJSON(stdout, result)
	}
	return writeTOON(stdout, result)
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
		return writeHelp(stdout, aliasHelp())
	}
	if help, ok := subcommandHelp(args, aliasHelp(), aliasCommandHelps()); ok {
		return writeHelp(stdout, help)
	}
	args, jsonOut := takeJSONFlag(args)
	switch args[0] {
	case "list":
		if err := rejectUnknownFlags(args[1:], "alias list", []string{"--json", "--fields"}, []string{"--fields"}); err != nil {
			return err
		}
		var err error
		args, fields, err := takeFieldsFlag(args)
		if err != nil {
			return err
		}
		fields, err = validatedFields(fields, []string{"command", "operation", "method", "path"}, []string{"command", "operation", "method", "path", "unsafe"}, "alias list")
		if err != nil {
			return err
		}
		if len(args) > 2 {
			return newUsageError("usage: fjgo alias list [filter] [--fields <a,b,c>] [--json]")
		}
		filter := ""
		if len(args) > 1 {
			filter = strings.ToLower(args[1])
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
	case "collisions":
		if err := rejectUnknownFlags(args[1:], "alias collisions", []string{"--json"}, nil); err != nil {
			return err
		}
		if len(args) > 2 {
			return newUsageError("usage: fjgo alias collisions [filter] [--json]")
		}
		filter := ""
		if len(args) == 2 {
			filter = strings.ToLower(args[1])
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
	case "omissions":
		if err := rejectUnknownFlags(args[1:], "alias omissions", []string{"--json"}, nil); err != nil {
			return err
		}
		if len(args) > 2 {
			return newUsageError("usage: fjgo alias omissions [filter] [--json]")
		}
		filter := ""
		if len(args) == 2 {
			filter = strings.ToLower(args[1])
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
	case "inspect":
		if err := rejectUnknownFlags(args[1:], "alias inspect", []string{"--json"}, nil); err != nil {
			return err
		}
		if len(args) < 2 {
			return newUsageError("usage: fjgo alias inspect <command...>", "Run `fjgo alias list <filter>` to find aliases")
		}
		alias, ok := aliasByCommand(args[1:])
		if !ok {
			return fmt.Errorf("unknown alias %q", strings.Join(args[1:], " "))
		}
		if jsonOut {
			return writeJSON(stdout, aliasView(alias))
		}
		return writeTOON(stdout, aliasBlocks(alias))
	default:
		return unknownSubcommandError("alias", args[0], []string{"list", "inspect", "collisions", "omissions"})
	}
}

func aliasCommandHelps() map[string]commandHelpSpec {
	return map[string]commandHelpSpec{
		"list":       {Usage: "fjgo alias list [filter] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo alias list repo", "fjgo alias list pulls --fields command,operation,path"}},
		"inspect":    {Usage: "fjgo alias inspect <command...> [--json]", Examples: []string{"fjgo alias inspect repo pulls get", "fjgo alias inspect repo issues list --json"}},
		"collisions": {Usage: "fjgo alias collisions [filter] [--json]", Examples: []string{"fjgo alias collisions pulls", "fjgo alias collisions --json"}},
		"omissions":  {Usage: "fjgo alias omissions [filter] [--json]", Examples: []string{"fjgo alias omissions release", "fjgo alias omissions --json"}},
	}
}

func runModel(args []string, stdout io.Writer) error {
	if len(args) == 0 || hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo model inspect <Model> [--json]\nexamples:\n  fjgo model inspect CreateIssueOption\n  fjgo model --json inspect CreateIssueOption")
	}
	if err := rejectUnknownFlags(args, "model inspect", []string{"--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	if len(args) != 2 || args[0] != "inspect" {
		return newUsageError("usage: fjgo model inspect <Model>", "Run `fjgo api inspect <operationId>` to find model names")
	}
	model, ok := forgejo.ModelByName(strings.TrimPrefix(args[1], "*"))
	if !ok {
		return fmt.Errorf("unknown model %q", args[1])
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

func runAlias(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	alias, rest, ok := matchAlias(args)
	if !ok {
		return unknownCommandError(args[0], rootCommands())
	}
	if hasHelp(rest) {
		return writeHelp(stdout, generatedAliasHelp(alias))
	}
	if err := rejectUnknownFlags(rest, strings.Join(alias.Command, " "), []string{"--json", "--full", "-body", "-body-raw", "--content-type", "--accept", "--output", "--raw", "--include-response", "--yes", "--dry-run", "--print-request"}, []string{"-body", "-body-raw", "--content-type", "--accept", "--output"}); err != nil {
		return err
	}
	rest, jsonOut := takeJSONFlag(rest)
	rest, full := takeFullFlag(rest)
	rest, input, err := takeAPIBodyInput(rest)
	if err != nil {
		return err
	}
	rest, output, err := takeAPIOutput(rest)
	if err != nil {
		return err
	}
	if output != "" && jsonOut {
		return newUsageError("use --output or --raw instead of --json for raw response bytes")
	}
	rest, includeResponse := takeIncludeResponse(rest)
	if output != "" && includeResponse {
		return newUsageError("use --include-response only with buffered output; --output and --raw already report transfer status")
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
	if err := validateOperationPath(op, pathValues); err != nil {
		return err
	}
	if err := validateOperationQuery(op, query); err != nil {
		return err
	}
	if input.JSON == nil && input.Raw == nil && op.BodyParam != nil && op.BodyParam.Required {
		return missingBodyError(op)
	}
	if err := validateOperationBody(op, input.JSON); err != nil {
		return err
	}
	if err := normalizeOperationBody(op, &input); err != nil {
		return err
	}
	if input.ContentType == "" {
		if input.Raw != nil && len(op.Consumes) != 0 {
			input.ContentType = op.Consumes[0]
		} else if input.JSON != nil {
			input.ContentType = "application/json"
		}
	}
	if input.Accept == "" && len(op.Produces) != 0 {
		input.Accept = strings.Join(op.Produces, ", ")
	}
	previewBody := apiBodyPreview(input)
	if dryRun {
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, pathValues),
			Query:       query,
			Body:        previewBody,
			ContentType: input.ContentType,
			Accept:      input.Accept,
			Output:      output,
			AuthPresent: clientHasAuth(client),
			RequiresYes: alias.Unsafe,
			YesProvided: yes,
		})
	}
	opts := forgejo.RequestOptions{Query: query, Body: input.JSON, RawBody: input.Raw, ContentType: input.ContentType, Accept: input.Accept}
	if output != "" {
		return streamAPIDestination(stdout, output, func(dst io.Writer) (forgejo.StreamResponse, error) {
			return client.DoOperationStream(ctx, op, pathValues, opts, dst)
		})
	}
	response, err := client.DoOperationRawResponse(ctx, op, pathValues, opts)
	if err != nil {
		return err
	}
	if includeResponse {
		return writeAPIResponse(stdout, response, strings.Join(op.Produces, ","), jsonOut, full)
	}
	if len(response.Body) == 0 {
		return writeAPIOK(stdout, jsonOut)
	}
	return writeAPIBody(stdout, response.Body, strings.Join(op.Produces, ","), jsonOut, full)
}

func generatedAliasHelp(alias forgejo.Alias) string {
	command := strings.Join(alias.Command, " ")
	positionals := make([]string, len(alias.Args))
	for i, arg := range alias.Args {
		positionals[i] = "<" + strings.ReplaceAll(arg, "_", "-") + ">"
	}
	usage := "fjgo " + command
	if len(positionals) != 0 {
		usage += " " + strings.Join(positionals, " ")
	}
	usage += " [name=value ...] [flags]"
	op, _ := forgejo.OperationByID(alias.Operation)
	flags := []string{"--json, --full", "--output <path>, --raw", "--include-response", "--accept <type>"}
	if op.BodyType != "" {
		flags = append(flags, "-body <json|@file|->, -body-raw <text|@file|->", "--content-type <type>")
	}
	if alias.Unsafe {
		flags = append(flags, "--yes (required), --dry-run, --print-request")
	}
	example := "fjgo " + command
	if len(positionals) != 0 {
		example += " " + strings.Join(positionals, " ")
	}
	examples := []string{example, example + " --json"}
	if alias.Unsafe {
		examples[1] = example
		if op.BodyType != "" {
			examples[1] += " -body '<json>'"
		}
		examples[1] += " --dry-run --yes"
	}
	return formatCommandHelp(commandHelpSpec{Usage: usage, Flags: flags, Examples: examples})
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
		if op.BodyParam != nil {
			writeOperationParams(w, "body_param", []forgejo.OperationParam{*op.BodyParam})
		}
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
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo auth status [--json]\nexamples:\n  fjgo auth status\n  fjgo -R origin auth status --json")
	}
	if err := rejectUnknownFlags(args, "auth status", []string{"--json"}, nil); err != nil {
		return err
	}
	args, jsonOut := takeJSONFlag(args)
	if len(args) > 1 || (len(args) == 1 && args[0] != "status") {
		return newUsageError("usage: fjgo auth status", "Run `fjgo auth status`")
	}
	status := map[string]any{
		"base_url":           cfg.BaseURL,
		"token_present":      cfg.Token != "",
		"basic_auth_present": cfg.BasicAuth,
		"otp_present":        cfg.OTP,
		"sudo_present":       cfg.Sudo,
		"authenticated":      false,
	}
	if cfg.RemoteRepo != nil {
		status["repo"] = cfg.RemoteRepo
	}
	if cfg.Token != "" || cfg.BasicAuth {
		user, err := client.Me(ctx)
		if err != nil {
			status["authenticated"] = false
			status["error"] = errorMessage(err)
			if jsonOut {
				return writeJSON(stdout, status)
			}
			return writeTOON(stdout, status)
		}
		status["authenticated"] = true
		status["user"] = userSummary(user)
	}
	if jsonOut {
		return writeJSON(stdout, status)
	}
	return writeTOON(stdout, status)
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

func fillRepoPathValues(values map[string]string, op forgejo.Operation, cfg runConfig) {
	if cfg.RemoteRepo == nil {
		return
	}
	if !slices.Contains(op.PathParams, "owner") || !slices.Contains(op.PathParams, "repo") {
		return
	}
	if values["owner"] == "" {
		values["owner"] = cfg.RemoteRepo.Owner
	}
	if values["repo"] == "" {
		values["repo"] = cfg.RemoteRepo.Repo
	}
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
		} else if _, ok := operationQueryParam(op, name); ok {
			query.Add(name, value)
		} else {
			valid := append(slices.Clone(op.PathParams), operationParamNames(op.QueryParams)...)
			slices.Sort(valid)
			return nil, nil, newUsageError(fmt.Sprintf("unknown parameter %q for %s", name, op.ID), "valid parameters: "+strings.Join(valid, ", "))
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
		case operationHasQueryParam(op, name):
			query.Add(name, value)
		default:
			valid := append(slices.Clone(op.PathParams), operationParamNames(op.QueryParams)...)
			valid = append(valid, operationParamNames(op.FormParams)...)
			slices.Sort(valid)
			return nil, nil, nil, nil, newUsageError(fmt.Sprintf("unknown parameter %q for %s", name, op.ID), "valid parameters: "+strings.Join(valid, ", "))
		}
	}
	return pathValues, query, fields, files, nil
}

func operationHasQueryParam(op forgejo.Operation, name string) bool {
	_, ok := operationQueryParam(op, name)
	return ok
}

func operationParamNames(params []forgejo.OperationParam) []string {
	out := make([]string, 0, len(params))
	for _, param := range params {
		out = append(out, param.Name)
	}
	return out
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
	return writeTOON(w, preview)
}

func clientHasAuth(client *forgejo.Client) bool {
	return client.HasAuth()
}

func readJSONBody(value string) (any, error) {
	b, err := readBoundedInput(value, maxRequestBodyBytes)
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
	extra := ""
	if len(field.Enum) != 0 {
		extra += " enum=" + strings.Join(field.Enum, "|")
	}
	if field.HasDefault {
		extra += " default=" + fmt.Sprint(field.Default)
	}
	if field.Description != "" {
		extra += " - " + field.Description
	}
	fmt.Fprintf(w, "%s%s: %s%s%s\n", prefix, field.Name, field.Type, required, extra)
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
	case "base-url", "host", "token", "timeout", "repo", "R", "repo-from-remote", "username", "password", "otp", "sudo":
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
	if help := errorHelp(err); len(help) != 0 {
		out["help"] = help
	}
	var httpErr forgejo.HTTPError
	if errors.As(err, &httpErr) {
		out["status"] = httpErr.StatusCode
		out["kind"] = "forgejo_api"
		out["code"] = forgejoErrorCode(httpErr)
	} else {
		out["kind"] = "cli"
		var cliErr cliError
		if errors.As(err, &cliErr) {
			out["code"] = cliErr.Code
		} else if isUsage(err) {
			out["code"] = "USAGE"
		} else {
			out["code"] = "ERROR"
		}
	}
	return out
}

func forgejoStatus(err error) int {
	var httpErr forgejo.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode
	}
	return 0
}

func forgejoErrorCode(err forgejo.HTTPError) string {
	body := strings.ToLower(err.Body)
	switch {
	case err.StatusCode == http.StatusUnauthorized && strings.Contains(body, "token"):
		return "AUTH_TOKEN_INVALID"
	case err.StatusCode == http.StatusForbidden && strings.Contains(body, "scope"):
		return "AUTH_SCOPE_MISSING"
	case err.StatusCode == http.StatusNotFound && strings.Contains(body, "repo"):
		return "REPO_NOT_FOUND"
	case err.StatusCode == http.StatusNotFound && strings.Contains(body, "issue"):
		return "ISSUE_NOT_FOUND"
	case err.StatusCode == http.StatusNotFound && (strings.Contains(body, "pull") || strings.Contains(body, "pr")):
		return "PR_NOT_FOUND"
	case err.StatusCode == http.StatusConflict:
		return "CONFLICT"
	case err.StatusCode == http.StatusUnprocessableEntity:
		return "VALIDATION"
	case err.StatusCode == http.StatusTooManyRequests || strings.Contains(body, "rate limit"):
		return "RATE_LIMITED"
	}
	switch err.StatusCode {
	case http.StatusUnauthorized:
		return "AUTH_REQUIRED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusUnprocessableEntity:
		return "VALIDATION"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	default:
		return "ERROR"
	}
}

func redactArgs(args []string) []string {
	out := slices.Clone(args)
	for i, arg := range out {
		if arg == "-token" || arg == "--token" || arg == "-password" || arg == "--password" || arg == "-otp" || arg == "--otp" {
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
		if strings.HasPrefix(arg, "--password=") {
			out[i] = "--password=redacted"
		}
		if strings.HasPrefix(arg, "-password=") {
			out[i] = "-password=redacted"
		}
		if strings.HasPrefix(arg, "--otp=") {
			out[i] = "--otp=redacted"
		}
		if strings.HasPrefix(arg, "-otp=") {
			out[i] = "-otp=redacted"
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

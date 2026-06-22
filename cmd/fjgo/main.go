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
	"slices"
	"strings"
	"time"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

const defaultBaseURL = "https://repos.astrazds.net/api/v1"

var (
	version = "v0.8.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fjgo:", errorMessage(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fjgo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baseURL := fs.String("base-url", getenv("FJGO_BASE_URL", defaultBaseURL), "Forgejo API base URL")
	token := fs.String("token", "", "Forgejo access token (or FJGO_TOKEN)")
	timeout := fs.Duration("timeout", 15*time.Second, "HTTP timeout")
	showVersion := fs.Bool("version", false, "print fjgo version")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: fjgo [flags] <version|me|get|api|repo|release>")
		fmt.Fprintln(stderr, "\ncommands:")
		fmt.Fprintln(stderr, "  version     print the Forgejo server version")
		fmt.Fprintln(stderr, "  me          print the authenticated user")
		fmt.Fprintln(stderr, "  get <path>  GET an API path, for example /version")
		fmt.Fprintln(stderr, "  api list [filter]")
		fmt.Fprintln(stderr, "  api inspect <operationId>")
		fmt.Fprintln(stderr, "  api call <operationId> [name=value ...] [-body JSON|@file|-]")
		fmt.Fprintln(stderr, "  repo get <owner/repo>")
		fmt.Fprintln(stderr, "  repo topics <owner/repo> [--set comma,separated,topics]")
		fmt.Fprintln(stderr, "  repo avatar <owner/repo> <png>")
		fmt.Fprintln(stderr, "  release list <owner/repo>")
		fmt.Fprintln(stderr, "\nflags:")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "fjgo %s %s %s\n", version, commit, date)
		return nil
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("missing command")
	}
	if *token == "" {
		*token = os.Getenv("FJGO_TOKEN")
	}

	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	client, err := forgejo.NewClient(*baseURL, *token, http.DefaultClient)
	if err != nil {
		return err
	}

	switch fs.Arg(0) {
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
	case "repo":
		return runRepo(ctx, client, fs.Args()[1:], stdout)
	case "release":
		return runRelease(ctx, client, fs.Args()[1:], stdout)
	default:
		return fmt.Errorf("unknown command %q", fs.Arg(0))
	}
}

func runAPI(ctx context.Context, client *forgejo.Client, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fjgo api <list|inspect|call>")
	}
	switch args[0] {
	case "list":
		filter := ""
		if len(args) > 1 {
			filter = strings.ToLower(args[1])
		}
		for _, op := range forgejo.Operations() {
			line := fmt.Sprintf("%-45s %-6s %s", op.ID, op.Method, op.Path)
			if filter == "" || strings.Contains(strings.ToLower(line+" "+op.Summary), filter) {
				fmt.Fprintln(stdout, line)
			}
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
		writeOperation(stdout, op)
		return nil
	case "call":
		return callOperation(ctx, client, args[1:], stdout, stderr)
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
	if op.BodyType != "" {
		fmt.Fprintf(w, "body: %s\n", op.BodyType)
	}
	if op.ReturnType != "" {
		fmt.Fprintf(w, "returns: %s\n", op.ReturnType)
	}
}

func callOperation(ctx context.Context, client *forgejo.Client, args []string, stdout, stderr io.Writer) error {
	args, body, err := takeBodyFlag(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("usage: fjgo api call <operationId> [name=value ...]")
	}
	op, ok := forgejo.OperationByID(args[0])
	if !ok {
		return fmt.Errorf("unknown operation %q", args[0])
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

func runRepo(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: fjgo repo <get|topics> <owner/repo>; fjgo repo avatar <owner/repo> <png>")
	}
	owner, repo, err := splitRepo(args[1])
	if err != nil {
		return err
	}
	switch args[0] {
	case "get":
		if len(args) != 2 {
			return errors.New("usage: fjgo repo get <owner/repo>")
		}
		out, err := client.RepoGet(ctx, owner, repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeJSON(stdout, out)
	case "topics":
		args, topics, err := takeSetFlag(args[2:])
		if err != nil {
			return err
		}
		if len(args) != 0 {
			return errors.New("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]")
		}
		if topics != "" {
			if err := client.RepoUpdateTopics(ctx, owner, repo, &forgejo.RepoTopicOptions{Topics: splitCSV(topics)}, forgejo.RequestOptions{}); err != nil {
				return err
			}
		}
		out, err := client.RepoListTopics(ctx, owner, repo, forgejo.RequestOptions{})
		if err != nil {
			return err
		}
		return writeJSON(stdout, out)
	case "avatar":
		if len(args) != 3 {
			return errors.New("usage: fjgo repo avatar <owner/repo> <png>")
		}
		b, err := os.ReadFile(args[2])
		if err != nil {
			return err
		}
		return client.RepoUpdateAvatar(ctx, owner, repo, &forgejo.UpdateRepoAvatarOption{Image: base64.StdEncoding.EncodeToString(b)}, forgejo.RequestOptions{})
	default:
		return fmt.Errorf("unknown repo command %q", args[0])
	}
}

func runRelease(ctx context.Context, client *forgejo.Client, args []string, stdout io.Writer) error {
	if len(args) != 2 || args[0] != "list" {
		return errors.New("usage: fjgo release list <owner/repo>")
	}
	owner, repo, err := splitRepo(args[1])
	if err != nil {
		return err
	}
	out, err := client.RepoListReleases(ctx, owner, repo, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	return writeJSON(stdout, out)
}

func splitRepo(full string) (string, string, error) {
	owner, repo, ok := strings.Cut(full, "/")
	if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("expected owner/repo, got %q", full)
	}
	return owner, repo, nil
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

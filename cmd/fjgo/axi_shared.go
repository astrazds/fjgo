package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

const (
	defaultListLimit = "30"
)

func commandWithRepo(command string, ref repoRef) string {
	if ref.Owner == "" || ref.Repo == "" {
		return command
	}
	return command + " " + ref.Owner + "/" + ref.Repo
}

func takeValueFlag(args []string, flag string) ([]string, string, bool, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == flag {
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
				return nil, "", false, newUsageError(flag + " requires a value")
			}
			return append(out, args[i+2:]...), args[i+1], true, nil
		}
		if strings.HasPrefix(arg, flag+"=") {
			return append(out, args[i+1:]...), strings.TrimPrefix(arg, flag+"="), true, nil
		}
		out = append(out, arg)
	}
	return out, "", false, nil
}

func takeAllValueFlags(args []string, flags ...string) ([]string, []string, error) {
	out := make([]string, 0, len(args))
	var values []string
	valueFlags := map[string]bool{}
	for _, flag := range flags {
		valueFlags[flag] = true
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		matched := false
		for _, flag := range flags {
			if arg == flag {
				if i+1 == len(args) || (strings.HasPrefix(args[i+1], "--") && !strings.HasPrefix(args[i+1], "---")) {
					return nil, nil, newUsageError(flag + " requires a value")
				}
				values = append(values, args[i+1])
				i++
				matched = true
				break
			}
			if strings.HasPrefix(arg, flag+"=") {
				values = append(values, strings.TrimPrefix(arg, flag+"="))
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			name := arg
			if before, _, ok := strings.Cut(arg, "="); ok {
				name = before
			}
			if valueFlags[name] {
				continue
			}
		}
		out = append(out, arg)
	}
	return out, values, nil
}

func takeBodyText(args []string, required bool) ([]string, string, bool, error) {
	var body string
	var found bool
	var sources []string
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--body" || arg == "-body":
			if i+1 == len(args) {
				return nil, "", false, newUsageError(arg + " requires text")
			}
			body = args[i+1]
			found = true
			sources = append(sources, arg)
			i++
		case strings.HasPrefix(arg, "--body="):
			body = strings.TrimPrefix(arg, "--body=")
			found = true
			sources = append(sources, "--body")
		case strings.HasPrefix(arg, "-body="):
			body = strings.TrimPrefix(arg, "-body=")
			found = true
			sources = append(sources, "-body")
		case arg == "--body-file":
			if i+1 == len(args) {
				return nil, "", false, newUsageError("--body-file requires a path")
			}
			b, err := os.ReadFile(args[i+1])
			if err != nil {
				return nil, "", false, newUsageError(fmt.Sprintf("could not read --body-file %s: %v", args[i+1], err))
			}
			body = string(b)
			found = true
			sources = append(sources, "--body-file")
			i++
		case strings.HasPrefix(arg, "--body-file="):
			path := strings.TrimPrefix(arg, "--body-file=")
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, "", false, newUsageError(fmt.Sprintf("could not read --body-file %s: %v", path, err))
			}
			body = string(b)
			found = true
			sources = append(sources, "--body-file")
		default:
			out = append(out, arg)
		}
	}
	if len(sources) > 1 {
		return nil, "", false, newUsageError("use only one body source: "+strings.Join(sources, ", "), "Use --body \"...\" or --body-file <path>")
	}
	if required && !found {
		return nil, "", false, newUsageError("--body or --body-file is required", "Use --body \"...\" for inline text or --body-file <path> for markdown")
	}
	return out, body, found, nil
}

func readPipedValue(noun string, allowFlagValue string, allowFlag bool) (string, error) {
	if allowFlag {
		if allowFlagValue == "" {
			return "", newUsageError("--body requires a value")
		}
		return allowFlagValue, nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		if noun == "secret" {
			return "", newUsageError("secret value is required: pipe the value via stdin", "echo -n \"<value>\" | fjgo secret set <name> --yes")
		}
		return "", newUsageError(noun + " value is required: pass --body <value> or pipe the value via stdin")
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "", newUsageError(noun + " value is required")
	}
	return string(b), nil
}

func validatedFields(requested, defaults, available []string, command string) ([]string, error) {
	if len(requested) == 0 {
		return slices.Clone(defaults), nil
	}
	availableSet := map[string]bool{}
	for _, field := range available {
		availableSet[field] = true
	}
	seen := map[string]bool{}
	var out []string
	var unknown []string
	for _, field := range requested {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		if !availableSet[field] {
			unknown = append(unknown, field)
			continue
		}
		out = append(out, field)
	}
	if len(unknown) != 0 {
		names := slices.Clone(available)
		slices.Sort(names)
		return nil, newUsageError(
			fmt.Sprintf("unknown field(s) for `%s`: %s", command, strings.Join(unknown, ", ")),
			"available fields: "+strings.Join(names, ", "),
		)
	}
	return out, nil
}

func fieldNames(row map[string]any) []string {
	fields := make([]string, 0, len(row))
	for key := range row {
		fields = append(fields, key)
	}
	slices.Sort(fields)
	return fields
}

func writeRows(stdout io.Writer, label string, rows []map[string]any, fields []string, total int64, empty string, help []string) error {
	blocks := toonBlocks{}
	if len(rows) == 0 {
		blocks = append(blocks, map[string]any{label: empty})
		if len(help) != 0 {
			blocks = append(blocks, helpBlock(help))
		}
		return writeTOON(stdout, blocks)
	}
	if total > 0 && total != int64(len(rows)) {
		blocks = append(blocks, map[string]any{"count": fmt.Sprintf("%d of %d total", len(rows), total)})
	} else {
		blocks = append(blocks, map[string]any{"count": len(rows)})
	}
	blocks = append(blocks, tableBlock(label, fields, rows))
	if len(help) != 0 {
		blocks = append(blocks, helpBlock(help))
	}
	return writeTOON(stdout, blocks)
}

func rowsSelect(rows []map[string]any, fields []string) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, selectFields(row, fields))
	}
	return out
}

func responseTotal(resp forgejo.RawResponse) int64 {
	raw := resp.Header.Get("X-Total-Count")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func decodeBody[T any](body []byte) (T, error) {
	var out T
	if len(strings.TrimSpace(string(body))) == 0 {
		return out, nil
	}
	err := json.Unmarshal(body, &out)
	return out, err
}

func queryFromPairs(args []string) (url.Values, error) {
	query := url.Values{}
	for _, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("expected name=value, got %q", arg)
		}
		query.Add(name, value)
	}
	return query, nil
}

func addQueryFlag(query url.Values, flagValue string, present bool, queryName string) {
	if present {
		query.Set(queryName, flagValue)
	}
}

func boolFlag(args []string, name string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == name {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return out, found
}

func parseBoolValue(name, value string) (bool, error) {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, newUsageError(name + " expects true or false")
	}
	return b, nil
}

func parseInt64Value(name, value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, newUsageError(name + " expects an integer")
	}
	return n, nil
}

func joinLabelNames(labels []*forgejo.Label) string {
	var names []string
	for _, label := range labels {
		if label != nil && label.Name != "" {
			names = append(names, label.Name)
		}
	}
	return strings.Join(names, "|")
}

func userName(user *forgejo.User) string {
	if user == nil {
		return ""
	}
	return user.UserName
}

func stateString(state *forgejo.StateType) string {
	if state == nil {
		return ""
	}
	return strings.ToLower(string(*state))
}

func reviewState(value string) *forgejo.ReviewStateType {
	state := forgejo.ReviewStateType(value)
	return &state
}

func issueState(value string) *forgejo.StateType {
	state := forgejo.StateType(value)
	return &state
}

func repoPath(ref repoRef) map[string]string {
	return map[string]string{"owner": ref.Owner, "repo": ref.Repo}
}

func writeMutationPreview(stdout io.Writer, client *forgejo.Client, operation string, ref repoRef, extraPath map[string]string, body any, yes bool) error {
	op, ok := forgejo.OperationByID(operation)
	if !ok {
		return fmt.Errorf("unknown operation %q", operation)
	}
	pathValues := repoPath(ref)
	for key, value := range extraPath {
		pathValues[key] = value
	}
	return writeRequestPreview(stdout, requestPreview{
		Operation:   op.ID,
		Method:      op.Method,
		Path:        previewPath(op, pathValues),
		Body:        body,
		AuthPresent: clientHasAuth(client),
		RequiresYes: op.Method != http.MethodGet,
		YesProvided: yes,
	})
}

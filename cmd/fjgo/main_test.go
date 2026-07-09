package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

func TestMain(m *testing.M) {
	_ = os.Unsetenv("FJGO_HOST")
	_ = os.Unsetenv("FJGO_BASE_URL")
	_ = os.Unsetenv("FJGO_TOKEN")
	_ = os.Unsetenv("FJGO_REPO")
	os.Exit(m.Run())
}

func TestAPICallRunsGeneratedOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "getVersion"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "version: test\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestVersionCommandUsesGeneratedOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "version: test\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestVersionFlagPrintsBinaryVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "fjgo v0.16.0 none unknown\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestHomeNoArgsShowsAxiHeader(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"bin:", "description:", "repo: none", "help["} {
		if !strings.Contains(got, want) {
			t.Fatalf("home output missing %q:\n%s", want, got)
		}
	}
}

func TestHomeShowsCheapTotals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"full_name":      "astra/fjgo",
				"default_branch": "main",
			})
		case "/api/v1/repos/astra/fjgo/issues":
			w.Header().Set("X-Total-Count", "7")
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"number": 1,
				"title":  "Bug",
				"state":  "open",
				"user":   map[string]any{"login": "alice"},
			}})
		case "/api/v1/repos/astra/fjgo/pulls":
			w.Header().Set("X-Total-Count", "5")
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"number": 2,
				"title":  "Fix",
				"state":  "open",
				"user":   map[string]any{"login": "bob"},
			}})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "--repo", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"issues_count: 1 of 7 total", "pulls_count: 1 of 5 total", "issues[1]{number,title,state,author}", "pulls[1]{number,title,state,author}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("home output missing %q:\n%s", want, got)
		}
	}
}

func TestRunCLIUsageErrorStructuredOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"api", "call", "repoGet", "owner=astra", "--bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"code: USAGE", "error:", "unknown flag --bogus", "help["} {
		if !strings.Contains(got, want) {
			t.Fatalf("structured error missing %q:\n%s", want, got)
		}
	}
}

func TestAXIRootUnknownFlagIsStructuredWithoutHelpDump(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"--json", "--bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{`"code": "USAGE"`, "unknown flag --bogus for `fjgo`", "valid flags for `fjgo`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("structured error missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "usage: fjgo [flags]") {
		t.Fatalf("root help leaked into JSON error:\n%s", got)
	}
}

func TestAXIUnknownFlagsFailLoud(t *testing.T) {
	cases := [][]string{
		{"hook", "capture", "--bogus"},
		{"alias", "list", "--bogus"},
		{"alias", "collisions", "--bogus"},
		{"skill", "status", "--bogus"},
		{"install", "--skills", "--bogus"},
		{"model", "inspect", "CreateIssueOption", "--bogus"},
		{"auth", "status", "--bogus"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runCLI(t.Context(), args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("code = %d, stdout = %s", code, stdout.String())
			}
			got := stdout.String()
			for _, want := range []string{"code: USAGE", "unknown flag --bogus"} {
				if !strings.Contains(got, want) {
					t.Fatalf("structured error missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestAXIFocusedSubcommandHelp(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		usage string
	}{
		{"issue list", []string{"issue", "list", "--help"}, "usage: fjgo issue list"},
		{"pr view", []string{"pr", "view", "--help"}, "usage: fjgo pr view"},
		{"release list", []string{"release", "list", "--help"}, "usage: fjgo release list"},
		{"run list", []string{"run", "list", "--help"}, "usage: fjgo run list"},
		{"repo get", []string{"repo", "get", "--help"}, "usage: fjgo repo get"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(t.Context(), tc.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
			}
			got := stdout.String()
			if !strings.Contains(got, tc.usage) {
				t.Fatalf("help missing focused usage %q:\n%s", tc.usage, got)
			}
			if strings.Contains(got, "subcommands:") {
				t.Fatalf("focused help included group subcommands:\n%s", got)
			}
		})
	}
}

func TestAXIUnknownSubcommandSuggestsValidSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"issue", "lsit"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	got := stdout.String()
	for _, want := range []string{"code: USAGE", "unknown issue command \\\"lsit\\\"", "valid subcommands for `issue`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("structured error missing %q:\n%s", want, got)
		}
	}
}

func TestAXINestedUnknownSubcommandsSuggestValidSubcommands(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"repo issue", []string{"repo", "issue", "wat"}, "valid subcommands for `repo issue`"},
		{"repo branches", []string{"repo", "branches", "wat"}, "valid subcommands for `repo branches`"},
		{"repo collaborators", []string{"repo", "collaborators", "wat"}, "valid subcommands for `repo collaborators`"},
		{"repo branch protection", []string{"repo", "branch-protection", "wat"}, "valid subcommands for `repo branch-protection`"},
		{"issue dependencies", []string{"issue", "dependencies", "wat"}, "valid subcommands for `issue dependencies`"},
		{"issue reactions", []string{"issue", "reactions", "wat"}, "valid subcommands for `issue reactions`"},
		{"issue deadline", []string{"issue", "deadline", "wat"}, "valid subcommands for `issue deadline`"},
		{"issue time", []string{"issue", "time", "wat"}, "valid subcommands for `issue time`"},
		{"release assets", []string{"release", "assets", "wat"}, "valid subcommands for `release assets`"},
		{"top level", []string{"project", "list"}, "valid commands:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runCLI(t.Context(), tc.args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("code = %d, stdout = %s", code, stdout.String())
			}
			got := stdout.String()
			for _, want := range []string{"code: USAGE", tc.want} {
				if !strings.Contains(got, want) {
					t.Fatalf("structured error missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestAPICallJSONEscapeHatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "--json", "call", "getVersion"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != `{"version":"test"}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAPICallTruncatesLongStringsByDefault(t *testing.T) {
	longBody := strings.Repeat("a", defaultTruncateChars+20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"body": longBody})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "getVersion"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "truncated") || !strings.Contains(got, "help[") || strings.Contains(got, longBody) {
		t.Fatalf("stdout = %s", got)
	}
}

func TestHelpDoesNotPrintTokenFromEnvironment(t *testing.T) {
	t.Setenv("FJGO_TOKEN", "secret-token")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--help"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v", err)
	}
	if strings.Contains(stderr.String(), "secret-token") {
		t.Fatalf("help leaked token:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("help leaked token:\n%s", stdout.String())
	}
	if strings.Contains(stderr.String(), "fjgo:") || strings.Contains(stderr.String(), "flag: help requested") {
		t.Fatalf("help printed error text:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "usage: fjgo") {
		t.Fatalf("help missing usage:\n%s", stdout.String())
	}
}

func TestAPIInspectShowsOperationMetadata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "inspect", "createCurrentUserRepo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"id: createCurrentUserRepo",
		"method: POST",
		"path: /user/repos",
		"body: CreateRepoOption",
		"body_fields[",
		"name,string,true",
		"returns: Repository",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestAPIInspectShowsUnsupportedUpload(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "inspect", "repoCreateReleaseAttachment"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"query_params[",
		"name,string,false",
		"form_params[",
		"attachment,file,false",
		"upload: multipart/form-data",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestAPIInspectShowsQueryParams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "inspect", "repoSearch"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"query_params[",
		"q,string,false,keyword",
		"limit,integer,false",
		"private,boolean,false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestAPIInspectJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "--json", "inspect", "createCurrentUserRepo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	var got struct {
		ID         string `json:"id"`
		Body       string `json:"body"`
		BodyFields []struct {
			Name     string `json:"name"`
			Required bool   `json:"required"`
		} `json:"body_fields"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "createCurrentUserRepo" || got.Body != "CreateRepoOption" {
		t.Fatalf("inspect json = %#v", got)
	}
	foundRequiredName := false
	for _, field := range got.BodyFields {
		if field.Name == "name" && field.Required {
			foundRequiredName = true
		}
	}
	if !foundRequiredName {
		t.Fatalf("body_fields = %#v", got.BodyFields)
	}
}

func TestAPICallMissingPathParamHintsInspect(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "call", "repoGet", "owner=astra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "fjgo api inspect repoGet") {
		t.Fatalf("error = %q", got)
	}
}

func TestAPICallRejectsUploadOperation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"api", "call", "repoCreateReleaseAttachment", "owner=astra", "repo=fjgo", "id=1", "--yes"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "multipart/form-data upload") || !strings.Contains(got, "api upload repoCreateReleaseAttachment") {
		t.Fatalf("error = %q", got)
	}
}

func TestAPIUploadReleaseAttachment(t *testing.T) {
	dir := t.TempDir()
	asset := dir + "/asset.txt"
	if err := os.WriteFile(asset, []byte("asset bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases/42/assets" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("name") != "asset.txt" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "token secret" {
			t.Fatalf("Authorization = %q", got)
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "asset.txt" {
			t.Fatalf("filename = %q", header.Filename)
		}
		b, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "asset bytes" {
			t.Fatalf("file = %q", b)
		}
		_, _ = w.Write([]byte(`{"name":"asset.txt"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "-token", "secret", "api", "upload", "repoCreateReleaseAttachment", "owner=astra", "repo=fjgo", "id=42", "name=asset.txt", "attachment=@" + asset, "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "name: asset.txt\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAPIDryRunDoesNotCallServer(t *testing.T) {
	t.Setenv("FJGO_TOKEN", "")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "createCurrentUserRepo", "--yes", "--dry-run", "-body", `{"name":"demo"}`}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if called {
		t.Fatal("server was called")
	}
	if got := stdout.String(); !strings.Contains(got, "method: POST") || !strings.Contains(got, "auth_present: false") {
		t.Fatalf("preview = %s", got)
	}
}

func TestAPICallMissingBodyFailsBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "createCurrentUserRepo", "--yes"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "requires -body CreateRepoOption") || !strings.Contains(got, "fjgo api inspect createCurrentUserRepo") {
		t.Fatalf("error = %q", got)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestAPICallRequiresYesForMutation(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "createCurrentUserRepo", "-body", `{"name":"demo"}`}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "createCurrentUserRepo requires --yes") {
		t.Fatalf("error = %q", got)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestAPICallAllowsMutationWithYes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/repos" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"full_name":"astra/demo"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "createCurrentUserRepo", "--yes", "-body", `{"name":"demo"}`}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "full_name: astra/demo\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAPIRawGetAndMutationDryRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("verbose") != "true" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"version":"test"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "raw", "GET", "/version", "verbose=true"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "version: test\n" {
		t.Fatalf("stdout = %q", got)
	}

	called := false
	mutation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer mutation.Close()
	stdout.Reset()
	stderr.Reset()
	err = run(t.Context(), []string{"-base-url", mutation.URL + "/api/v1", "api", "raw", "PATCH", "/repos/astra/fjgo", "--dry-run", "--yes", "-body", `{"description":"x"}`}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if called {
		t.Fatal("server was called")
	}
	if got := stdout.String(); !strings.Contains(got, "method: PATCH") || !strings.Contains(got, "description: x") {
		t.Fatalf("preview = %s", got)
	}
}

func TestAPICallUsesCommandLocalRepoContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "api", "call", "repoGet", "--repo", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "full_name: astra/fjgo") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestModelInspectShowsFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"model", "inspect", "CreateRepoOption"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"model: CreateRepoOption",
		"name,string,true",
		"private,bool,false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("model output missing %q:\n%s", want, got)
		}
	}
}

func TestRepoGetAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "get", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "full_name: astra/fjgo") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRepoTopicsAliasCanSetAndRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/topics" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			var body map[string][]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(body["topics"]) != "[go cli]" {
				t.Fatalf("topics = %#v", body["topics"])
			}
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string][]string{"topics": {"go", "cli"}})
		default:
			t.Fatalf("method = %s", r.Method)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "topics", "astra/fjgo", "--set", "go,cli", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "topics[2]{name}") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRepoTopicsSetRequiresYes(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "topics", "astra/fjgo", "--set", "go,cli"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "repo topics --set requires --yes") {
		t.Fatalf("error = %q", got)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestRepoTopicsDryRunDoesNotCallServer(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "topics", "astra/fjgo", "--set", "go,cli", "--yes", "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if called {
		t.Fatal("server was called")
	}
	got := stdout.String()
	if !strings.Contains(got, "operation: repoUpdateTopics") || !strings.Contains(got, "topics[2]: go,cli") {
		t.Fatalf("preview = %s", got)
	}
}

func TestRepoTopicsRequiresRepo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"repo", "topics"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "usage: fjgo repo topics <owner/repo>") {
		t.Fatalf("error = %q", got)
	}
}

func TestIssueListCuratedFieldsAndCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/issues" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "open" || r.URL.Query().Get("type") != "issues" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		w.Header().Set("X-Total-Count", "7")
		_, _ = w.Write([]byte(`[{"number":1,"title":"Bug","state":"open","user":{"login":"alice"},"comments":2}]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "issue", "list", "astra/fjgo", "--fields", "number,title,state,author"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"count: 1 of 7 total", "issues[1]{number,title,state,author}", "1,Bug,open,alice"} {
		if !strings.Contains(got, want) {
			t.Fatalf("issue list output missing %q:\n%s", want, got)
		}
	}
}

func TestWorkflowFieldsRejectUnknownField(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"issue", "list", "astra/fjgo", "--fields", "number,nope"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if got := stdout.String(); !strings.Contains(got, "unknown field") || !strings.Contains(got, "available fields") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestPRChecksUsesHeadSHA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo/pulls/5":
			_, _ = w.Write([]byte(`{"number":5,"title":"PR","head":{"sha":"abcdef1234567890","ref":"feature"}}`))
		case "/api/v1/repos/astra/fjgo/commits/abcdef1234567890/statuses":
			_, _ = w.Write([]byte(`[{"context":"ci","status":"success","description":"ok"}]`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "pr", "checks", "astra/fjgo", "5"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "checks[1]{context,status,description}") || !strings.Contains(got, "ci,success,ok") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestRunViewLogFailedUsesActionTasksAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo/actions/runs/1":
			_, _ = w.Write([]byte(`{"id":1,"index_in_repo":9,"title":"Verify","status":"failure","workflow_id":"verify.yml"}`))
		case "/api/v1/repos/astra/fjgo/actions/tasks":
			if r.URL.Query().Get("status") != "failure" {
				t.Fatalf("status query = %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"total_count":1,"workflow_runs":[{"id":3,"run_number":9,"display_title":"Verify job","name":"test","status":"failure","workflow_id":"verify.yml","head_sha":"abcdef123456"}]}`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "run", "view", "astra/fjgo", "1", "--log-failed"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"failed_tasks[1]", "Verify job", "status: failure"} {
		if !strings.Contains(got, want) {
			t.Fatalf("run task output missing %q:\n%s", want, got)
		}
	}
}

func TestSecretDryRunRedactsStdinValue(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	if _, err := w.WriteString("super-secret"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = run(t.Context(), []string{"secret", "set", "astra/fjgo", "TOKEN", "--dry-run", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if strings.Contains(got, "super-secret") || !strings.Contains(got, "data: redacted") {
		t.Fatalf("secret preview leaked or missed redaction:\n%s", got)
	}
}

func TestWorkflowDispatchDryRun(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "workflow", "run", "astra/fjgo", "verify.yml", "--ref", "main", "--input", "smoke=true", "--dry-run", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if called {
		t.Fatal("server was called")
	}
	got := stdout.String()
	for _, want := range []string{"operation: DispatchWorkflow", "path: /repos/astra/fjgo/actions/workflows/verify.yml/dispatches", "smoke: \"true\""} {
		if !strings.Contains(got, want) {
			t.Fatalf("workflow preview missing %q:\n%s", want, got)
		}
	}
}

func TestRunWatchUsesActionRunAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo/actions/runs/1":
			_, _ = w.Write([]byte(`{"id":1,"index_in_repo":9,"title":"Verify","status":"success","workflow_id":"verify.yml"}`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "run", "watch", "astra/fjgo", "1", "--timeout", "1s"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status: success") {
		t.Fatalf("watch stdout = %s", stdout.String())
	}
}

func TestWorkflowViewUsesContentsAPI(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("name: verify\non: push\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/contents/.forgejo/workflows/verify.yml" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":     "verify.yml",
			"path":     ".forgejo/workflows/verify.yml",
			"type":     "file",
			"size":     22,
			"encoding": "base64",
			"content":  content,
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "workflow", "view", "astra/fjgo", "verify.yml"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "content:") || !strings.Contains(stdout.String(), "name: verify") {
		t.Fatalf("workflow view = %s", stdout.String())
	}
}

func TestNonAPIAliasesAreNotRegistered(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "run-download", args: []string{"run", "download", "astra/fjgo", "1", "--job", "0"}, want: `unknown run command "download"`},
		{name: "run-rerun", args: []string{"run", "rerun", "astra/fjgo", "1", "--yes"}, want: `unknown run command "rerun"`},
		{name: "workflow-enable", args: []string{"workflow", "enable", "astra/fjgo", "verify.yml", "--yes"}, want: `unknown workflow command "enable"`},
		{name: "search-code", args: []string{"search", "code", "query"}, want: `unknown search command "code"`},
		{name: "project", args: []string{"project", "list"}, want: `unknown command "project"`},
		{name: "release-download", args: []string{"release", "download", "astra/fjgo", "7", "asset.txt"}, want: `unknown command "repo"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(t.Context(), tc.args, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "not supported") {
				t.Fatalf("removed alias still reports unsupported stub: %q", err)
			}
		})
	}
}

func TestEmbeddedSkillMentionsStaticGuidanceCommands(t *testing.T) {
	b, err := os.ReadFile("../../internal/fjgoskill/skill/fjgo/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, want := range []string{
		"fjgo -R origin issue list --state open",
		"fjgo -R origin pr list",
		"fjgo -R origin run list",
		"`issue`, `pr`, `run`, `workflow`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("embedded skill missing %q", want)
		}
	}
}

func TestRepoAvatarAlias(t *testing.T) {
	dir := t.TempDir()
	icon := dir + "/icon.png"
	if err := os.WriteFile(icon, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/avatar" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["image"] != "cG5n" {
			t.Fatalf("image = %q", body["image"])
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "avatar", "astra/fjgo", icon, "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
}

func TestRepoAvatarRequiresYes(t *testing.T) {
	dir := t.TempDir()
	icon := dir + "/icon.png"
	if err := os.WriteFile(icon, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "avatar", "astra/fjgo", icon}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "repo avatar requires --yes") {
		t.Fatalf("error = %q", got)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestRepoAvatarDryRunDoesNotReadFileOrCallServer(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "avatar", "astra/fjgo", "/no/such/icon.png", "--yes", "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if called {
		t.Fatal("server was called")
	}
	got := stdout.String()
	if !strings.Contains(got, "operation: repoUpdateAvatar") || strings.Contains(got, "cG5n") {
		t.Fatalf("preview = %s", got)
	}
}

func TestRepoIssueCloseAlreadyClosedIsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/issues/7" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"number": 7, "state": "closed"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "issue", "close", "astra/fjgo", "7", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "already closed (no-op)") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestReleaseListAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v0.15.0"}]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "release", "list", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "v0.15.0") || !strings.Contains(got, "releases[1]") {
		t.Fatalf("stdout = %q", got)
	}
}

func TestReleaseCreateAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tag_name"] != "v1.2.3" || body["name"] != "Release" || body["prerelease"] != true {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "release", "create", "astra/fjgo", "v1.2.3", "name=Release", "prerelease=true", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "tag_name: v1.2.3\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestReleaseCreateDryRunUsesRemoteRepo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", "https://v15.next.forgejo.org/api/v1", "release", "create", "kavemand/.forgejo", "v1.2.3", "--yes", "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "operation: repoCreateRelease") || !strings.Contains(got, "tag_name: v1.2.3") {
		t.Fatalf("preview = %s", got)
	}
}

func TestGeneratedAliasDispatchesGETOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.String(); got != "/api/v1/repos/astra/fjgo/issues?state=open" {
			t.Fatalf("url = %q", got)
		}
		_, _ = w.Write([]byte(`[{"number":1}]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "issues", "list", "astra/fjgo", "state=open"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "result[1]{number}:\n  1\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestGeneratedAliasDispatchesBodyOperation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/issues" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["title"] != "bug" {
			t.Fatalf("body = %#v", body)
		}
		_, _ = w.Write([]byte(`{"number":2}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "issues", "create", "astra/fjgo", "--yes", "-body", `{"title":"bug"}`}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "number: 2\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestGeneratedAliasRequiresYesForPost(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "issues", "create", "astra/fjgo", "-body", `{"title":"bug"}`}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error = %q", err)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestGeneratedAliasMissingBodyFailsBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "issues", "create", "astra/fjgo", "--yes"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "requires -body CreateIssueOption") || !strings.Contains(got, "fjgo api inspect issueCreateIssue") {
		t.Fatalf("error = %q", got)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestGeneratedAliasRequiresYesForDelete(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "delete", "astra/fjgo"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error = %q", err)
	}
	if called {
		t.Fatal("server was called")
	}
}

func TestGeneratedAliasAllowsDeleteWithYes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "delete", "astra/fjgo", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
}

func TestAliasInspectShowsMapping(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"alias", "inspect", "repo", "issues", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"command: repo issues get",
		"operation: issueGetIssue",
		"method: GET",
		"path: \"/repos/{owner}/{repo}/issues/{index}\"",
		"args[2]: owner/repo,index",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
	}
}

func TestAliasCollisionsCommandRuns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"alias", "collisions"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
}

func TestAliasInspectShowsBodyFieldsAndQueryParams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"alias", "inspect", "repo", "issues", "create"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"body_fields[",
		"title,string,true",
		"returns: Issue",
		"requires: --yes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("alias inspect missing %q:\n%s", want, got)
		}
	}
}

func TestAliasCollisionPolicyKeepsNaturalGets(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"alias", "inspect", "repo", "pulls", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "operation: repoGetPullRequest") {
		t.Fatalf("pull alias inspect = %s", got)
	}
	stdout.Reset()
	stderr.Reset()
	err = run(t.Context(), []string{"alias", "inspect", "repo", "git", "commits", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "operation: repoGetSingleCommit") {
		t.Fatalf("commit alias inspect = %s", got)
	}
	stdout.Reset()
	stderr.Reset()
	err = run(t.Context(), []string{"alias", "inspect", "repo", "pulls", "download", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "operation: repoDownloadPullDiffOrPatch") {
		t.Fatalf("pull download alias inspect = %s", got)
	}
}

func TestReleaseUploadAlias(t *testing.T) {
	dir := t.TempDir()
	asset := dir + "/asset.txt"
	if err := os.WriteFile(asset, []byte("asset bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases/42/assets" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if _, _, err := r.FormFile("attachment"); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"name":"asset.txt"}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "release", "upload", "astra/fjgo", "42", asset, "name=asset.txt", "--yes"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "name: asset.txt\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRemoteRepoParsing(t *testing.T) {
	for _, remote := range []string{
		"https://v15.next.forgejo.org/kavemand/.forgejo.git",
		"ssh://git@v15.next.forgejo.org/kavemand/.forgejo.git",
		"git@v15.next.forgejo.org:kavemand/.forgejo.git",
	} {
		ref, err := parseRemoteRepo(remote, defaultBaseURL)
		if err != nil {
			t.Fatalf("%s: %v", remote, err)
		}
		if ref.Owner != "kavemand" || ref.Repo != ".forgejo" {
			t.Fatalf("%s => %#v", remote, ref)
		}
	}
}

func TestRepoGetUsesRemoteRepoWhenOmitted(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Skipf("git init unavailable: %v", err)
	}
	cmd = exec.Command("git", "remote", "add", "origin", server.URL+"/astra/fjgo.git")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "-R", "origin", "repo", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "full_name: astra/fjgo") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestExplicitRepoContextFlagAndEnv(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "--repo", "astra/fjgo", "repo", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "full_name: astra/fjgo") {
		t.Fatalf("stdout = %s", stdout.String())
	}

	t.Setenv("FJGO_REPO", "astra/fjgo")
	stdout.Reset()
	stderr.Reset()
	err = run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "get"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "full_name: astra/fjgo") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestCommandLocalRepoAndHostContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/issues" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"number":1,"title":"Bug","state":"open","user":{"login":"alice"}}]`))
	}))
	defer server.Close()
	t.Setenv("FJGO_HOST", server.URL)

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"issue", "list", "--repo", "astra/fjgo", "--fields", "number,title,state,author"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "issues[1]{number,title,state,author}") || !strings.Contains(got, "1,Bug,open,alice") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestCommandLocalHostOverridesBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"repo", "get", "--host", server.URL, "--repo", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "full_name: astra/fjgo") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestExplicitBaseURLOverridesAmbientHost(t *testing.T) {
	explicit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("explicit path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"explicit"}`))
	}))
	defer explicit.Close()
	ambient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"ambient"}`))
	}))
	defer ambient.Close()
	t.Setenv("FJGO_HOST", ambient.URL)
	t.Setenv("FJGO_TOKEN", "secret")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", explicit.URL + "/api/v1", "version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "version: explicit\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestStructuredMissingRepoErrorSuggestsExplicitContext(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"repo", "get"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	got := stdout.String()
	for _, want := range []string{"code: USAGE", "--repo OWNER/REPO", "FJGO_REPO=OWNER/REPO", "-R origin"} {
		if !strings.Contains(got, want) {
			t.Fatalf("structured error missing %q:\n%s", want, got)
		}
	}
}

func TestHelpExamplesContract(t *testing.T) {
	helps := map[string]string{
		"root":     rootHelp(),
		"api":      apiHelp(),
		"alias":    aliasHelp(),
		"repo":     repoHelp(),
		"issue":    issueHelp(),
		"pr":       prHelp(),
		"run":      runHelp(),
		"workflow": workflowHelp(),
		"search":   searchHelp(),
		"label":    labelHelp(),
		"secret":   secretHelp(),
		"variable": variableHelp(),
		"release":  releaseHelp(),
		"setup":    setupHelp(),
	}
	for name, help := range helps {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(help, "examples:") {
				t.Fatalf("help missing examples section:\n%s", help)
			}
			examples := 0
			for _, line := range strings.Split(help[strings.Index(help, "examples:"):], "\n") {
				if strings.HasPrefix(line, "  fjgo ") || strings.HasPrefix(line, "  echo ") {
					examples++
				}
			}
			if examples < 2 {
				t.Fatalf("help has %d examples, want at least 2:\n%s", examples, help)
			}
		})
	}
}

func TestRepoLifecycleDryRuns(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "create",
			args: []string{"repo", "create", "demo", "--private", "--dry-run", "--yes"},
			want: []string{"operation: createCurrentUserRepo", "path: /user/repos", "private: true"},
		},
		{
			name: "edit",
			args: []string{"repo", "edit", "astra/fjgo", "--description", "updated", "--private", "false", "--dry-run", "--yes"},
			want: []string{"operation: repoEdit", "path: /repos/astra/fjgo", "private: false"},
		},
		{
			name: "branch-create",
			args: []string{"repo", "branches", "create", "astra/fjgo", "--name", "feature", "--from", "main", "--dry-run", "--yes"},
			want: []string{"operation: repoCreateBranch", "new_branch_name: feature", "old_ref_name: main"},
		},
		{
			name: "collaborator-add",
			args: []string{"repo", "collaborators", "add", "astra/fjgo", "alice", "--permission", "write", "--dry-run", "--yes"},
			want: []string{"operation: repoAddCollaborator", "collaborator", "permission: write"},
		},
		{
			name: "branch-protection-create",
			args: []string{"repo", "branch-protection", "create", "astra/fjgo", "--name", "main", "--required-approvals", "1", "--dry-run", "--yes"},
			want: []string{"operation: repoCreateBranchProtection", "rule_name: main", "required_approvals: 1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(t.Context(), tc.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
			}
			got := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("preview missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestReleaseExpandedDryRunsAndBodyFile(t *testing.T) {
	notes := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(notes, []byte("release notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "edit-body-file",
			args: []string{"release", "edit", "astra/fjgo", "7", "--body-file", notes, "--prerelease", "false", "--dry-run", "--yes"},
			want: []string{"operation: repoEditRelease", "body: release notes", "prerelease: false"},
		},
		{
			name: "create-notes-file",
			args: []string{"release", "create", "astra/fjgo", "v1.2.3", "--notes-file", notes, "--dry-run", "--yes"},
			want: []string{"operation: repoCreateRelease", "body: release notes"},
		},
		{
			name: "delete-by-tag",
			args: []string{"release", "delete", "astra/fjgo", "v1.2.3", "--dry-run", "--yes"},
			want: []string{"operation: repoDeleteReleaseByTag", "path: /repos/astra/fjgo/releases/tags/v1.2.3"},
		},
		{
			name: "asset-delete",
			args: []string{"release", "assets", "delete", "astra/fjgo", "7", "9", "--dry-run", "--yes"},
			want: []string{"operation: repoDeleteReleaseAttachment", "path: /repos/astra/fjgo/releases/7/assets/9"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(t.Context(), tc.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
			}
			got := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("preview missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestReleaseGenerateNotesIsUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"release", "create", "astra/fjgo", "v1.2.3", "--generate-notes", "--dry-run", "--yes"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown flag --generate-notes") {
		t.Fatalf("error = %q", err)
	}
}

func TestAdvancedIssuePRDryRunsAndDiff(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "dependency-add",
			args: []string{"issue", "dependencies", "add", "astra/fjgo", "42", "7", "--dry-run", "--yes"},
			want: []string{"operation: issueCreateIssueDependencies", "index: 7"},
		},
		{
			name: "reaction-add",
			args: []string{"issue", "reactions", "add", "astra/fjgo", "42", "+1", "--dry-run", "--yes"},
			want: []string{"operation: issuePostIssueReaction", `content: "+1"`},
		},
		{
			name: "deadline-clear",
			args: []string{"issue", "deadline", "clear", "astra/fjgo", "42", "--dry-run", "--yes"},
			want: []string{"operation: issueEditIssueDeadline", "due_date: null"},
		},
		{
			name: "time-add",
			args: []string{"issue", "time", "add", "astra/fjgo", "42", "--seconds", "900", "--dry-run", "--yes"},
			want: []string{"operation: issueAddTime", "time: 900"},
		},
		{
			name: "review-request",
			args: []string{"pr", "review-requests", "add", "astra/fjgo", "12", "--reviewer", "alice", "--dry-run", "--yes"},
			want: []string{"operation: repoCreatePullReviewRequests", "reviewers[1]: alice"},
		},
		{
			name: "review-comment",
			args: []string{"pr", "review-comment", "astra/fjgo", "12", "3", "--path", "main.go", "--new-line", "10", "--body", "note", "--dry-run", "--yes"},
			want: []string{"operation: repoCreatePullReviewComment", "path: main.go", "new_position: 10"},
		},
		{
			name: "pr-update",
			args: []string{"pr", "update", "astra/fjgo", "12", "--style", "rebase", "--dry-run", "--yes"},
			want: []string{"operation: repoUpdatePullRequest", "style[1]: rebase"},
		},
		{
			name: "pr-close",
			args: []string{"pr", "close", "astra/fjgo", "12", "--dry-run", "--yes"},
			want: []string{"operation: repoEditPullRequest", "state: closed"},
		},
		{
			name: "pr-edit",
			args: []string{"pr", "edit", "astra/fjgo", "12", "--title", "New title", "--base", "main", "--dry-run", "--yes"},
			want: []string{"operation: repoEditPullRequest", "title: New title", "base: main"},
		},
		{
			name: "pr-comment",
			args: []string{"pr", "comment", "astra/fjgo", "12", "--body", "ready", "--dry-run", "--yes"},
			want: []string{"operation: issueCreateComment", "body: ready"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(t.Context(), tc.args, &stdout, &stderr)
			if err != nil {
				t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
			}
			got := stdout.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("preview missing %q:\n%s", want, got)
				}
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/pulls/12.diff" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("diff --git a/main.go b/main.go\n"))
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "pr", "diff", "astra/fjgo", "12"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "diff:") || !strings.Contains(got, "diff --git") {
		t.Fatalf("diff output = %s", got)
	}
}

func TestPRViewWithReviewsAggregatesReviewSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo/pulls/12":
			_, _ = w.Write([]byte(`{"number":12,"title":"PR","state":"open","user":{"login":"alice"},"body":"body"}`))
		case "/api/v1/repos/astra/fjgo/pulls/12/reviews":
			_, _ = w.Write([]byte(`[{"id":1,"state":"APPROVED","user":{"login":"bob"},"comments_count":2}]`))
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "pr", "view", "astra/fjgo", "12", "--reviews"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"reviews: 1", "reviews[1]{id,state,author,comments,submitted}", "1,APPROVED,bob,2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("stdout missing %q:\n%s", want, got)
		}
	}
}

func TestAuthStatusDoesNotLeakToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "login": "astra", "email": "private@example.invalid", "login_name": "private-login"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "-token", "secret", "auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret") || strings.Contains(stdout.String(), "private@example.invalid") || strings.Contains(stdout.String(), "private-login") {
		t.Fatalf("auth status leaked token: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "token_present: true") || !strings.Contains(stdout.String(), "authenticated: true") || !strings.Contains(stdout.String(), "login: astra") {
		t.Fatalf("auth status = %s", stdout.String())
	}
}

func TestEnvTokenIgnoredForUnconfiguredDemoBase(t *testing.T) {
	t.Setenv("FJGO_TOKEN", "secret")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret") || !strings.Contains(stdout.String(), "token_present: false") {
		t.Fatalf("auth status = %s", stdout.String())
	}
}

func TestEnvTokenUsedWhenHostConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "astra"})
	}))
	defer server.Close()
	t.Setenv("FJGO_HOST", server.URL)
	t.Setenv("FJGO_TOKEN", "secret")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "authenticated: true") {
		t.Fatalf("auth status = %s", stdout.String())
	}
}

func TestLegacyBaseURLEnvIgnored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/version" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"version":"host"}`))
	}))
	defer server.Close()
	t.Setenv("FJGO_"+"BASE_URL", "https://example.invalid/api/v1")
	t.Setenv("FJGO_HOST", server.URL)

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "version: host") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestDoctorRedactsTokenAndSummarizesRepo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			if got := r.Header.Get("Authorization"); got != "token secret" {
				t.Fatalf("Authorization = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "login": "astra", "email": "private@example.invalid"})
		case "/api/v1/repos/astra/fjgo":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"full_name":         "astra/fjgo",
				"default_branch":    "main",
				"open_issues_count": 2,
				"open_pr_counter":   1,
				"topics":            []string{"forgejo", "cli"},
			})
		case "/api/v1/repos/astra/fjgo/releases":
			if r.URL.Query().Get("limit") != "5" {
				t.Fatalf("query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "tag_name": "v0.15.0", "name": "v0.15.0"}})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "-token", "secret", "doctor", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if strings.Contains(got, "secret") || strings.Contains(got, "private@example.invalid") {
		t.Fatalf("doctor leaked sensitive data: %s", got)
	}
	for _, want := range []string{"token_present: true", "full_name: astra/fjgo", "open_pr_count: 1", "tag_name: v0.15.0", "latest_release", "goos", "goarch", "go_version", "executable", "install_command: fjgo skill install", "status"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor missing %q:\n%s", want, got)
		}
	}
}

func TestSkillInstallCommand(t *testing.T) {
	dir := t.TempDir() + "/fjgo"
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"skill", "install", "--dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if _, err := os.Stat(dir + "/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "path: "+dir) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestInstallSkillsCommand(t *testing.T) {
	dir := t.TempDir() + "/fjgo"
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"install", "--skills", "--dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if _, err := os.Stat(dir + "/references/workflows.md"); err != nil {
		t.Fatal(err)
	}
}

func TestSkillStatusCommand(t *testing.T) {
	dir := t.TempDir() + "/fjgo"
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"skill", "status", "--dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "current: false") || !strings.Contains(stdout.String(), "SKILL.md") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestSkillGenerateCheckCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"skill", "generate", "--check"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "current: true") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestInstallSkillsCheckCommand(t *testing.T) {
	dir := t.TempDir() + "/fjgo"
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"install", "--skills", "--check", "--dir", dir}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "current: false") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestOpenCodePluginRegistersAmbientContextAndCapture(t *testing.T) {
	source := openCodePluginSource("/tmp/fjgo")
	for _, want := range []string{
		"experimental.chat.system.transform",
		"const captureArgs = [\"hook\", \"capture\"]",
		"process.once(\"beforeExit\"",
		"runFjgoCapture(directory)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("plugin source missing %q:\n%s", want, source)
		}
	}
}

func TestUpdateCheckReportsLatestReleaseAsset(t *testing.T) {
	assetName := fmt.Sprintf("fjgo_v9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases/latest" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v9.9.9",
			"assets": []map[string]any{{
				"name":                 assetName,
				"browser_download_url": server.URL + "/downloads/" + assetName,
				"size":                 123,
			}},
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"update", "--check", "--base-url", server.URL + "/api/v1", "--repo", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{"latest: v9.9.9", "update_available: true", "asset_url:", assetName} {
		if !strings.Contains(got, want) {
			t.Fatalf("update check missing %q:\n%s", want, got)
		}
	}
}

func TestSetupHooksCheckDoesNotWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FJGO_HOOK_HOME", home)

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"setup", "hooks", "--check"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, "status: check") || !strings.Contains(got, "Claude Code") || !strings.Contains(got, "OpenCode") {
		t.Fatalf("stdout = %s", got)
	}
	if _, err := os.Stat(home + "/.codex/hooks.json"); !os.IsNotExist(err) {
		t.Fatalf("check wrote hooks file: %v", err)
	}
}

func TestSetupHooksInstallsManagedFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FJGO_HOOK_HOME", home)

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"setup", "hooks"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	for _, path := range []string{
		home + "/.claude/settings.json",
		home + "/.codex/hooks.json",
		home + "/.codex/config.toml",
		home + "/.config/opencode/plugins/axi-fjgo.js",
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	codexConfig, err := os.ReadFile(home + "/.codex/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(codexConfig), "hooks = true") {
		t.Fatalf("codex config = %s", codexConfig)
	}
	plugin, err := os.ReadFile(home + "/.config/opencode/plugins/axi-fjgo.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plugin), "axi-sdk-js managed opencode plugin: fjgo") {
		t.Fatalf("plugin = %s", plugin)
	}
	stdout.Reset()
	err = run(t.Context(), []string{"setup", "hooks"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("second run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status: installed") {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestHookCaptureRecordsGitContext(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	t.Setenv("FJGO_HOOK_HOME", home)
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "agent@example.invalid"},
		{"config", "user.name", "Agent"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial"},
		{"remote", "add", "origin", "https://repos.example.invalid/astra/fjgo.git"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	var stdout bytes.Buffer
	if err := runHook([]string{"capture"}, &stdout); err != nil {
		t.Fatalf("hook capture: %v", err)
	}
	logPath := filepath.Join(home, ".local", "state", "fjgo", "sessions.log")
	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &record); err != nil {
		t.Fatalf("capture log is not JSON: %v\n%s", err, b)
	}
	for _, key := range []string{"time", "goos", "goarch", "cwd", "branch", "head", "repo", "dirty_files"} {
		if _, ok := record[key]; !ok {
			t.Fatalf("capture record missing %s: %#v", key, record)
		}
	}
	if record["cwd"] != repo || record["repo"] != "astra/fjgo" {
		t.Fatalf("capture record = %#v", record)
	}
	if got, ok := record["dirty_files"].(float64); !ok || got != 1 {
		t.Fatalf("dirty_files = %#v in %#v", record["dirty_files"], record)
	}
}

func TestRunCLIJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"--json", "api", "call", "repoGet", "owner=astra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, `"kind": "cli"`) || !strings.Contains(got, `"error"`) || !strings.Contains(got, "repoGet") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestRunCLIJSONErrorRedactsTokenAfterRootFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"-base-url", "https://example.invalid/api/v1", "-token", "secret-token", "--json", "api", "call", "repoGet", "owner=astra"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("code = %d", code)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	got := stdout.String()
	if !strings.Contains(got, `"kind": "cli"`) || strings.Contains(got, "secret-token") || !strings.Contains(got, "redacted") {
		t.Fatalf("stdout = %s", got)
	}
}

func TestErrorMessageFormatsAPIErrorJSON(t *testing.T) {
	err := errorMessage(forgejo.HTTPError{StatusCode: 401, Body: `{"message":"token is required","url":"https://example.invalid"}`})
	if err != "forgejo api: status 401: token is required (https://example.invalid)" {
		t.Fatalf("message = %q", err)
	}
}

func TestForgejoErrorCodeUsesStatusAndBodyPattern(t *testing.T) {
	cases := []struct {
		err  forgejo.HTTPError
		want string
	}{
		{err: forgejo.HTTPError{StatusCode: http.StatusUnauthorized, Body: "access token does not exist"}, want: "AUTH_TOKEN_INVALID"},
		{err: forgejo.HTTPError{StatusCode: http.StatusNotFound, Body: "repository does not exist"}, want: "REPO_NOT_FOUND"},
		{err: forgejo.HTTPError{StatusCode: http.StatusTooManyRequests, Body: "rate limit exceeded"}, want: "RATE_LIMITED"},
	}
	for _, tc := range cases {
		view := errorView([]string{"repo", "get"}, tc.err)
		if got := view["code"]; got != tc.want {
			t.Fatalf("%v code = %v, want %s", tc.err, got, tc.want)
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

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
	if got := stdout.String(); got != `{"version":"test"}` {
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
	if got := stdout.String(); got != `{"version":"test"}` {
		t.Fatalf("stdout = %q", got)
	}
}

func TestVersionFlagPrintsBinaryVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"--version"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "fjgo v0.11.0 none unknown\n" {
		t.Fatalf("stdout = %q", got)
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
	if strings.Contains(stderr.String(), "fjgo:") || strings.Contains(stderr.String(), "flag: help requested") {
		t.Fatalf("help printed error text:\n%s", stderr.String())
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
		"body_fields:",
		"name: string required",
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
		"query_params:",
		"name: string",
		"form_params:",
		"attachment: file",
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
		"query_params:",
		"q: string - keyword",
		"limit: integer",
		"private: boolean",
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
	if got := stdout.String(); got != `{"name":"asset.txt"}` {
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
	if got := stdout.String(); !strings.Contains(got, `"method": "POST"`) || !strings.Contains(got, `"auth_present": false`) {
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
	if got := stdout.String(); got != `{"full_name":"astra/demo"}` {
		t.Fatalf("stdout = %q", got)
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
		"name: string required",
		"private: bool",
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
	if got := stdout.String(); !strings.Contains(got, `"full_name": "astra/fjgo"`) {
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
	if got := stdout.String(); !strings.Contains(got, `"topics":`) {
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
	if !strings.Contains(got, `"operation": "repoUpdateTopics"`) || !strings.Contains(got, `"topics"`) {
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
	if !strings.Contains(got, `"operation": "repoUpdateAvatar"`) || strings.Contains(got, "cG5n") {
		t.Fatalf("preview = %s", got)
	}
}

func TestReleaseListAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v0.11.0"}]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "release", "list", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `"tag_name": "v0.11.0"`) {
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
	if got := stdout.String(); got != `{"tag_name":"v1.2.3"}` {
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
	if !strings.Contains(got, `"operation": "repoCreateRelease"`) || !strings.Contains(got, `"tag_name": "v1.2.3"`) {
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
	if got := stdout.String(); got != `[{"number":1}]` {
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
	if got := stdout.String(); got != `{"number":2}` {
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
		"path: /repos/{owner}/{repo}/issues/{index}",
		"args: owner/repo, index",
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
		"body_fields:",
		"title: string required",
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
	if got := stdout.String(); got != `{"name":"asset.txt"}` {
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
	if got := stdout.String(); !strings.Contains(got, `"full_name": "astra/fjgo"`) {
		t.Fatalf("stdout = %s", got)
	}
}

func TestAuthStatusDoesNotLeakToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "astra"})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "-token", "secret", "auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret") {
		t.Fatalf("auth status leaked token: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"token_present": true`) || !strings.Contains(stdout.String(), `"authenticated": true`) {
		t.Fatalf("auth status = %s", stdout.String())
	}
}

func TestEnvTokenIgnoredForUnconfiguredDemoBase(t *testing.T) {
	t.Setenv("FJGO_BASE_URL", "")
	t.Setenv("FJGO_TOKEN", "secret")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret") || !strings.Contains(stdout.String(), `"token_present": false`) {
		t.Fatalf("auth status = %s", stdout.String())
	}
}

func TestEnvTokenUsedWhenBaseURLConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"login": "astra"})
	}))
	defer server.Close()
	t.Setenv("FJGO_BASE_URL", server.URL+"/api/v1")
	t.Setenv("FJGO_TOKEN", "secret")

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"auth", "status"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"authenticated": true`) {
		t.Fatalf("auth status = %s", stdout.String())
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
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 7, "tag_name": "v0.11.0", "name": "v0.11.0"}})
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
	for _, want := range []string{`"token_present": true`, `"full_name": "astra/fjgo"`, `"open_pr_count": 1`, `"tag_name": "v0.11.0"`, `"latest_release"`, `"goos"`, `"goarch"`, `"go_version"`, `"executable"`, `"install_command": "fjgo skill install"`, `"status"`} {
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
	if !strings.Contains(stdout.String(), `"path": "`+dir+`"`) {
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
	if !strings.Contains(stdout.String(), `"installed": false`) || !strings.Contains(stdout.String(), `"SKILL.md"`) {
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
	if !strings.Contains(stdout.String(), `"installed": false`) {
		t.Fatalf("stdout = %s", stdout.String())
	}
}

func TestRunCLIJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"--json", "api", "call", "repoGet", "owner=astra"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s", stdout.String())
	}
	got := stderr.String()
	if !strings.Contains(got, `"kind": "cli"`) || !strings.Contains(got, `"error"`) || !strings.Contains(got, "repoGet") {
		t.Fatalf("stderr = %s", got)
	}
}

func TestRunCLIJSONErrorRedactsTokenAfterRootFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(t.Context(), []string{"-base-url", "https://example.invalid/api/v1", "-token", "secret-token", "--json", "api", "call", "repoGet", "owner=astra"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	got := stderr.String()
	if !strings.Contains(got, `"kind": "cli"`) || strings.Contains(got, "secret-token") || !strings.Contains(got, "redacted") {
		t.Fatalf("stderr = %s", got)
	}
}

func TestErrorMessageFormatsAPIErrorJSON(t *testing.T) {
	err := errorMessage(forgejo.HTTPError{StatusCode: 401, Body: `{"message":"token is required","url":"https://example.invalid"}`})
	if err != "forgejo api: status 401: token is required (https://example.invalid)" {
		t.Fatalf("message = %q", err)
	}
}

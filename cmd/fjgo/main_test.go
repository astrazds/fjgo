package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	if got := stdout.String(); got != "fjgo dev none unknown\n" {
		t.Fatalf("stdout = %q", got)
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
		"returns: Repository",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("inspect output missing %q:\n%s", want, got)
		}
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
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "repo", "topics", "astra/fjgo", "--set", "go,cli"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `"topics":`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestReleaseListAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/releases" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v0.1.0"}]`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	err := run(t.Context(), []string{"-base-url", server.URL + "/api/v1", "release", "list", "astra/fjgo"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("run error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `"tag_name": "v0.1.0"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestErrorMessageFormatsAPIErrorJSON(t *testing.T) {
	err := errorMessage(forgejo.HTTPError{StatusCode: 401, Body: `{"message":"token is required","url":"https://example.invalid"}`})
	if err != "forgejo api: status 401: token is required (https://example.invalid)" {
		t.Fatalf("message = %q", err)
	}
}

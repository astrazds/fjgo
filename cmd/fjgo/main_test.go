package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeSendsTokenAndDecodesUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(User{ID: 1, UserName: "astra"})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if user.UserName != "astra" {
		t.Fatalf("user_name = %q", user.UserName)
	}
}

func TestGetRawKeepsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.String(); got != "/api/v1/repos/search?q=fjgo" {
			t.Fatalf("url = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRaw(context.Background(), "/repos/search?q=fjgo"); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedRepoGetEscapesPathAndUsesOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.String(); got != "/api/v1/repos/astra/fjgo%2Fcli?ref=main" {
			t.Fatalf("url = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"full_name": "astra/fjgo"})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	out, err := client.RepoGet(context.Background(), "astra", "fjgo/cli", RequestOptions{
		Query: url.Values{"ref": {"main"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.FullName != "astra/fjgo" {
		t.Fatalf("repo = %#v", out)
	}
}

func TestGeneratedCreateCurrentUserRepoUsesTypedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/repos" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body CreateRepoOption
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Name != "fjgo" || !body.Private {
			t.Fatalf("body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(Repository{FullName: "astra/fjgo"})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := client.CreateCurrentUserRepo(context.Background(), &CreateRepoOption{
		Name:    "fjgo",
		Private: true,
	}, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if repo == nil || repo.FullName != "astra/fjgo" {
		t.Fatalf("repo = %#v", repo)
	}
}

func TestGeneratedOptionalBodyCanBeAbsentOrExplicitlyOverridden(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		body, _ := io.ReadAll(r.Body)
		switch requests {
		case 1:
			if len(body) != 0 {
				t.Fatalf("nil optional body = %q", body)
			}
		case 2:
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil || got["private"] != false {
				t.Fatalf("override body = %q, decoded = %#v, err = %v", body, got, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"demo"}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateCurrentUserRepo(t.Context(), nil, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateCurrentUserRepo(t.Context(), nil, RequestOptions{Body: map[string]any{"name": "demo", "private": false}}); err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedRepositoryModelSupportsParent(t *testing.T) {
	repo := Repository{
		FullName: "astra/fjgo",
		Parent:   &Repository{FullName: "astra/template"},
	}
	b, err := json.Marshal(repo)
	if err != nil {
		t.Fatal(err)
	}
	var got Repository
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Parent == nil || got.Parent.FullName != "astra/template" {
		t.Fatalf("parent = %#v", got.Parent)
	}
}

func TestHTTPErrorRedactsToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"bad secret-token","url":"https://forgejo.test/?token=secret-token"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "secret-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Me(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked token: %s", err)
	}
}

func TestReadLimitedBodyRejectsOversize(t *testing.T) {
	_, err := readLimitedBody(strings.NewReader("abcd"), 3)
	if !errors.Is(err, errResponseBodyTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestGeneratedAttachmentMethodUsesMultipart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/astra/fjgo/issues/42/assets" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.ContentLength != -1 {
			t.Fatalf("multipart body was buffered with Content-Length %d", r.ContentLength)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("Accept = %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		file, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if header.Filename != "note.txt" {
			t.Fatalf("filename = %q", header.Filename)
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(file); err != nil {
			t.Fatal(err)
		}
		if body.String() != "hello" {
			t.Fatalf("body = %q", body.String())
		}
		w.Header().Set("X-Upload", "complete")
		_ = json.NewEncoder(w).Encode(Attachment{ID: 7, Name: "note.txt"})
	}))
	defer server.Close()

	file := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var metadata ResponseMetadata
	attachment, err := client.IssueCreateIssueAttachment(t.Context(), "astra", "fjgo", "42", UploadPart{FilePath: file}, RequestOptions{Response: &metadata})
	if err != nil {
		t.Fatal(err)
	}
	if attachment == nil || attachment.ID != 7 {
		t.Fatalf("attachment = %#v", attachment)
	}
	if metadata.StatusCode != http.StatusOK || metadata.Header.Get("X-Upload") != "complete" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestClientSupportsBasicTOTPAndSudoAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "alice" || password != "secret-password" {
			t.Fatalf("basic auth = %q %q %t", username, password, ok)
		}
		if got := r.Header.Get("X-FORGEJO-OTP"); got != "123456" {
			t.Fatalf("OTP = %q", got)
		}
		if got := r.Header.Get("Sudo"); got != "bob" {
			t.Fatalf("Sudo = %q", got)
		}
		http.Error(w, "secret-password 123456", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClientWithAuth(server.URL+"/api/v1", AuthConfig{Username: "alice", Password: "secret-password", OTP: "123456", Sudo: "bob"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Me(t.Context())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-password") || strings.Contains(err.Error(), "123456") {
		t.Fatalf("error leaked credentials: %s", err)
	}
}

func TestDoOperationStreamWritesRawResponse(t *testing.T) {
	payload := []byte{0, 1, 2, 3, 255}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/octet-stream") {
			t.Fatalf("Accept = %q", got)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	op, _ := OperationByID("repoGetRawFile")
	var out bytes.Buffer
	response, err := client.DoOperationStream(t.Context(), op, map[string]string{"owner": "astra", "repo": "fjgo", "filepath": "asset.bin"}, RequestOptions{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), payload) || response.BytesWritten != int64(len(payload)) {
		t.Fatalf("stream = %v, response = %#v", out.Bytes(), response)
	}
}

func TestGeneratedTextMethodUsesRawBodyAndReturnsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "text/plain" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "text/html" {
			t.Fatalf("Accept = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "# heading" {
			t.Fatalf("body = %q", body)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<h1>heading</h1>")
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.RenderMarkdownRaw(t.Context(), "# heading", RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "<h1>heading</h1>" {
		t.Fatalf("result = %q", got)
	}
}

func TestGeneratedBinaryMethodReturnsBytesAndMetadata(t *testing.T) {
	payload := []byte("{}")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Archive", "ready")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	var metadata ResponseMetadata
	got, err := client.RepoGetArchive(t.Context(), "astra", "fjgo", "main.zip", RequestOptions{Response: &metadata})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("result = %v", got)
	}
	if metadata.StatusCode != http.StatusPartialContent || metadata.Header.Get("X-Archive") != "ready" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

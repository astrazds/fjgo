package forgejo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

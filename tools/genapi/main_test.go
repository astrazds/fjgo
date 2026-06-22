package main

import (
	"fmt"
	"testing"
)

func TestSuccessTypeResolvesResponseRef(t *testing.T) {
	got := successType(operation{
		Responses: map[string]response{
			"200": {Ref: "#/responses/Repository"},
		},
	}, map[string]response{
		"Repository": {Schema: schema{Ref: "#/definitions/Repository"}},
	})
	if got != "*Repository" {
		t.Fatalf("success type = %q", got)
	}
}

func TestSuccessTypeIgnoresEmptyResponse(t *testing.T) {
	got := successType(operation{
		Responses: map[string]response{
			"204": {Ref: "#/responses/empty"},
		},
	}, map[string]response{
		"empty": {},
	})
	if got != "" {
		t.Fatalf("success type = %q", got)
	}
}

func TestBodyTypeUsesBodySchema(t *testing.T) {
	got := bodyType([]parameter{
		{In: "path", Name: "owner"},
		{In: "body", Name: "body", Schema: schema{Ref: "#/definitions/CreateRepoOption"}},
	})
	if got != "*CreateRepoOption" {
		t.Fatalf("body type = %q", got)
	}
}

func TestAliasesFromEndpointsCoversObviousRepoGet(t *testing.T) {
	aliases := aliasesFromEndpoints([]endpoint{{
		Method:    "GET",
		Path:      "/repos/{owner}/{repo}/issues/{index}",
		Operation: "issueGetIssue",
		PathParams: []pathParam{
			{Name: "owner"},
			{Name: "repo"},
			{Name: "index"},
		},
	}})
	if len(aliases) != 1 {
		t.Fatalf("aliases = %#v", aliases)
	}
	got := aliases[0]
	if fmt.Sprint(got.Command) != "[repo issues get]" || fmt.Sprint(got.Args) != "[owner/repo index]" || got.Operation != "issueGetIssue" || got.Unsafe {
		t.Fatalf("alias = %#v", got)
	}
}

func TestAliasesFromEndpointsSkipsAwkwardOperations(t *testing.T) {
	aliases := aliasesFromEndpoints([]endpoint{
		{Method: "GET", Path: "/activitypub/actor", Operation: "activitypubInstanceActor"},
		{Method: "POST", Path: "/org/{org}/repos", Operation: "createOrgRepoDeprecated"},
		{Method: "POST", Path: "/repos/{owner}/{repo}/releases/{id}/assets", Operation: "repoCreateReleaseAttachment", Upload: true},
	})
	if len(aliases) != 0 {
		t.Fatalf("aliases = %#v", aliases)
	}
}

func TestHasUploadDetectsMultipart(t *testing.T) {
	if !hasUpload(operation{Consumes: []string{"multipart/form-data"}}) {
		t.Fatal("expected multipart upload")
	}
	if !hasUpload(operation{Parameters: []parameter{{In: "formData", Type: "file"}}}) {
		t.Fatal("expected formData upload")
	}
}

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

func TestOperationParamsCaptureQueryMetadata(t *testing.T) {
	got := operationParams([]parameter{
		{In: "path", Name: "owner", Type: "string", Required: true},
		{In: "query", Name: "limit", Type: "integer", Description: "page size"},
		{In: "query", Name: "q", Type: "string", Required: true, Description: "keyword"},
	}, "query")
	if len(got) != 2 {
		t.Fatalf("params = %#v", got)
	}
	if got[0].Name != "limit" || got[0].Type != "integer" || got[0].Required {
		t.Fatalf("limit param = %#v", got[0])
	}
	if got[1].Name != "q" || got[1].Type != "string" || !got[1].Required || got[1].Description != "keyword" {
		t.Fatalf("q param = %#v", got[1])
	}
}

func TestAliasesFromEndpointsCoversObviousRepoGet(t *testing.T) {
	aliases, collisions := aliasesFromEndpoints([]endpoint{{
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
	if len(collisions) != 0 {
		t.Fatalf("collisions = %#v", collisions)
	}
	got := aliases[0]
	if fmt.Sprint(got.Command) != "[repo issues get]" || fmt.Sprint(got.Args) != "[owner/repo index]" || got.Operation != "issueGetIssue" || got.Unsafe {
		t.Fatalf("alias = %#v", got)
	}
}

func TestAliasesFromEndpointsSkipsAwkwardOperations(t *testing.T) {
	aliases, collisions := aliasesFromEndpoints([]endpoint{
		{Method: "GET", Path: "/activitypub/actor", Operation: "activitypubInstanceActor"},
		{Method: "POST", Path: "/org/{org}/repos", Operation: "createOrgRepoDeprecated"},
		{Method: "POST", Path: "/repos/{owner}/{repo}/releases/{id}/assets", Operation: "repoCreateReleaseAttachment", Upload: true},
	})
	if len(aliases) != 0 {
		t.Fatalf("aliases = %#v", aliases)
	}
	if len(collisions) != 0 {
		t.Fatalf("collisions = %#v", collisions)
	}
}

func TestAliasesFromEndpointsRecordsDuplicateCommands(t *testing.T) {
	aliases, collisions := aliasesFromEndpoints([]endpoint{
		{Method: "GET", Path: "/repos/{owner}/{repo}/issues/{index}", Operation: "issueGetIssue", PathParams: []pathParam{{Name: "owner"}, {Name: "repo"}, {Name: "index"}}},
		{Method: "GET", Path: "/repos/{owner}/{repo}/issues/{number}", Operation: "issueGetIssueByNumber", PathParams: []pathParam{{Name: "owner"}, {Name: "repo"}, {Name: "number"}}},
	})
	if len(aliases) != 1 {
		t.Fatalf("aliases = %#v", aliases)
	}
	if len(collisions) != 1 {
		t.Fatalf("collisions = %#v", collisions)
	}
	if got := collisions[0]; fmt.Sprint(got.Command) != "[repo issues get]" || got.Kept != "issueGetIssue" || got.Skipped != "issueGetIssueByNumber" {
		t.Fatalf("collision = %#v", got)
	}
}

func TestAliasesFromEndpointsNamesDownloadVariantsExplicitly(t *testing.T) {
	aliases, collisions := aliasesFromEndpoints([]endpoint{
		{Method: "GET", Path: "/repos/{owner}/{repo}/pulls/{index}", Operation: "repoGetPullRequest", PathParams: []pathParam{{Name: "owner"}, {Name: "repo"}, {Name: "index"}}},
		{Method: "GET", Path: "/repos/{owner}/{repo}/pulls/{index}.{diffType}", Operation: "repoDownloadPullDiffOrPatch", PathParams: []pathParam{{Name: "owner"}, {Name: "repo"}, {Name: "index"}, {Name: "diffType"}}},
	})
	if len(collisions) != 0 {
		t.Fatalf("collisions = %#v", collisions)
	}
	seen := map[string]string{}
	for _, alias := range aliases {
		seen[fmt.Sprint(alias.Command)] = alias.Operation
	}
	if seen["[repo pulls get]"] != "repoGetPullRequest" {
		t.Fatalf("aliases = %#v", aliases)
	}
	if seen["[repo pulls download get]"] != "repoDownloadPullDiffOrPatch" {
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

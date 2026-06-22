package main

import "testing"

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

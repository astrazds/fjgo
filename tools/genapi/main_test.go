package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSuccessTypeResolvesResponseRef(t *testing.T) {
	got := successType(operation{
		Responses: map[string]response{
			"200": {Ref: "#/responses/Repository"},
		},
	}, map[string]response{
		"Repository": {Schema: schema{Ref: "#/definitions/Repository"}},
	}, nil)
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
	}, nil)
	if got != "" {
		t.Fatalf("success type = %q", got)
	}
}

func TestSuccessTypeUsesMediaTypeForUnschematizedResponses(t *testing.T) {
	op := operation{Responses: map[string]response{"200": {}}}
	if got := successType(op, nil, []string{"application/octet-stream"}); got != "[]byte" {
		t.Fatalf("binary success type = %q", got)
	}
	if got := successType(op, nil, []string{"text/plain"}); got != "string" {
		t.Fatalf("text success type = %q", got)
	}
	if got := successType(op, nil, []string{"application/json", "text/plain"}); got != "" {
		t.Fatalf("JSON success type = %q", got)
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

func TestGoTypePreservesUnsignedInt64(t *testing.T) {
	if got := goType(schema{Type: "integer", Format: "uint64"}); got != "uint64" {
		t.Fatalf("type = %q", got)
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

func TestOperationParamsCaptureConstraintsAndBodyRequirement(t *testing.T) {
	minimum := float64(1)
	params := []parameter{
		{In: "query", Name: "state", Type: "string", Enum: []any{"open", "closed"}, Default: []byte(`"open"`)},
		{In: "query", Name: "page", Type: "integer", Minimum: &minimum},
		{In: "query", Name: "labels", Type: "array", Items: &schema{Type: "string", Enum: []any{"bug", "feature"}}, CollectionFormat: "multi"},
		{In: "body", Name: "options", Required: true, Description: "request options", Schema: schema{Ref: "#/definitions/CreateRepoOption"}},
	}
	query := operationParams(params, "query")
	if len(query) != 3 || query[0].Name != "labels" || query[0].CollectionFormat != "multi" || fmt.Sprint(query[0].Enum) != "[bug feature]" {
		t.Fatalf("query metadata = %#v", query)
	}
	if query[1].Name != "page" || query[1].Minimum == nil || *query[1].Minimum != 1 {
		t.Fatalf("page metadata = %#v", query[1])
	}
	if query[2].Name != "state" || fmt.Sprint(query[2].Enum) != "[open closed]" || !query[2].HasDefault || query[2].Default != "open" {
		t.Fatalf("state metadata = %#v", query[2])
	}
	body := operationBodyParam(params)
	if body == nil || body.Name != "options" || body.Type != "CreateRepoOption" || !body.Required || body.Description != "request options" {
		t.Fatalf("body metadata = %#v", body)
	}
}

func TestOperationParamsPreserveTypedDefaults(t *testing.T) {
	params := operationParams([]parameter{{In: "query", Name: "enabled", Type: "boolean", Default: json.RawMessage(`false`)}}, "query")
	if len(params) != 1 || !params[0].HasDefault || params[0].Default != false {
		t.Fatalf("params = %#v", params)
	}
	var out bytes.Buffer
	writeOperationParamLiteral(&out, "QueryParams", params)
	if !strings.Contains(out.String(), "Default: false, HasDefault: true") {
		t.Fatalf("literal = %s", out.String())
	}
}

func TestAliasOmissionsExplainEveryUnaliasedOperation(t *testing.T) {
	endpoints := []endpoint{
		{Method: "GET", Path: "/version", Operation: "getVersion"},
		{Method: "POST", Path: "/repos/{owner}/{repo}/assets", Operation: "uploadAsset", Upload: true},
	}
	aliases, collisions := aliasesFromEndpoints(endpoints)
	omissions := aliasOmissions(endpoints, aliases, collisions)
	if len(omissions) != 2 || omissions[0].Operation != "getVersion" || omissions[1].Use != "fjgo api upload uploadAsset" {
		t.Fatalf("omissions = %#v", omissions)
	}
}

func TestEffectiveMediaTypesPreferOperationValues(t *testing.T) {
	global := []string{"application/json"}
	if got := effectiveMediaTypes([]string{"text/plain"}, global); fmt.Sprint(got) != "[text/plain]" {
		t.Fatalf("operation media = %#v", got)
	}
	got := effectiveMediaTypes(nil, global)
	got[0] = "changed"
	if global[0] != "application/json" {
		t.Fatalf("effective media aliases source: %#v", global)
	}
}

func TestOperationResponsesResolveSharedSchemasAndHeaders(t *testing.T) {
	got := operationResponses(operation{Responses: map[string]response{
		"200": {Ref: "#/responses/RepositoryList"},
	}}, map[string]response{
		"RepositoryList": {
			Description: "repositories",
			Schema:      schema{Type: "array", Items: &schema{Ref: "#/definitions/Repository"}},
			Headers:     map[string]responseHeader{"X-Total-Count": {Type: "integer", Format: "int64", Description: "total"}},
		},
	})
	if len(got) != 1 || got[0].Code != "200" || got[0].Type != "[]*Repository" || len(got[0].Headers) != 1 || got[0].Headers[0].Type != "integer:int64" {
		t.Fatalf("responses = %#v", got)
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
		{Method: "POST", Path: "/org/{org}/repos", Operation: "createOrgRepoDeprecated", Deprecated: true},
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

func TestReadLimitedSpecRejectsOversize(t *testing.T) {
	_, err := readLimitedSpec(strings.NewReader("abcd"), 3)
	if !errors.Is(err, errSpecTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateSpecDocumentRejectsUnsupportedSchemaFeatures(t *testing.T) {
	err := validateSpecDocument([]byte(`{"definitions":{"Thing":{"type":"array","uniqueItems":true}}}`))
	if err == nil || !strings.Contains(err.Error(), "uniqueness") {
		t.Fatalf("error = %v", err)
	}
	if err := validateSpecDocument([]byte(`{"definitions":{"Thing":{"type":"string","uniqueItems":true}}}`)); err != nil {
		t.Fatalf("invalid upstream string keyword should be ignored: %v", err)
	}
}

func TestValidateParsedSpecRejectsSilentCoverageLoss(t *testing.T) {
	s := spec{Paths: map[string]map[string]operation{
		"/things": {"head": {OperationID: "headThings"}},
	}}
	if err := validateParsedSpec(s); err == nil || !strings.Contains(err.Error(), "unsupported HTTP method") {
		t.Fatalf("error = %v", err)
	}
	s = spec{Paths: map[string]map[string]operation{
		"/things": {"get": {OperationID: "getThings", Parameters: []parameter{{In: "header", Name: "X-Mode"}}}},
	}}
	if err := validateParsedSpec(s); err == nil || !strings.Contains(err.Error(), "parameter location") {
		t.Fatalf("error = %v", err)
	}
}

func TestModelIndexPreservesExamples(t *testing.T) {
	var out bytes.Buffer
	writeModelIndex(&out, map[string]schema{
		"Thing": {Type: "object", Title: "A thing", Description: "Thing details", Properties: map[string]schema{
			"color":   {Type: "string", Format: "hex", Example: json.RawMessage(`"ff0000"`)},
			"enabled": {Type: "boolean", Example: json.RawMessage(`false`)},
			"scopes":  {Type: "array", Example: json.RawMessage(`["read","write"]`)},
		}},
	}, []string{"Thing"})
	if !strings.Contains(out.String(), `Example: "ff0000", HasExample: true`) ||
		!strings.Contains(out.String(), `Example: false, HasExample: true`) ||
		!strings.Contains(out.String(), `Example: []any{"read", "write"}, HasExample: true`) ||
		!strings.Contains(out.String(), `Title: "A thing", Description: "Thing details"`) ||
		!strings.Contains(out.String(), `Format: "hex"`) {
		t.Fatalf("model index = %s", out.String())
	}
}

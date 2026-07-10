package forgejo

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

type coverageSpec struct {
	Paths       map[string]map[string]coverageOperation `json:"paths"`
	Definitions map[string]json.RawMessage              `json:"definitions"`
	Consumes    []string                                `json:"consumes"`
	Produces    []string                                `json:"produces"`
	Responses   map[string]coverageResponse             `json:"responses"`
}

type coverageOperation struct {
	OperationID string                      `json:"operationId"`
	Summary     string                      `json:"summary"`
	Description string                      `json:"description"`
	Tags        []string                    `json:"tags"`
	Deprecated  bool                        `json:"deprecated"`
	Consumes    []string                    `json:"consumes"`
	Produces    []string                    `json:"produces"`
	Parameters  []coverageParameter         `json:"parameters"`
	Responses   map[string]coverageResponse `json:"responses"`
}

type coverageParameter struct {
	In       string   `json:"in"`
	Required bool     `json:"required"`
	Minimum  *float64 `json:"minimum"`
}

type coverageResponse struct {
	Ref     string                     `json:"$ref"`
	Headers map[string]json.RawMessage `json:"headers"`
}

type coverageDefinition struct {
	Title       string                    `json:"title"`
	Description string                    `json:"description"`
	Properties  map[string]coverageSchema `json:"properties"`
}

type coverageSchema struct {
	Format  string          `json:"format"`
	Default json.RawMessage `json:"default"`
	Example json.RawMessage `json:"example"`
}

func TestGeneratedCoverageExactlyMatchesPinnedSwagger(t *testing.T) {
	data, err := os.ReadFile("../../swagger.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec coverageSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	wantOperations := map[string]string{}
	wantMetadata := map[string]coverageOperation{}
	for path, methods := range spec.Paths {
		for method, operation := range methods {
			method = strings.ToUpper(method)
			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				continue
			}
			if operation.OperationID == "" {
				t.Fatalf("%s %s has no operationId", method, path)
			}
			if previous, exists := wantOperations[operation.OperationID]; exists {
				t.Fatalf("duplicate operationId %q for %s and %s %s", operation.OperationID, previous, method, path)
			}
			wantOperations[operation.OperationID] = method + " " + path
			wantMetadata[operation.OperationID] = operation
		}
	}
	gotOperations := map[string]string{}
	gotMetadata := map[string]Operation{}
	for _, operation := range Operations() {
		if previous, exists := gotOperations[operation.ID]; exists {
			t.Fatalf("duplicate generated operation %q for %s and %s %s", operation.ID, previous, operation.Method, operation.Path)
		}
		gotOperations[operation.ID] = operation.Method + " " + operation.Path
		gotMetadata[operation.ID] = operation
	}
	if len(gotOperations) != len(wantOperations) {
		t.Fatalf("generated operations = %d, Swagger operations = %d", len(gotOperations), len(wantOperations))
	}
	for id, want := range wantOperations {
		if got := gotOperations[id]; got != want {
			t.Errorf("operation %s = %q, want %q", id, got, want)
		}
		wantMeta := wantMetadata[id]
		gotMeta := gotMetadata[id]
		if gotMeta.Summary != coverageText(wantMeta.Summary) || gotMeta.Description != coverageText(wantMeta.Description) || gotMeta.Deprecated != wantMeta.Deprecated || !reflect.DeepEqual(gotMeta.Tags, wantMeta.Tags) {
			t.Errorf("operation %s metadata = %#v, want summary=%q description=%q deprecated=%t tags=%v", id, gotMeta, strings.TrimSpace(wantMeta.Summary), strings.TrimSpace(wantMeta.Description), wantMeta.Deprecated, wantMeta.Tags)
		}
		wantConsumes := wantMeta.Consumes
		if len(wantConsumes) == 0 {
			wantConsumes = spec.Consumes
		}
		wantProduces := wantMeta.Produces
		if len(wantProduces) == 0 {
			wantProduces = spec.Produces
		}
		if !reflect.DeepEqual(gotMeta.Consumes, wantConsumes) || !reflect.DeepEqual(gotMeta.Produces, wantProduces) {
			t.Errorf("operation %s media types = consumes %v produces %v, want %v and %v", id, gotMeta.Consumes, gotMeta.Produces, wantConsumes, wantProduces)
		}
	}
	wantParamCounts := map[string]int{}
	wantRequiredBodies, wantMinimums, wantResponses, wantResponseHeaders := 0, 0, 0, 0
	for _, operation := range wantMetadata {
		for _, parameter := range operation.Parameters {
			wantParamCounts[parameter.In]++
			if parameter.In == "body" && parameter.Required {
				wantRequiredBodies++
			}
			if parameter.Minimum != nil {
				wantMinimums++
			}
		}
		wantResponses += len(operation.Responses)
		for _, response := range operation.Responses {
			if response.Ref != "" {
				parts := strings.Split(response.Ref, "/")
				response = spec.Responses[parts[len(parts)-1]]
			}
			wantResponseHeaders += len(response.Headers)
		}
	}
	gotParamCounts := map[string]int{}
	gotRequiredBodies, gotMinimums, gotResponses, gotResponseHeaders := 0, 0, 0, 0
	for _, operation := range gotMetadata {
		gotParamCounts["path"] += len(operation.PathParamInfo)
		gotParamCounts["query"] += len(operation.QueryParams)
		gotParamCounts["formData"] += len(operation.FormParams)
		if operation.BodyParam != nil {
			gotParamCounts["body"]++
			if operation.BodyParam.Required {
				gotRequiredBodies++
			}
		}
		for _, params := range [][]OperationParam{operation.PathParamInfo, operation.QueryParams, operation.FormParams} {
			for _, parameter := range params {
				if parameter.HasMinimum {
					gotMinimums++
				}
			}
		}
		if operation.BodyParam != nil && operation.BodyParam.HasMinimum {
			gotMinimums++
		}
		gotResponses += len(operation.Responses)
		for _, response := range operation.Responses {
			gotResponseHeaders += len(response.Headers)
		}
	}
	if !reflect.DeepEqual(gotParamCounts, wantParamCounts) || gotRequiredBodies != wantRequiredBodies || gotMinimums != wantMinimums || gotResponses != wantResponses || gotResponseHeaders != wantResponseHeaders {
		t.Fatalf("generated metadata counts params=%v required_bodies=%d minimums=%d responses=%d response_headers=%d; want %v %d %d %d %d", gotParamCounts, gotRequiredBodies, gotMinimums, gotResponses, gotResponseHeaders, wantParamCounts, wantRequiredBodies, wantMinimums, wantResponses, wantResponseHeaders)
	}

	gotModels := map[string]Model{}
	for _, model := range Models() {
		gotModels[model.Name] = model
	}
	if len(gotModels) != len(spec.Definitions) {
		t.Fatalf("generated models = %d, Swagger definitions = %d", len(gotModels), len(spec.Definitions))
	}
	for name, rawDefinition := range spec.Definitions {
		model, ok := gotModels[name]
		if !ok {
			t.Errorf("missing generated model %s", name)
			continue
		}
		var definition coverageDefinition
		if err := json.Unmarshal(rawDefinition, &definition); err != nil {
			t.Fatal(err)
		}
		if model.Title != coverageText(definition.Title) || model.Description != coverageText(definition.Description) || len(model.Fields) != len(definition.Properties) {
			t.Errorf("model %s metadata = title %q description %q fields %d, want %q %q %d", name, model.Title, model.Description, len(model.Fields), definition.Title, definition.Description, len(definition.Properties))
		}
		fields := map[string]ModelField{}
		for _, field := range model.Fields {
			fields[field.Name] = field
		}
		for fieldName, property := range definition.Properties {
			field, exists := fields[fieldName]
			if !exists {
				t.Errorf("model %s missing field %s", name, fieldName)
				continue
			}
			if field.Format != property.Format || field.HasExample != (len(property.Example) != 0) {
				t.Errorf("model %s field %s format/example metadata = %q/%t, want %q/%t", name, fieldName, field.Format, field.HasExample, property.Format, len(property.Example) != 0)
			}
			if field.HasDefault != (len(property.Default) != 0) {
				t.Errorf("model %s field %s default presence = %t, want %t", name, fieldName, field.HasDefault, len(property.Default) != 0)
			}
			if len(property.Default) != 0 {
				var wantDefault any
				if err := json.Unmarshal(property.Default, &wantDefault); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(field.Default, wantDefault) {
					t.Errorf("model %s field %s default = %#v, want %#v", name, fieldName, field.Default, wantDefault)
				}
			}
			if len(property.Example) != 0 {
				var wantExample any
				if err := json.Unmarshal(property.Example, &wantExample); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(field.Example, wantExample) {
					t.Errorf("model %s field %s example = %#v, want %#v", name, fieldName, field.Example, wantExample)
				}
			}
		}
	}
}

func coverageText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func TestAliasCoverageExplainsEveryOperation(t *testing.T) {
	covered := map[string]string{}
	for _, alias := range Aliases() {
		if previous := covered[alias.Operation]; previous != "" {
			t.Fatalf("operation %s has multiple alias coverage entries: %s and alias", alias.Operation, previous)
		}
		covered[alias.Operation] = "alias"
	}
	for _, omission := range AliasOmissions() {
		if omission.Reason == "" || omission.Use == "" {
			t.Fatalf("unexplained alias omission: %#v", omission)
		}
		if previous := covered[omission.Operation]; previous != "" {
			t.Fatalf("operation %s has multiple alias coverage entries: %s and omission", omission.Operation, previous)
		}
		covered[omission.Operation] = "omission"
	}
	for _, operation := range Operations() {
		if covered[operation.ID] == "" {
			t.Errorf("operation %s has neither an alias nor an explained omission", operation.ID)
		}
	}
	if len(covered) != len(Operations()) {
		t.Fatalf("alias coverage entries = %d, operations = %d", len(covered), len(Operations()))
	}
}

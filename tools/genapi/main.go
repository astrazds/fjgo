package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const defaultSpecURL = "https://repos.astrazds.net/swagger.v1.json"

type spec struct {
	Paths       map[string]map[string]operation `json:"paths"`
	Definitions map[string]schema               `json:"definitions"`
	Responses   map[string]response             `json:"responses"`
}

type operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Consumes    []string            `json:"consumes"`
	Parameters  []parameter         `json:"parameters"`
	Responses   map[string]response `json:"responses"`
}

type parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Type        string  `json:"type"`
	Format      string  `json:"format"`
	Required    bool    `json:"required"`
	Description string  `json:"description"`
	Items       *schema `json:"items"`
	Schema      schema  `json:"schema"`
}

type schema struct {
	Ref                  string            `json:"$ref"`
	Type                 string            `json:"type"`
	Format               string            `json:"format"`
	Description          string            `json:"description"`
	Required             []string          `json:"required"`
	Properties           map[string]schema `json:"properties"`
	Items                *schema           `json:"items"`
	AdditionalProperties *schema           `json:"additionalProperties"`
	XGoName              string            `json:"x-go-name"`
}

type response struct {
	Ref    string `json:"$ref"`
	Schema schema `json:"schema"`
}

type endpoint struct {
	Method      string
	Path        string
	Operation   string
	FuncName    string
	Summary     string
	ReturnType  string
	BodyType    string
	Upload      bool
	PathParams  []pathParam
	QueryParams []operationParam
	FormParams  []operationParam
}

type operationParam struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

type alias struct {
	Command   []string
	Args      []string
	Operation string
	Unsafe    bool
}

type aliasCollision struct {
	Command []string
	Kept    string
	Skipped string
	Reason  string
}

type pathParam struct {
	Name  string
	Ident string
}

func main() {
	endpointsOut := flag.String("endpoints", "endpoints_gen.go", "endpoints output file")
	modelsOut := flag.String("models", "models_gen.go", "models output file")
	specPath := flag.String("spec", defaultSpecURL, "swagger JSON file or URL")
	flag.Parse()

	data, err := readSpec(*specPath)
	check(err)

	var s spec
	check(json.Unmarshal(data, &s))

	endpoints := endpointsFromSpec(s)
	var buf bytes.Buffer
	writeEndpointsFile(&buf, endpoints)

	formatted, err := format.Source(buf.Bytes())
	check(err)
	check(os.WriteFile(*endpointsOut, formatted, 0o644))

	buf.Reset()
	writeModelsFile(&buf, s.Definitions)
	formatted, err = format.Source(buf.Bytes())
	check(err)
	check(os.WriteFile(*modelsOut, formatted, 0o644))
}

func readSpec(path string) ([]byte, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := http.Get(path)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("fetch spec: %s", resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(path)
}

func endpointsFromSpec(s spec) []endpoint {
	var endpoints []endpoint
	for p, methods := range s.Paths {
		for method, op := range methods {
			if !isHTTPMethod(method) {
				continue
			}
			name := exportedName(op.OperationID)
			if name == "" {
				name = exportedName(method + " " + p)
			}
			endpoints = append(endpoints, endpoint{
				Method:      strings.ToUpper(method),
				Path:        p,
				Operation:   op.OperationID,
				FuncName:    name,
				Summary:     strings.TrimSpace(op.Summary),
				ReturnType:  successType(op, s.Responses),
				BodyType:    bodyType(op.Parameters),
				Upload:      hasUpload(op),
				PathParams:  pathParams(op.Parameters),
				QueryParams: operationParams(op.Parameters, "query"),
				FormParams:  operationParams(op.Parameters, "formData"),
			})
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].FuncName < endpoints[j].FuncName
	})
	dedupeNames(endpoints)
	return endpoints
}

func bodyType(params []parameter) string {
	for _, p := range params {
		if p.In == "body" {
			return goType(p.Schema)
		}
	}
	return ""
}

func hasUpload(op operation) bool {
	for _, consume := range op.Consumes {
		if strings.Contains(strings.ToLower(consume), "multipart/") {
			return true
		}
	}
	for _, p := range op.Parameters {
		if p.In == "formData" || p.Type == "file" {
			return true
		}
	}
	return false
}

func successType(op operation, responses map[string]response) string {
	codes := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		resp := op.Responses[code]
		if resp.Ref != "" {
			name := strings.TrimPrefix(resp.Ref, "#/responses/")
			resp = responses[name]
		}
		if resp.Schema.Ref != "" || resp.Schema.Type != "" || len(resp.Schema.Properties) > 0 {
			return goType(resp.Schema)
		}
	}
	return ""
}

func isHTTPMethod(s string) bool {
	switch strings.ToLower(s) {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

func pathParams(params []parameter) []pathParam {
	var names []pathParam
	for _, p := range params {
		if p.In == "path" {
			names = append(names, pathParam{Name: p.Name, Ident: safeIdent(p.Name)})
		}
	}
	return names
}

func operationParams(params []parameter, in string) []operationParam {
	var out []operationParam
	for _, p := range params {
		if p.In != in {
			continue
		}
		out = append(out, operationParam{
			Name:        p.Name,
			Type:        paramType(p),
			Required:    p.Required,
			Description: strings.TrimSpace(p.Description),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func paramType(p parameter) string {
	if p.Schema.Ref != "" || p.Schema.Type != "" || len(p.Schema.Properties) != 0 {
		return cleanType(goType(p.Schema))
	}
	if p.Type == "array" && p.Items != nil {
		return cleanType(goType(schema{Type: "array", Items: p.Items}))
	}
	if p.Type != "" {
		if p.Format != "" {
			return p.Type + ":" + p.Format
		}
		return p.Type
	}
	return "any"
}

func dedupeNames(endpoints []endpoint) {
	seen := map[string]int{}
	for i := range endpoints {
		seen[endpoints[i].FuncName]++
		if seen[endpoints[i].FuncName] > 1 {
			endpoints[i].FuncName = fmt.Sprintf("%s%d", endpoints[i].FuncName, seen[endpoints[i].FuncName])
		}
	}
}

func writeEndpointsFile(w io.Writer, endpoints []endpoint) {
	fmt.Fprintln(w, "// Code generated by tools/genapi; DO NOT EDIT.")
	fmt.Fprintln(w, "package forgejo")
	fmt.Fprintln(w)
	fmt.Fprintln(w, `import (
	"context"
	"net/http"
	"net/url"
	"strings"
)`)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func pathEscape(v string) string {")
	fmt.Fprintln(w, "\treturn url.PathEscape(v)")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "var operations = []Operation{")
	for _, e := range endpoints {
		operationID := e.Operation
		if operationID == "" {
			operationID = e.FuncName
		}
		fmt.Fprintf(w, "\t{ID: %q, Method: http.Method%s, Path: %q, Summary: %q, BodyType: %q, ReturnType: %q, Upload: %t, PathParams: []string{", operationID, methodConstSuffix(e.Method), e.Path, sanitizeComment(e.Summary), cleanType(e.BodyType), cleanType(e.ReturnType), e.Upload)
		for i, p := range e.PathParams {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%q", p.Name)
		}
		fmt.Fprint(w, "}")
		writeOperationParamLiteral(w, "QueryParams", e.QueryParams)
		writeOperationParamLiteral(w, "FormParams", e.FormParams)
		fmt.Fprintln(w, "},")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func Operations() []Operation {")
	fmt.Fprintln(w, "\tout := make([]Operation, len(operations))")
	fmt.Fprintln(w, "\tcopy(out, operations)")
	fmt.Fprintln(w, "\treturn out")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func OperationByID(id string) (Operation, bool) {")
	fmt.Fprintln(w, "\tfor _, op := range operations {")
	fmt.Fprintln(w, "\t\tif op.ID == id {")
	fmt.Fprintln(w, "\t\t\treturn op, true")
	fmt.Fprintln(w, "\t\t}")
	fmt.Fprintln(w, "\t}")
	fmt.Fprintln(w, "\treturn Operation{}, false")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	aliases, collisions := aliasesFromEndpoints(endpoints)
	writeAliases(w, aliases, collisions)
	fmt.Fprintln(w)
	for _, e := range endpoints {
		if e.Summary != "" {
			fmt.Fprintf(w, "// %s %s.\n", e.FuncName, sanitizeComment(e.Summary))
		}
		fmt.Fprintf(w, "func (c *Client) %s(ctx context.Context", e.FuncName)
		for _, p := range e.PathParams {
			fmt.Fprintf(w, ", %s string", p.Ident)
		}
		if e.BodyType != "" {
			fmt.Fprintf(w, ", body %s", e.BodyType)
		}
		if e.ReturnType == "" {
			fmt.Fprintln(w, ", opts RequestOptions) error {")
		} else {
			fmt.Fprintf(w, ", opts RequestOptions) (%s, error) {\n", e.ReturnType)
			fmt.Fprintf(w, "\tvar out %s\n", e.ReturnType)
			fmt.Fprintln(w, "\topts.Out = &out")
		}
		if e.BodyType != "" {
			fmt.Fprintln(w, "\topts.Body = body")
		}
		fmt.Fprintf(w, "\tapiPath := %q\n", e.Path)
		for _, p := range e.PathParams {
			fmt.Fprintf(w, "\tapiPath = strings.ReplaceAll(apiPath, %q, pathEscape(%s))\n", "{"+p.Name+"}", p.Ident)
		}
		if e.ReturnType == "" {
			fmt.Fprintf(w, "\treturn c.Do(ctx, http.Method%s, apiPath, opts)\n", methodConstSuffix(e.Method))
		} else {
			fmt.Fprintf(w, "\terr := c.Do(ctx, http.Method%s, apiPath, opts)\n", methodConstSuffix(e.Method))
			fmt.Fprintln(w, "\treturn out, err")
		}
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w)
	}
}

func writeOperationParamLiteral(w io.Writer, field string, params []operationParam) {
	if len(params) == 0 {
		return
	}
	fmt.Fprintf(w, ", %s: []OperationParam{", field)
	for _, p := range params {
		fmt.Fprintf(w, "{Name: %q, Type: %q, Required: %t, Description: %q}, ", p.Name, p.Type, p.Required, sanitizeComment(p.Description))
	}
	fmt.Fprint(w, "}")
}

func aliasesFromEndpoints(endpoints []endpoint) ([]alias, []aliasCollision) {
	var aliases []alias
	var collisions []aliasCollision
	seen := map[string]int{}
	for _, e := range endpoints {
		if e.Upload || strings.Contains(strings.ToLower(e.Operation), "deprecated") || strings.HasPrefix(strings.ToLower(e.Operation), "activitypub") {
			continue
		}
		command, args, ok := aliasCommand(e)
		if !ok {
			continue
		}
		key := strings.Join(command, "\x00")
		next := alias{Command: command, Args: args, Operation: operationID(e), Unsafe: e.Method != "GET"}
		if keptIndex, ok := seen[key]; ok {
			kept := aliases[keptIndex]
			if aliasPriority(e) > aliasPriority(endpointByOperation(endpoints, kept.Operation)) {
				collisions = append(collisions, aliasCollision{
					Command: command,
					Kept:    next.Operation,
					Skipped: kept.Operation,
					Reason:  "duplicate generated command; higher priority operation kept",
				})
				aliases[keptIndex] = next
				continue
			}
			collisions = append(collisions, aliasCollision{
				Command: command,
				Kept:    kept.Operation,
				Skipped: next.Operation,
				Reason:  "duplicate generated command",
			})
			continue
		}
		seen[key] = len(aliases)
		aliases = append(aliases, next)
	}
	sort.Slice(aliases, func(i, j int) bool {
		return strings.Join(aliases[i].Command, " ") < strings.Join(aliases[j].Command, " ")
	})
	sort.Slice(collisions, func(i, j int) bool {
		return strings.Join(collisions[i].Command, " ") < strings.Join(collisions[j].Command, " ")
	})
	return aliases, collisions
}

func endpointByOperation(endpoints []endpoint, operation string) endpoint {
	for _, e := range endpoints {
		if operationID(e) == operation {
			return e
		}
	}
	return endpoint{Operation: operation}
}

func aliasPriority(e endpoint) int {
	score := 100
	op := strings.ToLower(e.Operation)
	path := strings.ToLower(e.Path)
	if strings.Contains(op, "download") || strings.Contains(path, ".{") {
		score -= 50
	}
	if strings.Contains(op, "all") {
		score -= 10
	}
	if strings.Contains(op, "list") || strings.Contains(op, "search") {
		score += 5
	}
	if strings.Contains(op, "get") {
		score += 3
	}
	return score
}

func aliasCommand(e endpoint) ([]string, []string, bool) {
	parts := strings.Split(strings.Trim(e.Path, "/"), "/")
	if len(parts) == 0 {
		return nil, nil, false
	}
	switch parts[0] {
	case "repos":
		if len(parts) < 3 || parts[1] != "{owner}" || parts[2] != "{repo}" {
			return nil, nil, false
		}
		cmd := append([]string{"repo"}, aliasTail(e, parts[3:])...)
		return cmd, aliasArgs(e, "owner/repo", e.PathParams[2:]), true
	case "orgs":
		if len(parts) > 1 && parts[1] == "{org}" {
			cmd := append([]string{"org"}, aliasTail(e, parts[2:])...)
			return cmd, aliasArgs(e, "org", e.PathParams[1:]), true
		}
		cmd := append([]string{"org"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "user":
		cmd := append([]string{"user"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "users":
		if len(parts) > 1 && parts[1] == "{username}" {
			cmd := append([]string{"user"}, aliasTail(e, parts[2:])...)
			return cmd, aliasArgs(e, "username", e.PathParams[1:]), true
		}
		cmd := append([]string{"user"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "admin":
		cmd := append([]string{"admin"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "packages":
		cmd := append([]string{"package"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "notifications":
		cmd := append([]string{"notification"}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	case "licenses", "gitignore", "label":
		cmd := append([]string{parts[0]}, aliasTail(e, parts[1:])...)
		return cmd, aliasArgs(e, "", e.PathParams), true
	default:
		return nil, nil, false
	}
}

func aliasTail(e endpoint, parts []string) []string {
	var out []string
	for _, part := range parts {
		if strings.Contains(part, ".{") {
			out = append(out, "download")
			continue
		}
		if strings.HasPrefix(part, "{") || part == "-" {
			continue
		}
		out = append(out, aliasWord(part))
	}
	verb := aliasVerb(e.Method, e.Operation)
	if len(out) == 0 && strings.Contains(strings.ToLower(e.Operation), "all") {
		out = append(out, "all")
	}
	if len(out) == 0 || out[len(out)-1] != verb {
		out = append(out, verb)
	}
	return out
}

func aliasVerb(method, operation string) string {
	op := strings.ToLower(operation)
	switch method {
	case "GET":
		switch {
		case strings.Contains(op, "search"):
			return "search"
		case strings.Contains(op, "list") || strings.Contains(op, "all"):
			return "list"
		default:
			return "get"
		}
	case "POST":
		switch {
		case strings.Contains(op, "create"):
			return "create"
		case strings.Contains(op, "add"):
			return "add"
		case strings.Contains(op, "run"):
			return "run"
		case strings.Contains(op, "accept"):
			return "accept"
		case strings.Contains(op, "reject"):
			return "reject"
		case strings.Contains(op, "rename"):
			return "rename"
		case strings.Contains(op, "update"):
			return "update"
		case strings.Contains(op, "link"):
			return "link"
		case strings.Contains(op, "unlink"):
			return "unlink"
		default:
			return "post"
		}
	case "PUT":
		switch {
		case strings.Contains(op, "add"):
			return "add"
		case strings.Contains(op, "set"):
			return "set"
		case strings.Contains(op, "update"):
			return "update"
		default:
			return "put"
		}
	case "PATCH":
		return "edit"
	case "DELETE":
		switch {
		case strings.Contains(op, "remove"):
			return "remove"
		default:
			return "delete"
		}
	default:
		return strings.ToLower(method)
	}
}

func aliasArgs(e endpoint, first string, rest []pathParam) []string {
	var args []string
	if first != "" {
		args = append(args, first)
	}
	for _, p := range rest {
		args = append(args, p.Name)
	}
	return args
}

func aliasWord(s string) string {
	s = strings.Trim(s, "{}")
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ToLower(s)
}

func operationID(e endpoint) string {
	if e.Operation != "" {
		return e.Operation
	}
	return e.FuncName
}

func writeAliases(w io.Writer, aliases []alias, collisions []aliasCollision) {
	fmt.Fprintln(w, "var aliases = []Alias{")
	for _, a := range aliases {
		fmt.Fprint(w, "\t{Command: []string{")
		for i, part := range a.Command {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%q", part)
		}
		fmt.Fprint(w, "}, Args: []string{")
		for i, arg := range a.Args {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%q", arg)
		}
		fmt.Fprintf(w, "}, Operation: %q, Unsafe: %t},\n", a.Operation, a.Unsafe)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func Aliases() []Alias {")
	fmt.Fprintln(w, "\tout := make([]Alias, len(aliases))")
	fmt.Fprintln(w, "\tcopy(out, aliases)")
	fmt.Fprintln(w, "\treturn out")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "var aliasCollisions = []AliasCollision{")
	for _, c := range collisions {
		fmt.Fprint(w, "\t{Command: []string{")
		for i, part := range c.Command {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%q", part)
		}
		fmt.Fprintf(w, "}, Kept: %q, Skipped: %q, Reason: %q},\n", c.Kept, c.Skipped, c.Reason)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func AliasCollisions() []AliasCollision {")
	fmt.Fprintln(w, "\tout := make([]AliasCollision, len(aliasCollisions))")
	fmt.Fprintln(w, "\tcopy(out, aliasCollisions)")
	fmt.Fprintln(w, "\treturn out")
	fmt.Fprintln(w, "}")
}

func writeModelsFile(w io.Writer, definitions map[string]schema) {
	fmt.Fprintln(w, "// Code generated by tools/genapi; DO NOT EDIT.")
	fmt.Fprintln(w, "package forgejo")
	fmt.Fprintln(w)
	names := make([]string, 0, len(definitions))
	for name := range definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := definitions[name]
		typeName := exportedName(name)
		if def.Description != "" {
			fmt.Fprintf(w, "// %s %s.\n", typeName, sanitizeComment(def.Description))
		}
		switch {
		case def.Type == "object" || len(def.Properties) > 0:
			writeStruct(w, typeName, def)
		default:
			fmt.Fprintf(w, "type %s %s\n\n", typeName, goType(def))
		}
	}
	writeModelIndex(w, definitions, names)
}

func writeModelIndex(w io.Writer, definitions map[string]schema, names []string) {
	fmt.Fprintln(w, "var models = []Model{")
	for _, name := range names {
		def := definitions[name]
		typeName := exportedName(name)
		fmt.Fprintf(w, "\t{Name: %q, Fields: []ModelField{", typeName)
		if len(def.Properties) != 0 {
			required := map[string]bool{}
			for _, name := range def.Required {
				required[name] = true
			}
			propNames := make([]string, 0, len(def.Properties))
			for name := range def.Properties {
				propNames = append(propNames, name)
			}
			sort.Strings(propNames)
			for _, propName := range propNames {
				prop := def.Properties[propName]
				fmt.Fprintf(w, "{Name: %q, Type: %q, Required: %t}, ", propName, cleanType(goType(prop)), required[propName])
			}
		}
		fmt.Fprintln(w, "}},")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func Models() []Model {")
	fmt.Fprintln(w, "\tout := make([]Model, len(models))")
	fmt.Fprintln(w, "\tcopy(out, models)")
	fmt.Fprintln(w, "\treturn out")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func ModelByName(name string) (Model, bool) {")
	fmt.Fprintln(w, "\tfor _, model := range models {")
	fmt.Fprintln(w, "\t\tif model.Name == name {")
	fmt.Fprintln(w, "\t\t\treturn model, true")
	fmt.Fprintln(w, "\t\t}")
	fmt.Fprintln(w, "\t}")
	fmt.Fprintln(w, "\treturn Model{}, false")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
}

func writeStruct(w io.Writer, typeName string, def schema) {
	if len(def.Properties) == 0 {
		if def.AdditionalProperties != nil {
			fmt.Fprintf(w, "type %s map[string]%s\n\n", typeName, goType(*def.AdditionalProperties))
			return
		}
		fmt.Fprintf(w, "type %s map[string]any\n\n", typeName)
		return
	}
	fmt.Fprintf(w, "type %s struct {\n", typeName)
	names := make([]string, 0, len(def.Properties))
	for name := range def.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	used := map[string]int{}
	for _, jsonName := range names {
		prop := def.Properties[jsonName]
		fieldName := prop.XGoName
		if fieldName == "" {
			fieldName = exportedName(jsonName)
		}
		if fieldName == "" {
			fieldName = "Field"
		}
		used[fieldName]++
		if used[fieldName] > 1 {
			fieldName = fmt.Sprintf("%s%d", fieldName, used[fieldName])
		}
		if prop.Description != "" {
			fmt.Fprintf(w, "\t// %s\n", sanitizeComment(prop.Description))
		}
		fmt.Fprintf(w, "\t%s %s `json:%q`\n", fieldName, goType(prop), jsonName+",omitempty")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
}

func goType(s schema) string {
	if s.Ref != "" {
		return "*" + exportedName(strings.TrimPrefix(s.Ref, "#/definitions/"))
	}
	switch s.Type {
	case "array":
		if s.Items == nil {
			return "[]any"
		}
		return "[]" + goType(*s.Items)
	case "boolean":
		return "bool"
	case "integer":
		if s.Format == "int64" {
			return "int64"
		}
		return "int"
	case "number":
		return "float64"
	case "object":
		if s.AdditionalProperties != nil {
			return "map[string]" + goType(*s.AdditionalProperties)
		}
		return "map[string]any"
	case "string":
		return "string"
	default:
		if len(s.Properties) > 0 {
			return "map[string]any"
		}
		return "any"
	}
}

func cleanType(s string) string {
	return strings.TrimPrefix(s, "*")
}

func methodConstSuffix(method string) string {
	switch method {
	case "GET":
		return "Get"
	case "POST":
		return "Post"
	case "PUT":
		return "Put"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	default:
		panic(method)
	}
}

var wordRE = regexp.MustCompile(`[A-Za-z0-9]+`)

func exportedName(s string) string {
	words := wordRE.FindAllString(s, -1)
	var b strings.Builder
	for _, word := range words {
		for i, r := range word {
			if i == 0 {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func safeIdent(s string) string {
	words := wordRE.FindAllString(s, -1)
	if len(words) == 0 {
		return "value"
	}
	var b strings.Builder
	for i, word := range words {
		for j, r := range word {
			if i == 0 && j == 0 {
				b.WriteRune(unicode.ToLower(r))
			} else if j == 0 {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	out := b.String()
	switch out {
	case "type", "func", "map", "range", "var":
		return out + "Value"
	default:
		return out
	}
}

func sanitizeComment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "*/", "")
	return strings.TrimSpace(s)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

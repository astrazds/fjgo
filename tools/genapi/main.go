package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const defaultSpecURL = "https://v15.next.forgejo.org/swagger.v1.json"
const maxSpecBytes int64 = 32 << 20

var (
	errSpecTooLarge = errors.New("spec response body too large")
	specHTTPClient  = &http.Client{Timeout: 30 * time.Second}
)

type spec struct {
	Paths       map[string]map[string]operation `json:"paths"`
	Definitions map[string]schema               `json:"definitions"`
	Responses   map[string]response             `json:"responses"`
	Consumes    []string                        `json:"consumes"`
	Produces    []string                        `json:"produces"`
}

type operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary"`
	Description string              `json:"description"`
	Consumes    []string            `json:"consumes"`
	Parameters  []parameter         `json:"parameters"`
	Responses   map[string]response `json:"responses"`
	Produces    []string            `json:"produces"`
	Tags        []string            `json:"tags"`
	Deprecated  bool                `json:"deprecated"`
}

type parameter struct {
	Name             string          `json:"name"`
	In               string          `json:"in"`
	Type             string          `json:"type"`
	Format           string          `json:"format"`
	Required         bool            `json:"required"`
	Description      string          `json:"description"`
	Items            *schema         `json:"items"`
	Schema           schema          `json:"schema"`
	Enum             []any           `json:"enum"`
	Default          json.RawMessage `json:"default"`
	CollectionFormat string          `json:"collectionFormat"`
	Minimum          *float64        `json:"minimum"`
}

type schema struct {
	Ref                  string            `json:"$ref"`
	Type                 string            `json:"type"`
	Format               string            `json:"format"`
	Description          string            `json:"description"`
	Title                string            `json:"title"`
	Required             []string          `json:"required"`
	Properties           map[string]schema `json:"properties"`
	Items                *schema           `json:"items"`
	AdditionalProperties *schema           `json:"additionalProperties"`
	XGoName              string            `json:"x-go-name"`
	Enum                 []any             `json:"enum"`
	Default              json.RawMessage   `json:"default"`
	Example              json.RawMessage   `json:"example"`
	Minimum              *float64          `json:"minimum"`
}

type response struct {
	Ref         string                    `json:"$ref"`
	Description string                    `json:"description"`
	Schema      schema                    `json:"schema"`
	Headers     map[string]responseHeader `json:"headers"`
}

type responseHeader struct {
	Type        string  `json:"type"`
	Format      string  `json:"format"`
	Description string  `json:"description"`
	Items       *schema `json:"items"`
}

type endpoint struct {
	Method        string
	Path          string
	Operation     string
	FuncName      string
	Summary       string
	Description   string
	ReturnType    string
	BodyType      string
	BodyParam     *operationParam
	Upload        bool
	PathParams    []pathParam
	PathParamInfo []operationParam
	QueryParams   []operationParam
	FormParams    []operationParam
	Consumes      []string
	Produces      []string
	Responses     []operationResponse
	Tags          []string
	Deprecated    bool
}

type operationResponse struct {
	Code        string
	Type        string
	Description string
	Headers     []operationParam
}

type operationParam struct {
	Name             string
	Type             string
	Required         bool
	Description      string
	Enum             []string
	Default          any
	HasDefault       bool
	CollectionFormat string
	Minimum          *float64
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

type aliasOmission struct {
	Operation string
	Reason    string
	Use       string
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
	check(validateSpecDocument(data))

	var s spec
	check(json.Unmarshal(data, &s))
	check(validateParsedSpec(s))

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

func validateParsedSpec(s spec) error {
	seen := map[string]string{}
	for path, methods := range s.Paths {
		for method, op := range methods {
			if !isHTTPMethod(method) {
				if method != "parameters" && !strings.HasPrefix(strings.ToLower(method), "x-") {
					return fmt.Errorf("unsupported HTTP method %q for %s", method, path)
				}
				continue
			}
			if op.OperationID == "" {
				return fmt.Errorf("missing operationId for %s %s", strings.ToUpper(method), path)
			}
			if previous := seen[op.OperationID]; previous != "" {
				return fmt.Errorf("duplicate operationId %q for %s and %s %s", op.OperationID, previous, strings.ToUpper(method), path)
			}
			seen[op.OperationID] = strings.ToUpper(method) + " " + path
			for _, parameter := range op.Parameters {
				switch parameter.In {
				case "path", "query", "body", "formData":
				default:
					return fmt.Errorf("unsupported parameter location %q for %s", parameter.In, op.OperationID)
				}
			}
		}
	}
	return nil
}

func validateSpecDocument(data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	unsupported := map[string]bool{
		"allOf": true, "anyOf": true, "oneOf": true, "not": true,
		"maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
		"minLength": true, "maxLength": true, "pattern": true,
		"minItems": true, "maxItems": true, "multipleOf": true,
	}
	var walk func(any, string) error
	walk = func(value any, path string) error {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if unsupported[key] {
					return fmt.Errorf("unsupported Swagger keyword %q at %s; extend the generator before updating the pinned spec", key, path)
				}
				if key == "uniqueItems" && child == true && value["type"] == "array" {
					return fmt.Errorf("unsupported array uniqueness constraint at %s; extend the generator before updating the pinned spec", path)
				}
				if err := walk(child, path+"/"+key); err != nil {
					return err
				}
			}
		case []any:
			for i, child := range value {
				if err := walk(child, fmt.Sprintf("%s/%d", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(document, "#")
}

func readSpec(path string) ([]byte, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := specHTTPClient.Get(path)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("fetch spec: %s", resp.Status)
		}
		return readLimitedSpec(resp.Body, maxSpecBytes)
	}
	return os.ReadFile(path)
}

func readLimitedSpec(r io.Reader, max int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, r, max+1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n > max {
		return nil, errSpecTooLarge
	}
	return buf.Bytes(), nil
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
			produces := effectiveMediaTypes(op.Produces, s.Produces)
			endpoints = append(endpoints, endpoint{
				Method:        strings.ToUpper(method),
				Path:          p,
				Operation:     op.OperationID,
				FuncName:      name,
				Summary:       strings.TrimSpace(op.Summary),
				Description:   strings.TrimSpace(op.Description),
				ReturnType:    successType(op, s.Responses, produces),
				BodyType:      bodyType(op.Parameters),
				BodyParam:     operationBodyParam(op.Parameters),
				Upload:        hasUpload(op),
				PathParams:    pathParams(op.Parameters),
				PathParamInfo: operationParams(op.Parameters, "path"),
				QueryParams:   operationParams(op.Parameters, "query"),
				FormParams:    operationParams(op.Parameters, "formData"),
				Consumes:      effectiveMediaTypes(op.Consumes, s.Consumes),
				Produces:      produces,
				Responses:     operationResponses(op, s.Responses),
				Tags:          slices.Clone(op.Tags),
				Deprecated:    op.Deprecated,
			})
		}
	}
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].FuncName < endpoints[j].FuncName
	})
	dedupeNames(endpoints)
	return endpoints
}

func operationResponses(op operation, responses map[string]response) []operationResponse {
	codes := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]operationResponse, 0, len(codes))
	for _, code := range codes {
		value := op.Responses[code]
		if value.Ref != "" {
			value = responses[strings.TrimPrefix(value.Ref, "#/responses/")]
		}
		headers := make([]operationParam, 0, len(value.Headers))
		for name, header := range value.Headers {
			typeName := header.Type
			if header.Type == "array" && header.Items != nil {
				typeName = cleanType(goType(schema{Type: "array", Items: header.Items}))
			} else if header.Format != "" {
				typeName += ":" + header.Format
			}
			headers = append(headers, operationParam{Name: name, Type: typeName, Description: strings.TrimSpace(header.Description)})
		}
		sort.Slice(headers, func(i, j int) bool { return headers[i].Name < headers[j].Name })
		responseType := ""
		if value.Schema.Ref != "" || value.Schema.Type != "" || len(value.Schema.Properties) != 0 {
			responseType = cleanType(goType(value.Schema))
		}
		out = append(out, operationResponse{
			Code:        code,
			Type:        responseType,
			Description: strings.TrimSpace(value.Description),
			Headers:     headers,
		})
	}
	return out
}

func effectiveMediaTypes(operation, global []string) []string {
	if len(operation) != 0 {
		return slices.Clone(operation)
	}
	return slices.Clone(global)
}

func bodyType(params []parameter) string {
	for _, p := range params {
		if p.In == "body" {
			return goType(p.Schema)
		}
	}
	return ""
}

func operationBodyParam(params []parameter) *operationParam {
	values := operationParams(params, "body")
	if len(values) == 0 {
		return nil
	}
	return &values[0]
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

func successType(op operation, responses map[string]response, produces []string) string {
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
	for _, mediaType := range produces {
		if strings.Contains(strings.ToLower(mediaType), "json") {
			return ""
		}
	}
	for _, mediaType := range produces {
		mediaType = strings.ToLower(mediaType)
		if strings.HasPrefix(mediaType, "text/") {
			return "string"
		}
		if strings.Contains(mediaType, "octet-stream") || strings.Contains(mediaType, "zip") || strings.Contains(mediaType, "gzip") {
			return "[]byte"
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
			Name:             p.Name,
			Type:             paramType(p),
			Required:         p.Required,
			Description:      strings.TrimSpace(p.Description),
			Enum:             parameterEnum(p),
			Default:          specValue(p.Default),
			HasDefault:       len(p.Default) != 0,
			CollectionFormat: parameterCollectionFormat(p),
			Minimum:          p.Minimum,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func parameterEnum(p parameter) []string {
	if len(p.Enum) != 0 {
		return specValues(p.Enum)
	}
	if p.Type == "array" && p.Items != nil {
		return specValues(p.Items.Enum)
	}
	return nil
}

func parameterCollectionFormat(p parameter) string {
	if p.Type != "array" {
		return ""
	}
	if p.CollectionFormat != "" {
		return p.CollectionFormat
	}
	return "csv"
}

func specValues(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch x := value.(type) {
		case string:
			out = append(out, x)
		default:
			b, _ := json.Marshal(x)
			out = append(out, string(b))
		}
	}
	return out
}

func specValue(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(value, &decoded) != nil {
		return nil
	}
	return decoded
}

func specGoLiteral(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "nil"
	}
	return goValueLiteral(value)
}

func goValueLiteral(value any) string {
	switch value := value.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(value)
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64)
	case string:
		return fmt.Sprintf("%q", value)
	case []any:
		parts := make([]string, len(value))
		for i, item := range value {
			parts[i] = goValueLiteral(item)
		}
		return "[]any{" + strings.Join(parts, ", ") + "}"
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%q: %s", key, goValueLiteral(value[key])))
		}
		return "map[string]any{" + strings.Join(parts, ", ") + "}"
	default:
		return "nil"
	}
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
		fmt.Fprintf(w, "\t{ID: %q, Method: http.Method%s, Path: %q, Summary: %q, Description: %q, BodyType: %q, ReturnType: %q, Upload: %t, Deprecated: %t", operationID, methodConstSuffix(e.Method), e.Path, sanitizeComment(e.Summary), sanitizeComment(e.Description), cleanType(e.BodyType), cleanType(e.ReturnType), e.Upload, e.Deprecated)
		writeStringSliceLiteral(w, "Tags", e.Tags)
		writeStringSliceLiteral(w, "Consumes", e.Consumes)
		writeStringSliceLiteral(w, "Produces", e.Produces)
		writeOperationParamPointerLiteral(w, "BodyParam", e.BodyParam)
		fmt.Fprint(w, ", PathParams: []string{")
		for i, p := range e.PathParams {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%q", p.Name)
		}
		fmt.Fprint(w, "}")
		writeOperationParamLiteral(w, "QueryParams", e.QueryParams)
		writeOperationParamLiteral(w, "FormParams", e.FormParams)
		writeOperationParamLiteral(w, "PathParamInfo", e.PathParamInfo)
		writeOperationResponseLiteral(w, e.Responses)
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
	writeAliases(w, aliases, collisions, aliasOmissions(endpoints, aliases, collisions))
	fmt.Fprintln(w)
	for _, e := range endpoints {
		if e.Upload {
			writeUploadMethod(w, e)
			continue
		}
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
			fmt.Fprintln(w, "\tif opts.Body == nil && opts.RawBody == nil {")
			if cleanType(e.BodyType) == "string" && firstNonJSONMediaType(e.Consumes) != "" {
				fmt.Fprintln(w, "\t\topts.RawBody = []byte(body)")
			} else if strings.HasPrefix(e.BodyType, "*") {
				fmt.Fprintln(w, "\t\tif body != nil { opts.Body = body }")
			} else {
				fmt.Fprintln(w, "\t\topts.Body = body")
			}
			fmt.Fprintln(w, "\t}")
		}
		writeGeneratedMediaTypes(w, e)
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

func writeOperationResponseLiteral(w io.Writer, responses []operationResponse) {
	if len(responses) == 0 {
		return
	}
	fmt.Fprint(w, ", Responses: []OperationResponse{")
	for _, response := range responses {
		fmt.Fprintf(w, "{Code: %q, Type: %q, Description: %q", response.Code, response.Type, sanitizeComment(response.Description))
		writeOperationParamLiteral(w, "Headers", response.Headers)
		fmt.Fprint(w, "}, ")
	}
	fmt.Fprint(w, "}")
}

func firstNonJSONMediaType(mediaTypes []string) string {
	for _, mediaType := range mediaTypes {
		if !strings.Contains(strings.ToLower(mediaType), "json") {
			return mediaType
		}
	}
	return ""
}

func writeGeneratedMediaTypes(w io.Writer, e endpoint) {
	if len(e.Consumes) != 0 {
		fmt.Fprintf(w, "\tif opts.ContentType == \"\" && (opts.Body != nil || opts.RawBody != nil) { opts.ContentType = %q }\n", e.Consumes[0])
	}
	if len(e.Produces) != 0 {
		fmt.Fprintf(w, "\tif opts.Accept == \"\" { opts.Accept = %q }\n", strings.Join(e.Produces, ", "))
	}
}

func writeUploadMethod(w io.Writer, e endpoint) {
	if e.Summary != "" {
		fmt.Fprintf(w, "// %s %s.\n", e.FuncName, sanitizeComment(e.Summary))
	}
	fmt.Fprintf(w, "func (c *Client) %s(ctx context.Context", e.FuncName)
	for _, p := range e.PathParams {
		fmt.Fprintf(w, ", %s string", p.Ident)
	}
	used := map[string]int{}
	for _, p := range e.FormParams {
		ident := safeIdent(p.Name)
		used[ident]++
		if used[ident] > 1 {
			ident = fmt.Sprintf("%s%d", ident, used[ident])
		}
		paramType := "string"
		if p.Type == "file" {
			paramType = "UploadPart"
		}
		fmt.Fprintf(w, ", %s %s", ident, paramType)
	}
	if e.ReturnType == "" {
		fmt.Fprintln(w, ", opts RequestOptions) error {")
	} else {
		fmt.Fprintf(w, ", opts RequestOptions) (%s, error) {\n", e.ReturnType)
		fmt.Fprintf(w, "\tvar out %s\n", e.ReturnType)
	}
	fmt.Fprintln(w, "\tfields := map[string]string{}")
	fmt.Fprintln(w, "\tvar files []UploadPart")
	used = map[string]int{}
	for _, p := range e.FormParams {
		ident := safeIdent(p.Name)
		used[ident]++
		if used[ident] > 1 {
			ident = fmt.Sprintf("%s%d", ident, used[ident])
		}
		if p.Type == "file" {
			if p.Required {
				fmt.Fprintf(w, "\t%s.FieldName = %q\n", ident, p.Name)
				fmt.Fprintf(w, "\tfiles = append(files, %s)\n", ident)
			} else {
				fmt.Fprintf(w, "\tif %s.FilePath != \"\" {\n", ident)
				fmt.Fprintf(w, "\t\t%s.FieldName = %q\n", ident, p.Name)
				fmt.Fprintf(w, "\t\tfiles = append(files, %s)\n", ident)
				fmt.Fprintln(w, "\t}")
			}
			continue
		}
		if p.Required {
			fmt.Fprintf(w, "\tfields[%q] = %s\n", p.Name, ident)
		} else {
			fmt.Fprintf(w, "\tif %s != \"\" { fields[%q] = %s }\n", ident, p.Name, ident)
		}
	}
	writeGeneratedMediaTypes(w, e)
	fmt.Fprintf(w, "\tapiPath := %q\n", e.Path)
	for _, p := range e.PathParams {
		fmt.Fprintf(w, "\tapiPath = strings.ReplaceAll(apiPath, %q, pathEscape(%s))\n", "{"+p.Name+"}", p.Ident)
	}
	if e.ReturnType == "" {
		fmt.Fprintln(w, "\t_, err := c.DoMultipartWithOptions(ctx, http.Method"+methodConstSuffix(e.Method)+", apiPath, fields, files, opts, nil)")
		fmt.Fprintln(w, "\treturn err")
	} else {
		fmt.Fprintln(w, "\t_, err := c.DoMultipartWithOptions(ctx, http.Method"+methodConstSuffix(e.Method)+", apiPath, fields, files, opts, &out)")
		fmt.Fprintln(w, "\treturn out, err")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
}

func writeOperationParamLiteral(w io.Writer, field string, params []operationParam) {
	if len(params) == 0 {
		return
	}
	fmt.Fprintf(w, ", %s: []OperationParam{", field)
	for _, p := range params {
		fmt.Fprintf(w, "{Name: %q, Type: %q, Required: %t, Description: %q", p.Name, p.Type, p.Required, sanitizeComment(p.Description))
		if len(p.Enum) != 0 {
			fmt.Fprint(w, ", Enum: []string{")
			for _, value := range p.Enum {
				fmt.Fprintf(w, "%q, ", value)
			}
			fmt.Fprint(w, "}")
		}
		if p.HasDefault {
			fmt.Fprintf(w, ", Default: %s, HasDefault: true", goValueLiteral(p.Default))
		}
		if p.CollectionFormat != "" {
			fmt.Fprintf(w, ", CollectionFormat: %q", p.CollectionFormat)
		}
		if p.Minimum != nil {
			fmt.Fprintf(w, ", Minimum: %s, HasMinimum: true", strconv.FormatFloat(*p.Minimum, 'g', -1, 64))
		}
		fmt.Fprint(w, "}, ")
	}
	fmt.Fprint(w, "}")
}

func writeOperationParamPointerLiteral(w io.Writer, field string, param *operationParam) {
	if param == nil {
		return
	}
	fmt.Fprintf(w, ", %s: &OperationParam{", field)
	writeOperationParamFields(w, *param)
	fmt.Fprint(w, "}")
}

func writeOperationParamFields(w io.Writer, p operationParam) {
	fmt.Fprintf(w, "Name: %q, Type: %q, Required: %t, Description: %q", p.Name, p.Type, p.Required, sanitizeComment(p.Description))
	if len(p.Enum) != 0 {
		fmt.Fprint(w, ", Enum: []string{")
		for _, value := range p.Enum {
			fmt.Fprintf(w, "%q, ", value)
		}
		fmt.Fprint(w, "}")
	}
	if p.HasDefault {
		fmt.Fprintf(w, ", Default: %s, HasDefault: true", goValueLiteral(p.Default))
	}
	if p.CollectionFormat != "" {
		fmt.Fprintf(w, ", CollectionFormat: %q", p.CollectionFormat)
	}
	if p.Minimum != nil {
		fmt.Fprintf(w, ", Minimum: %s, HasMinimum: true", strconv.FormatFloat(*p.Minimum, 'g', -1, 64))
	}
}

func writeStringSliceLiteral(w io.Writer, field string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(w, ", %s: []string{", field)
	for _, value := range values {
		fmt.Fprintf(w, "%q, ", value)
	}
	fmt.Fprint(w, "}")
}

func aliasesFromEndpoints(endpoints []endpoint) ([]alias, []aliasCollision) {
	var aliases []alias
	var collisions []aliasCollision
	seen := map[string]int{}
	for _, e := range endpoints {
		if e.Upload || e.Deprecated || strings.HasPrefix(strings.ToLower(e.Operation), "activitypub") {
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

func aliasOmissions(endpoints []endpoint, aliases []alias, collisions []aliasCollision) []aliasOmission {
	covered := make(map[string]bool, len(aliases))
	for _, item := range aliases {
		covered[item.Operation] = true
	}
	collided := make(map[string]bool, len(collisions))
	for _, collision := range collisions {
		collided[collision.Skipped] = true
	}
	omissions := make([]aliasOmission, 0, len(endpoints)-len(aliases))
	for _, e := range endpoints {
		operation := operationID(e)
		if covered[operation] {
			continue
		}
		item := aliasOmission{Operation: operation, Use: "fjgo api call " + operation}
		switch {
		case e.Upload:
			item.Reason = "multipart operation uses the explicit upload surface"
			item.Use = "fjgo api upload " + operation
		case e.Deprecated:
			item.Reason = "deprecated operation is intentionally not promoted"
		case strings.HasPrefix(strings.ToLower(operation), "activitypub"):
			item.Reason = "ActivityPub operation is intentionally kept on the generic surface"
		case collided[operation]:
			item.Reason = "generated command collides with a higher-priority operation"
		default:
			item.Reason = "path shape has no unambiguous generated command"
		}
		omissions = append(omissions, item)
	}
	sort.Slice(omissions, func(i, j int) bool { return omissions[i].Operation < omissions[j].Operation })
	return omissions
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

func writeAliases(w io.Writer, aliases []alias, collisions []aliasCollision, omissions []aliasOmission) {
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
	fmt.Fprintln(w)
	fmt.Fprintln(w, "var aliasOmissions = []AliasOmission{")
	for _, omission := range omissions {
		fmt.Fprintf(w, "\t{Operation: %q, Reason: %q, Use: %q},\n", omission.Operation, omission.Reason, omission.Use)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "func AliasOmissions() []AliasOmission {")
	fmt.Fprintln(w, "\tout := make([]AliasOmission, len(aliasOmissions))")
	fmt.Fprintln(w, "\tcopy(out, aliasOmissions)")
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
		typeDescription := def.Description
		if typeDescription == "" {
			typeDescription = def.Title
		}
		if typeDescription != "" {
			fmt.Fprintf(w, "// %s %s.\n", typeName, sanitizeComment(typeDescription))
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
		fmt.Fprintf(w, "\t{Name: %q, Title: %q, Description: %q, AdditionalProperties: %t, Fields: []ModelField{", typeName, sanitizeComment(def.Title), sanitizeComment(def.Description), def.AdditionalProperties != nil || (def.Type == "object" && len(def.Properties) == 0))
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
				fmt.Fprintf(w, "{Name: %q, Type: %q, Format: %q, Required: %t, Description: %q", propName, cleanType(goType(prop)), prop.Format, required[propName], sanitizeComment(prop.Description))
				enum := schemaEnum(prop)
				if len(enum) != 0 {
					fmt.Fprint(w, ", Enum: []string{")
					for _, value := range enum {
						fmt.Fprintf(w, "%q, ", value)
					}
					fmt.Fprint(w, "}")
				}
				if len(prop.Default) != 0 {
					fmt.Fprintf(w, ", Default: %s, HasDefault: true", specGoLiteral(prop.Default))
				}
				if len(prop.Example) != 0 {
					fmt.Fprintf(w, ", Example: %s, HasExample: true", specGoLiteral(prop.Example))
				}
				if prop.Minimum != nil {
					fmt.Fprintf(w, ", Minimum: %s, HasMinimum: true", strconv.FormatFloat(*prop.Minimum, 'g', -1, 64))
				}
				fmt.Fprint(w, "}, ")
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

func schemaEnum(value schema) []string {
	if len(value.Enum) != 0 {
		return specValues(value.Enum)
	}
	if value.Type == "array" && value.Items != nil {
		return specValues(value.Items.Enum)
	}
	return nil
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
		if s.Format == "uint64" {
			return "uint64"
		}
		return "int"
	case "number":
		return "float64"
	case "object":
		if s.AdditionalProperties != nil {
			return "map[string]" + goType(*s.AdditionalProperties)
		}
		return "map[string]any"
	case "file":
		return "[]byte"
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

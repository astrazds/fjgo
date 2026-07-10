package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"repos.astrazds.net/astrazds/fjgo/internal/forgejo"
)

const maxRequestBodyBytes int64 = 32 << 20

type apiBodyInput struct {
	JSON        any
	Raw         []byte
	ContentType string
	Accept      string
}

func apiBodyPreview(input apiBodyInput) any {
	if input.Raw == nil {
		return input.JSON
	}
	return truncateString(string(input.Raw), defaultTruncateChars, "body", &truncateReport{})
}

func takeAPIBodyInput(args []string) ([]string, apiBodyInput, error) {
	args, jsonSource, err := takeBodyFlag(args)
	if err != nil {
		return nil, apiBodyInput{}, err
	}
	args, rawSource, rawSet, err := takeValueFlag(args, "-body-raw")
	if err != nil {
		return nil, apiBodyInput{}, err
	}
	args, contentType, _, err := takeValueFlag(args, "--content-type")
	if err != nil {
		return nil, apiBodyInput{}, err
	}
	args, accept, _, err := takeValueFlag(args, "--accept")
	if err != nil {
		return nil, apiBodyInput{}, err
	}
	if jsonSource != "" && rawSet {
		return nil, apiBodyInput{}, newUsageError("use only one request body source: -body or -body-raw")
	}
	input := apiBodyInput{ContentType: contentType, Accept: accept}
	if jsonSource != "" {
		input.JSON, err = readJSONBody(jsonSource)
		if err != nil {
			return nil, apiBodyInput{}, err
		}
	}
	if rawSet {
		input.Raw, err = readBoundedInput(rawSource, maxRequestBodyBytes)
		if err != nil {
			return nil, apiBodyInput{}, err
		}
	}
	return args, input, nil
}

func readBoundedInput(source string, limit int64) ([]byte, error) {
	var reader io.Reader
	var file *os.File
	switch {
	case source == "-":
		reader = os.Stdin
	case strings.HasPrefix(source, "@"):
		var err error
		file, err = os.Open(strings.TrimPrefix(source, "@"))
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	default:
		reader = strings.NewReader(source)
	}
	b, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("request body exceeds %d bytes", limit)
	}
	return b, nil
}

func takeAPIOutput(args []string) ([]string, string, error) {
	args, output, outputSet, err := takeValueFlag(args, "--output")
	if err != nil {
		return nil, "", err
	}
	args, raw := boolFlag(args, "--raw")
	if raw && outputSet {
		return nil, "", newUsageError("use only one raw response destination: --raw or --output <path>")
	}
	if raw {
		return args, "-", nil
	}
	return args, output, nil
}

func takeIncludeResponse(args []string) ([]string, bool) {
	return boolFlag(args, "--include-response")
}

func writeAPIOK(w io.Writer, jsonOut bool) error {
	result := map[string]any{"result": "ok"}
	if jsonOut {
		return writeJSON(w, result)
	}
	return writeTOON(w, result)
}

func writeAPIBody(w io.Writer, raw []byte, mediaHint string, jsonOut, full bool) error {
	if len(raw) == 0 {
		return writeAPIOK(w, jsonOut)
	}
	body, encoding := decodedResponseBody(raw, mediaHint)
	if !jsonOut && encoding == "json" {
		return writeJSONAsTOON(w, raw, "result", full)
	}
	if jsonOut && encoding == "json" && json.Valid(raw) {
		_, err := w.Write(raw)
		return err
	}
	if !jsonOut {
		view := map[string]any{"result": body, "encoding": encoding}
		var report truncateReport
		if !full {
			view = truncateLargeStrings(view, defaultTruncateChars, "result", &report).(map[string]any)
		}
		blocks := toonBlocks{view}
		if report.Truncated() {
			blocks = append(blocks, helpBlock([]string{
				fmt.Sprintf("Run `fjgo <same command> --full` to show complete truncated fields (%s)", strings.Join(report.Paths, ", ")),
			}))
		}
		return writeTOON(w, blocks)
	}
	return writeJSON(w, map[string]any{"result": body, "encoding": encoding})
}

func writeAPIResponse(w io.Writer, response forgejo.RawResponse, mediaHint string, jsonOut, full bool) error {
	body, encoding := decodedResponseBody(response.Body, mediaHint)
	view := map[string]any{
		"status":  response.StatusCode,
		"headers": safeResponseHeaders(response.Header),
		"body":    body,
	}
	if encoding != "json" {
		view["body_encoding"] = encoding
	}
	if jsonOut {
		return writeJSON(w, view)
	}
	var report truncateReport
	if !full {
		view = truncateLargeStrings(view, defaultTruncateChars, "response", &report).(map[string]any)
	}
	blocks := toonBlocks{map[string]any{"response": view}}
	if report.Truncated() {
		blocks = append(blocks, helpBlock([]string{
			fmt.Sprintf("Run `fjgo <same command> --full` to show complete truncated fields (%s)", strings.Join(report.Paths, ", ")),
		}))
	}
	return writeTOON(w, blocks)
}

func decodedResponseBody(raw []byte, mediaHint string) (any, string) {
	if len(raw) == 0 {
		return nil, "json"
	}
	mediaHint = strings.ToLower(mediaHint)
	if strings.Contains(mediaHint, "octet-stream") || strings.Contains(mediaHint, "application/zip") || strings.Contains(mediaHint, "application/gzip") {
		return base64.StdEncoding.EncodeToString(raw), "base64"
	}
	if strings.HasPrefix(mediaHint, "text/") {
		if utf8.Valid(raw) {
			return string(raw), "utf-8"
		}
		return base64.StdEncoding.EncodeToString(raw), "base64"
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err == nil {
		var trailing any
		if err := dec.Decode(&trailing); err == io.EOF {
			return value, "json"
		}
	}
	if utf8.Valid(raw) {
		return string(raw), "utf-8"
	}
	return base64.StdEncoding.EncodeToString(raw), "base64"
}

func safeResponseHeaders(header http.Header) http.Header {
	out := header.Clone()
	for _, name := range []string{"Authorization", "Proxy-Authorization", "Set-Cookie"} {
		if _, ok := out[http.CanonicalHeaderKey(name)]; ok {
			out[http.CanonicalHeaderKey(name)] = []string{"redacted"}
		}
	}
	return out
}

func streamAPIDestination(stdout io.Writer, destination string, stream func(io.Writer) (forgejo.StreamResponse, error)) error {
	if destination == "-" {
		_, err := stream(stdout)
		return err
	}
	dir := filepath.Dir(destination)
	base := filepath.Base(destination)
	tmp, err := os.CreateTemp(dir, "."+base+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	response, err := stream(tmp)
	if err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return err
	}
	committed = true
	return writeTOON(stdout, map[string]any{
		"output": map[string]any{
			"path":   destination,
			"bytes":  response.BytesWritten,
			"status": response.StatusCode,
		},
	})
}

package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxResponseBodyBytes int64 = 32 << 20

var errResponseBodyTooLarge = errors.New("response body too large")

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

type RequestOptions struct {
	Query url.Values
	Body  any
	Out   any
}

type RawResponse struct {
	Body       []byte
	StatusCode int
	Header     http.Header
}

type OperationParam struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

type Operation struct {
	ID          string           `json:"id"`
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	Summary     string           `json:"summary,omitempty"`
	BodyType    string           `json:"body,omitempty"`
	ReturnType  string           `json:"returns,omitempty"`
	Upload      bool             `json:"upload,omitempty"`
	PathParams  []string         `json:"path_params,omitempty"`
	QueryParams []OperationParam `json:"query_params,omitempty"`
	FormParams  []OperationParam `json:"form_params,omitempty"`
}

type Alias struct {
	Command   []string `json:"command"`
	Args      []string `json:"args,omitempty"`
	Operation string   `json:"operation"`
	Unsafe    bool     `json:"unsafe,omitempty"`
}

type AliasCollision struct {
	Command []string `json:"command"`
	Kept    string   `json:"kept"`
	Skipped string   `json:"skipped"`
	Reason  string   `json:"reason"`
}

type Model struct {
	Name   string       `json:"name"`
	Fields []ModelField `json:"fields,omitempty"`
}

type ModelField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

type UploadPart struct {
	FieldName string `json:"field_name"`
	FilePath  string `json:"file_path"`
	FileName  string `json:"file_name,omitempty"`
}

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("forgejo api: status %d: %s", e.StatusCode, e.Body)
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("base url must be absolute: %q", baseURL)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: u, token: token, http: httpClient}, nil
}

func (c *Client) HasToken() bool {
	return c.token != ""
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var user User
	return user, c.doJSON(ctx, http.MethodGet, "/user", nil, &user)
}

func (c *Client) GetRaw(ctx context.Context, apiPath string) ([]byte, error) {
	resp, err := c.GetRawResponse(ctx, apiPath)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) GetRawResponse(ctx context.Context, apiPath string) (RawResponse, error) {
	req, err := c.newRequest(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return RawResponse{}, err
	}
	return c.doResponse(req)
}

func (c *Client) GetExternalRawResponse(ctx context.Context, rawURL string) (RawResponse, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return RawResponse{}, err
	}
	if u.Scheme == "" || u.Host == "" {
		return RawResponse{}, fmt.Errorf("download URL must be absolute: %q", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return RawResponse{}, err
	}
	req.Header.Set("Accept", "application/octet-stream, text/plain, application/json")
	if c.token != "" && strings.EqualFold(u.Host, c.baseURL.Host) {
		req.Header.Set("Authorization", "token "+c.token)
	}
	return c.doResponse(req)
}

func (c *Client) Do(ctx context.Context, method, apiPath string, opts RequestOptions) error {
	apiPath = withQuery(apiPath, opts.Query)
	return c.doJSON(ctx, method, apiPath, opts.Body, opts.Out)
}

func (c *Client) DoRaw(ctx context.Context, method, apiPath string, opts RequestOptions) ([]byte, error) {
	resp, err := c.DoRawResponse(ctx, method, apiPath, opts)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) DoRawResponse(ctx context.Context, method, apiPath string, opts RequestOptions) (RawResponse, error) {
	var body io.Reader
	if opts.Body != nil {
		b, err := json.Marshal(opts.Body)
		if err != nil {
			return RawResponse{}, err
		}
		body = bytes.NewReader(b)
	}
	req, err := c.newRequest(ctx, method, withQuery(apiPath, opts.Query), body)
	if err != nil {
		return RawResponse{}, err
	}
	if opts.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.doResponse(req)
}

func (c *Client) DoMultipart(ctx context.Context, method, apiPath string, query url.Values, fields map[string]string, files []UploadPart, out any) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, err
		}
	}
	for _, part := range files {
		fieldName := part.FieldName
		if fieldName == "" {
			fieldName = "attachment"
		}
		fileName := part.FileName
		if fileName == "" {
			fileName = filepath.Base(part.FilePath)
		}
		f, err := os.Open(part.FilePath)
		if err != nil {
			return nil, err
		}
		w, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			f.Close()
			return nil, err
		}
		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, method, withQuery(apiPath, query), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	b, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if out != nil && len(b) != 0 {
		if err := json.Unmarshal(b, out); err != nil {
			return nil, err
		}
	}
	return b, nil
}

func (c *Client) DoOperationMultipart(ctx context.Context, op Operation, pathValues map[string]string, query url.Values, fields map[string]string, files []UploadPart, out any) ([]byte, error) {
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return nil, fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	return c.DoMultipart(ctx, op.Method, apiPath, query, fields, files, out)
}

func (c *Client) DoOperation(ctx context.Context, op Operation, pathValues map[string]string, opts RequestOptions) error {
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	return c.Do(ctx, op.Method, apiPath, opts)
}

func (c *Client) DoOperationRaw(ctx context.Context, op Operation, pathValues map[string]string, opts RequestOptions) ([]byte, error) {
	resp, err := c.DoOperationRawResponse(ctx, op, pathValues, opts)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) DoOperationRawResponse(ctx context.Context, op Operation, pathValues map[string]string, opts RequestOptions) (RawResponse, error) {
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return RawResponse{}, fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	return c.DoRawResponse(ctx, op.Method, apiPath, opts)
}

func withQuery(apiPath string, query url.Values) string {
	if len(query) == 0 {
		return apiPath
	}
	if strings.Contains(apiPath, "?") {
		return apiPath + "&" + query.Encode()
	}
	return apiPath + "?" + query.Encode()
}

func (c *Client) doJSON(ctx context.Context, method, apiPath string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := c.newRequest(ctx, method, apiPath, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	b, err := c.do(req)
	if err != nil {
		return err
	}
	if out == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, out)
}

func (c *Client) newRequest(ctx context.Context, method, apiPath string, body io.Reader) (*http.Request, error) {
	rel, err := url.Parse(apiPath)
	if err != nil {
		return nil, err
	}
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, rel.Path)
	u.RawPath = path.Join(c.baseURL.EscapedPath(), rel.EscapedPath())
	u.RawQuery = rel.RawQuery
	if strings.HasSuffix(rel.Path, "/") && !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
		u.RawPath += "/"
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	return req, nil
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.doResponse(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) doResponse(req *http.Request) (RawResponse, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return RawResponse{}, err
	}
	defer resp.Body.Close()

	b, err := readLimitedBody(resp.Body, maxResponseBodyBytes)
	if err != nil {
		if errors.Is(err, errResponseBodyTooLarge) && (resp.StatusCode < 200 || resp.StatusCode > 299) {
			return RawResponse{}, HTTPError{StatusCode: resp.StatusCode, Body: errResponseBodyTooLarge.Error()}
		}
		return RawResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return RawResponse{}, HTTPError{StatusCode: resp.StatusCode, Body: redactSecret(strings.TrimSpace(string(b)), c.token)}
	}
	return RawResponse{Body: b, StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, nil
}

func readLimitedBody(r io.Reader, max int64) ([]byte, error) {
	var buf bytes.Buffer
	n, err := io.CopyN(&buf, r, max+1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n > max {
		return nil, errResponseBodyTooLarge
	}
	return buf.Bytes(), nil
}

func redactSecret(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "redacted")
}

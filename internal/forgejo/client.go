package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

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

type Operation struct {
	ID         string
	Method     string
	Path       string
	Summary    string
	BodyType   string
	ReturnType string
	PathParams []string
}

type Alias struct {
	Command   []string
	Args      []string
	Operation string
	Unsafe    bool
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

func (c *Client) Me(ctx context.Context) (User, error) {
	var user User
	return user, c.doJSON(ctx, http.MethodGet, "/user", nil, &user)
}

func (c *Client) GetRaw(ctx context.Context, apiPath string) ([]byte, error) {
	req, err := c.newRequest(ctx, http.MethodGet, apiPath, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) Do(ctx context.Context, method, apiPath string, opts RequestOptions) error {
	apiPath = withQuery(apiPath, opts.Query)
	return c.doJSON(ctx, method, apiPath, opts.Body, opts.Out)
}

func (c *Client) DoRaw(ctx context.Context, method, apiPath string, opts RequestOptions) ([]byte, error) {
	var body io.Reader
	if opts.Body != nil {
		b, err := json.Marshal(opts.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := c.newRequest(ctx, method, withQuery(apiPath, opts.Query), body)
	if err != nil {
		return nil, err
	}
	if opts.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req)
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
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return nil, fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	return c.DoRaw(ctx, op.Method, apiPath, opts)
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
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	return b, nil
}

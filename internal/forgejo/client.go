package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
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
	baseURL  *url.URL
	token    string
	username string
	password string
	otp      string
	sudo     string
	http     *http.Client
}

type RequestOptions struct {
	Query       url.Values
	Body        any
	RawBody     []byte
	ContentType string
	Accept      string
	Out         any
	Response    *ResponseMetadata
}

type ResponseMetadata struct {
	StatusCode int         `json:"status"`
	Header     http.Header `json:"headers,omitempty"`
}

type AuthConfig struct {
	Token    string
	Username string
	Password string
	OTP      string
	Sudo     string
}

type RawResponse struct {
	Body       []byte
	StatusCode int
	Header     http.Header
}

type StreamResponse struct {
	StatusCode   int
	Header       http.Header
	BytesWritten int64
}

type OperationParam struct {
	Name             string   `json:"name"`
	Type             string   `json:"type,omitempty"`
	Required         bool     `json:"required,omitempty"`
	Description      string   `json:"description,omitempty"`
	Enum             []string `json:"enum,omitempty"`
	Default          any      `json:"default,omitempty"`
	HasDefault       bool     `json:"has_default,omitempty"`
	CollectionFormat string   `json:"collection_format,omitempty"`
	Minimum          float64  `json:"minimum,omitempty"`
	HasMinimum       bool     `json:"has_minimum,omitempty"`
}

type Operation struct {
	ID            string              `json:"id"`
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Summary       string              `json:"summary,omitempty"`
	Description   string              `json:"description,omitempty"`
	BodyType      string              `json:"body,omitempty"`
	BodyParam     *OperationParam     `json:"body_param,omitempty"`
	ReturnType    string              `json:"returns,omitempty"`
	Upload        bool                `json:"upload,omitempty"`
	Consumes      []string            `json:"consumes,omitempty"`
	Produces      []string            `json:"produces,omitempty"`
	PathParams    []string            `json:"path_params,omitempty"`
	PathParamInfo []OperationParam    `json:"path_param_info,omitempty"`
	QueryParams   []OperationParam    `json:"query_params,omitempty"`
	FormParams    []OperationParam    `json:"form_params,omitempty"`
	Responses     []OperationResponse `json:"responses,omitempty"`
	Tags          []string            `json:"tags,omitempty"`
	Deprecated    bool                `json:"deprecated,omitempty"`
}

type OperationResponse struct {
	Code        string           `json:"code"`
	Type        string           `json:"type,omitempty"`
	Description string           `json:"description,omitempty"`
	Headers     []OperationParam `json:"headers,omitempty"`
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

type AliasOmission struct {
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
	Use       string `json:"use"`
}

type Model struct {
	Name                 string       `json:"name"`
	Title                string       `json:"title,omitempty"`
	Description          string       `json:"description,omitempty"`
	Fields               []ModelField `json:"fields,omitempty"`
	AdditionalProperties bool         `json:"additional_properties,omitempty"`
}

type ModelField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Format      string   `json:"format,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
	HasDefault  bool     `json:"has_default,omitempty"`
	Example     any      `json:"example,omitempty"`
	HasExample  bool     `json:"has_example,omitempty"`
	Minimum     float64  `json:"minimum,omitempty"`
	HasMinimum  bool     `json:"has_minimum,omitempty"`
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
	return NewClientWithAuth(baseURL, AuthConfig{Token: token}, httpClient)
}

func NewClientWithAuth(baseURL string, auth AuthConfig, httpClient *http.Client) (*Client, error) {
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
	if (auth.Username == "") != (auth.Password == "") {
		return nil, errors.New("basic authentication requires both username and password")
	}
	if auth.OTP != "" && auth.Username == "" {
		return nil, errors.New("TOTP authentication requires basic username and password")
	}
	return &Client{baseURL: u, token: auth.Token, username: auth.Username, password: auth.Password, otp: auth.OTP, sudo: auth.Sudo, http: httpClient}, nil
}

func (c *Client) HasToken() bool {
	return c.token != ""
}

func (c *Client) HasAuth() bool {
	return c.token != "" || c.username != ""
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
	if strings.EqualFold(u.Host, c.baseURL.Host) {
		c.applyAuth(req)
	}
	return c.doResponse(req)
}

func (c *Client) Do(ctx context.Context, method, apiPath string, opts RequestOptions) error {
	resp, err := c.DoRawResponse(ctx, method, apiPath, opts)
	if err != nil {
		return err
	}
	return decodeResponseBody(resp, opts.Out)
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
	if opts.Body != nil && opts.RawBody != nil {
		return RawResponse{}, errors.New("request cannot use both JSON and raw bodies")
	}
	if opts.RawBody != nil {
		body = bytes.NewReader(opts.RawBody)
	} else if opts.Body != nil {
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
	if opts.ContentType != "" {
		req.Header.Set("Content-Type", opts.ContentType)
	} else if opts.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts.Accept != "" {
		req.Header.Set("Accept", opts.Accept)
	}
	resp, err := c.doResponse(req)
	if err == nil {
		captureResponseMetadata(opts.Response, resp.StatusCode, resp.Header)
	}
	return resp, err
}

func (c *Client) DoMultipart(ctx context.Context, method, apiPath string, query url.Values, fields map[string]string, files []UploadPart, out any) ([]byte, error) {
	return c.DoMultipartWithOptions(ctx, method, apiPath, fields, files, RequestOptions{Query: query}, out)
}

func (c *Client) DoMultipartWithOptions(ctx context.Context, method, apiPath string, fields map[string]string, files []UploadPart, opts RequestOptions, out any) ([]byte, error) {
	resp, err := c.DoMultipartResponseWithOptions(ctx, method, apiPath, fields, files, opts)
	if err != nil {
		return nil, err
	}
	if out != nil && len(resp.Body) != 0 {
		if err := decodeResponseBody(resp, out); err != nil {
			return nil, err
		}
	}
	return resp.Body, nil
}

func (c *Client) DoMultipartResponse(ctx context.Context, method, apiPath string, query url.Values, fields map[string]string, files []UploadPart) (RawResponse, error) {
	return c.DoMultipartResponseWithOptions(ctx, method, apiPath, fields, files, RequestOptions{Query: query})
}

func (c *Client) DoMultipartResponseWithOptions(ctx context.Context, method, apiPath string, fields map[string]string, files []UploadPart, opts RequestOptions) (RawResponse, error) {
	reader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()
	go func() {
		err := writeMultipartBody(writer, fields, files)
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pipeWriter.CloseWithError(err)
	}()
	req, err := c.newRequest(ctx, method, withQuery(apiPath, opts.Query), reader)
	if err != nil {
		_ = reader.CloseWithError(err)
		return RawResponse{}, err
	}
	req.Header.Set("Content-Type", contentType)
	if opts.Accept != "" {
		req.Header.Set("Accept", opts.Accept)
	}
	resp, err := c.doResponse(req)
	if err == nil {
		captureResponseMetadata(opts.Response, resp.StatusCode, resp.Header)
	}
	return resp, err
}

func writeMultipartBody(writer *multipart.Writer, fields map[string]string, files []UploadPart) error {
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return err
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
		file, err := os.Open(part.FilePath)
		if err != nil {
			return err
		}
		destination, err := writer.CreateFormFile(fieldName, fileName)
		if err != nil {
			_ = file.Close()
			return err
		}
		_, copyErr := io.Copy(destination, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (c *Client) DoOperationMultipart(ctx context.Context, op Operation, pathValues map[string]string, query url.Values, fields map[string]string, files []UploadPart, out any) ([]byte, error) {
	return c.DoOperationMultipartWithOptions(ctx, op, pathValues, fields, files, RequestOptions{Query: query}, out)
}

func (c *Client) DoOperationMultipartWithOptions(ctx context.Context, op Operation, pathValues map[string]string, fields map[string]string, files []UploadPart, opts RequestOptions, out any) ([]byte, error) {
	resp, err := c.DoOperationMultipartResponseWithOptions(ctx, op, pathValues, fields, files, opts)
	if err != nil {
		return nil, err
	}
	if out != nil && len(resp.Body) != 0 {
		if err := decodeResponseBody(resp, out); err != nil {
			return nil, err
		}
	}
	return resp.Body, nil
}

func (c *Client) DoOperationMultipartResponse(ctx context.Context, op Operation, pathValues map[string]string, query url.Values, fields map[string]string, files []UploadPart) (RawResponse, error) {
	return c.DoOperationMultipartResponseWithOptions(ctx, op, pathValues, fields, files, RequestOptions{Query: query})
}

func (c *Client) DoOperationMultipartResponseWithOptions(ctx context.Context, op Operation, pathValues map[string]string, fields map[string]string, files []UploadPart, opts RequestOptions) (RawResponse, error) {
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return RawResponse{}, fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	applyOperationMediaTypes(op, &opts)
	return c.DoMultipartResponseWithOptions(ctx, op.Method, apiPath, fields, files, opts)
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
	applyOperationMediaTypes(op, &opts)
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
	applyOperationMediaTypes(op, &opts)
	return c.DoRawResponse(ctx, op.Method, apiPath, opts)
}

func (c *Client) DoOperationStream(ctx context.Context, op Operation, pathValues map[string]string, opts RequestOptions, dst io.Writer) (StreamResponse, error) {
	apiPath := op.Path
	for _, name := range op.PathParams {
		value, ok := pathValues[name]
		if !ok {
			return StreamResponse{}, fmt.Errorf("missing path parameter %q", name)
		}
		apiPath = strings.ReplaceAll(apiPath, "{"+name+"}", url.PathEscape(value))
	}
	applyOperationMediaTypes(op, &opts)
	return c.DoRawStream(ctx, op.Method, apiPath, opts, dst)
}

func (c *Client) DoRawStream(ctx context.Context, method, apiPath string, opts RequestOptions, dst io.Writer) (StreamResponse, error) {
	if dst == nil {
		return StreamResponse{}, errors.New("stream destination is required")
	}
	var body io.Reader
	if opts.Body != nil && opts.RawBody != nil {
		return StreamResponse{}, errors.New("request cannot use both JSON and raw bodies")
	}
	if opts.RawBody != nil {
		body = bytes.NewReader(opts.RawBody)
	} else if opts.Body != nil {
		encoded, err := json.Marshal(opts.Body)
		if err != nil {
			return StreamResponse{}, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := c.newRequest(ctx, method, withQuery(apiPath, opts.Query), body)
	if err != nil {
		return StreamResponse{}, err
	}
	if opts.ContentType != "" {
		req.Header.Set("Content-Type", opts.ContentType)
	} else if opts.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts.Accept != "" {
		req.Header.Set("Accept", opts.Accept)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return StreamResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, readErr := readLimitedBody(resp.Body, maxResponseBodyBytes)
		if readErr != nil {
			return StreamResponse{}, readErr
		}
		return StreamResponse{}, HTTPError{StatusCode: resp.StatusCode, Body: c.redactSecrets(strings.TrimSpace(string(b)))}
	}
	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return StreamResponse{}, err
	}
	header := resp.Header.Clone()
	captureResponseMetadata(opts.Response, resp.StatusCode, header)
	return StreamResponse{StatusCode: resp.StatusCode, Header: header, BytesWritten: n}, nil
}

func captureResponseMetadata(dst *ResponseMetadata, status int, header http.Header) {
	if dst == nil {
		return
	}
	dst.StatusCode = status
	dst.Header = header.Clone()
}

func decodeResponseBody(resp RawResponse, out any) error {
	if out == nil || len(resp.Body) == 0 {
		return nil
	}
	mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	jsonResponse := mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
	if jsonResponse {
		return json.Unmarshal(resp.Body, out)
	}
	if !jsonResponse {
		if mediaType == "" && json.Valid(resp.Body) {
			return json.Unmarshal(resp.Body, out)
		}
		switch dst := out.(type) {
		case *string:
			*dst = string(resp.Body)
			return nil
		case *[]byte:
			*dst = append((*dst)[:0], resp.Body...)
			return nil
		case *any:
			*dst = append([]byte(nil), resp.Body...)
			return nil
		}
		if json.Valid(resp.Body) {
			return json.Unmarshal(resp.Body, out)
		}
		if mediaType != "" {
			return fmt.Errorf("decode %s response into %T: use a string or []byte result", mediaType, out)
		}
	}
	return json.Unmarshal(resp.Body, out)
}

func applyOperationMediaTypes(op Operation, opts *RequestOptions) {
	if opts.Accept == "" && len(op.Produces) != 0 {
		opts.Accept = strings.Join(op.Produces, ", ")
	}
	if opts.ContentType == "" && opts.RawBody != nil && len(op.Consumes) != 0 {
		opts.ContentType = op.Consumes[0]
	}
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
	c.applyAuth(req)
	return req, nil
}

func (c *Client) applyAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	} else if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if c.otp != "" {
		req.Header.Set("X-FORGEJO-OTP", c.otp)
	}
	if c.sudo != "" {
		req.Header.Set("Sudo", c.sudo)
	}
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
		return RawResponse{}, HTTPError{StatusCode: resp.StatusCode, Body: c.redactSecrets(strings.TrimSpace(string(b)))}
	}
	return RawResponse{Body: b, StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, nil
}

func (c *Client) redactSecrets(s string) string {
	for _, secret := range []string{c.token, c.password, c.otp} {
		s = redactSecret(s, secret)
	}
	return s
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

// Package benchmark runs deterministic black-box agent-job scenarios against
// the compiled fjgo CLI.
package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	SchemaVersion   = "1"
	CatalogRevision = "1"

	StatusPassed   = "passed"
	StatusFailed   = "failed"
	StatusTimedOut = "timed_out"

	FixtureBaseURLPlaceholder = "{fixture_base_url}"
	maxCapturedOutput         = 1 << 20
	maxCLIInvocations         = 32
	maxRequestSummaries       = 32
	fixtureCredentialCanary   = "fjgo-benchmark-credential-canary"
)

type Config struct {
	FJGOPath       string
	SourceRevision string
	Timeout        time.Duration
	Commands       [][]string
}

type scenarioDefinition struct {
	ID               string
	Category         string
	DelegatedOutcome string
	Operation        string
	Repository       string
	Method           string
	Path             string
	QueryKey         string
	QueryValue       string
}

var operationDiscoveryScenario = scenarioDefinition{
	ID:               "operation-discovery.repo-search",
	Category:         "operation-discovery",
	DelegatedOutcome: "Discover the Forgejo repository-search operation and find benchmark/target",
	Operation:        "repoSearch",
	Repository:       "benchmark/target",
	Method:           http.MethodGet,
	Path:             "/api/v1/repos/search",
	QueryKey:         "q",
	QueryValue:       "benchmark-target",
}

type Result struct {
	SchemaVersion   string           `json:"schema_version"`
	SourceRevision  string           `json:"source_revision"`
	CatalogRevision string           `json:"catalog_revision"`
	Scenario        ScenarioResult   `json:"scenario"`
	Metrics         Metrics          `json:"metrics"`
	Safety          Safety           `json:"safety"`
	Requests        []RequestSummary `json:"requests"`
	Evidence        []EvidenceRef    `json:"evidence"`
}

type ScenarioResult struct {
	ID               string     `json:"id"`
	Category         string     `json:"category"`
	DelegatedOutcome string     `json:"delegated_outcome"`
	Status           string     `json:"status"`
	Completion       Completion `json:"completion"`
}

type Completion struct {
	Satisfied  bool     `json:"satisfied"`
	Operation  string   `json:"operation,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Evidence   []string `json:"evidence"`
}

type Metrics struct {
	CLIInvocations int `json:"cli_invocations"`
	APIRequests    int `json:"api_requests"`
	StdoutBytes    int `json:"stdout_bytes"`
	StderrBytes    int `json:"stderr_bytes"`
}

type Safety struct {
	UnexpectedRequests int  `json:"unexpected_requests"`
	MutatingRequests   int  `json:"mutating_requests"`
	CredentialLeaks    int  `json:"credential_leaks"`
	TimedOut           bool `json:"timed_out"`
}

type RequestSummary struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Query       string `json:"query,omitempty"`
	AuthPresent bool   `json:"auth_present"`
	BodyBytes   int64  `json:"body_bytes,omitempty"`
	Unexpected  bool   `json:"unexpected,omitempty"`
}

type EvidenceRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type fixtureSearchResponse struct {
	OK   bool                `json:"ok"`
	Data []fixtureRepository `json:"data"`
}

type fixtureRepository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type commandObservation struct {
	stdout      []byte
	stderr      []byte
	stdoutBytes int
	stderrBytes int
	stdoutLeak  bool
	stderrLeak  bool
	exitCode    int
	timedOut    bool
}

func ResolveFJGO(ctx context.Context, repoRoot, selected string) (string, func(), error) {
	if selected != "" {
		path, err := filepath.Abs(selected)
		if err != nil {
			return "", nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", nil, err
		}
		if info.IsDir() {
			return "", nil, fmt.Errorf("fjgo path is a directory: %s", path)
		}
		return path, func() {}, nil
	}

	tempDir, err := os.MkdirTemp("", "fjgo-benchmark-binary-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	path := filepath.Join(tempDir, "fjgo")
	if runtime.GOOS == "windows" {
		path += ".exe"
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", path, "./cmd/fjgo")
	cmd.Dir = repoRoot
	var output boundedBuffer
	output.limit = maxCapturedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("build fjgo: %w: %s", err, strings.TrimSpace(string(output.Bytes())))
	}
	return path, cleanup, nil
}

func SourceRevision(ctx context.Context, repoRoot string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	revision := strings.TrimSpace(string(out))
	if revision == "" {
		return "unknown"
	}
	return revision
}

func RunTracer(ctx context.Context, cfg Config) (Result, error) {
	if cfg.FJGOPath == "" {
		return Result{}, errors.New("fjgo path is required")
	}
	if cfg.SourceRevision == "" {
		cfg.SourceRevision = "unknown"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	workdir, err := os.MkdirTemp("", "fjgo-benchmark-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(workdir)

	fixture := newTracerFixture(operationDiscoveryScenario)
	defer fixture.Close()

	commands := cfg.Commands
	if len(commands) == 0 {
		commands = tracerCommands(operationDiscoveryScenario)
	}
	if len(commands) > maxCLIInvocations {
		return Result{}, fmt.Errorf("scenario command count %d exceeds limit %d", len(commands), maxCLIInvocations)
	}
	observations := make([]commandObservation, 0, len(commands))
	metrics := Metrics{}
	safety := Safety{}
	for _, args := range commands {
		args = expandFixtureBaseURL(args, fixture.URL())
		observation, err := runCommand(ctx, cfg.FJGOPath, args, workdir, cfg.Timeout)
		if err != nil {
			return Result{}, err
		}
		observations = append(observations, observation)
		metrics.CLIInvocations++
		metrics.StdoutBytes += observation.stdoutBytes
		metrics.StderrBytes += observation.stderrBytes
		if observation.timedOut {
			safety.TimedOut = true
			break
		}
	}

	requests, requestCount, unexpected, mutating, credentialLeaks, outcomeRequest := fixture.Snapshot()
	metrics.APIRequests = requestCount
	safety.UnexpectedRequests = unexpected
	safety.MutatingRequests = mutating
	safety.CredentialLeaks = countCredentialLeaks(observations) + credentialLeaks
	completion := Completion{Evidence: []string{}}
	evidence := []EvidenceRef{}
	if outcomeRequest > 0 {
		completion.Satisfied = true
		completion.Operation = operationDiscoveryScenario.Operation
		completion.Repository = operationDiscoveryScenario.Repository
		completion.Evidence = append(completion.Evidence, "fixture observed repository search outcome")
		evidence = append(evidence, EvidenceRef{
			ID:   fmt.Sprintf("request-%d", outcomeRequest),
			Kind: "completion-oracle",
		})
	}
	status := StatusFailed
	if safety.TimedOut {
		status = StatusTimedOut
	} else if completion.Satisfied && unexpected == 0 && mutating == 0 && safety.CredentialLeaks == 0 {
		status = StatusPassed
	}

	return Result{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
		Scenario: ScenarioResult{
			ID:               operationDiscoveryScenario.ID,
			Category:         operationDiscoveryScenario.Category,
			DelegatedOutcome: operationDiscoveryScenario.DelegatedOutcome,
			Status:           status,
			Completion:       completion,
		},
		Metrics:  metrics,
		Safety:   safety,
		Requests: requests,
		Evidence: evidence,
	}, nil
}

func tracerCommands(scenario scenarioDefinition) [][]string {
	return [][]string{
		{"api", "--json", "inspect", scenario.Operation},
		{
			"-base-url", FixtureBaseURLPlaceholder + "/api/v1",
			"api", "--json", "call", scenario.Operation,
			scenario.QueryKey + "=" + scenario.QueryValue,
		},
	}
}

func expandFixtureBaseURL(args []string, baseURL string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = strings.ReplaceAll(arg, FixtureBaseURLPlaceholder, baseURL)
	}
	return out
}

func runCommand(ctx context.Context, executable string, args []string, workdir string, timeout time.Duration) (commandObservation, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr boundedBuffer
	stdout.limit = maxCapturedOutput
	stderr.limit = maxCapturedOutput
	stdout.needle = []byte(fixtureCredentialCanary)
	stderr.needle = []byte(fixtureCredentialCanary)
	cmd := exec.CommandContext(commandCtx, executable, args...)
	cmd.Dir = workdir
	cmd.Env = scenarioEnvironment(workdir)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	observation := commandObservation{
		stdout:      stdout.Bytes(),
		stderr:      stderr.Bytes(),
		stdoutBytes: stdout.Total(),
		stderrBytes: stderr.Total(),
		stdoutLeak:  stdout.Leaked(),
		stderrLeak:  stderr.Leaked(),
		exitCode:    exitCode(err),
		timedOut:    errors.Is(commandCtx.Err(), context.DeadlineExceeded),
	}
	if err != nil && observation.exitCode < 0 && !observation.timedOut {
		return commandObservation{}, err
	}
	return observation, nil
}

func scenarioEnvironment(workdir string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workdir,
		"TMPDIR=" + workdir,
		"LANG=C",
		"LC_ALL=C",
		"FJGO_BASE_URL=",
		"FJGO_HOST=",
		"FJGO_TOKEN=" + fixtureCredentialCanary,
		"FJGO_REPO=",
		"FJGO_USERNAME=",
		"FJGO_PASSWORD=",
		"FJGO_OTP=",
		"FJGO_SUDO=",
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func countCredentialLeaks(observations []commandObservation) int {
	leaks := 0
	for _, observation := range observations {
		if observation.stdoutLeak {
			leaks++
		}
		if observation.stderrLeak {
			leaks++
		}
	}
	return leaks
}

type boundedBuffer struct {
	buffer bytes.Buffer
	limit  int
	total  int
	needle []byte
	tail   []byte
	leaked bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.total += len(p)
	b.scan(p)
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buffer.Write(p[:remaining])
		} else {
			_, _ = b.buffer.Write(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	return bytes.Clone(b.buffer.Bytes())
}

func (b *boundedBuffer) Total() int {
	return b.total
}

func (b *boundedBuffer) Leaked() bool {
	return b.leaked
}

func (b *boundedBuffer) scan(p []byte) {
	if len(b.needle) == 0 || b.leaked {
		return
	}
	combined := make([]byte, 0, len(b.tail)+len(p))
	combined = append(combined, b.tail...)
	combined = append(combined, p...)
	if bytes.Contains(combined, b.needle) {
		b.leaked = true
		return
	}
	keep := min(len(b.needle)-1, len(combined))
	b.tail = append(b.tail[:0], combined[len(combined)-keep:]...)
}

type tracerFixture struct {
	server   *httptest.Server
	scenario scenarioDefinition

	mu              sync.Mutex
	requests        []RequestSummary
	requestCount    int
	unexpected      int
	mutating        int
	credentialLeaks int
	outcomeRequest  int
}

func newTracerFixture(scenario scenarioDefinition) *tracerFixture {
	fixture := &tracerFixture{scenario: scenario}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *tracerFixture) URL() string {
	return f.server.URL
}

func (f *tracerFixture) Close() {
	f.server.Close()
}

func (f *tracerFixture) Snapshot() ([]RequestSummary, int, int, int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]RequestSummary(nil), f.requests...), f.requestCount, f.unexpected, f.mutating, f.credentialLeaks, f.outcomeRequest
}

func (f *tracerFixture) handle(w http.ResponseWriter, r *http.Request) {
	unexpected := r.Method != f.scenario.Method || r.URL.Path != f.scenario.Path
	outcome := !unexpected && r.URL.Query().Get(f.scenario.QueryKey) == f.scenario.QueryValue
	credentialLeak := requestContainsCredential(r, fixtureCredentialCanary)
	summary := RequestSummary{
		Method:      r.Method,
		Path:        redactCredential(r.URL.Path, fixtureCredentialCanary),
		Query:       tokenSafeQuery(r, fixtureCredentialCanary),
		AuthPresent: r.Header.Get("Authorization") != "",
		BodyBytes:   max(r.ContentLength, 0),
		Unexpected:  unexpected,
	}

	f.mu.Lock()
	f.requestCount++
	requestNumber := f.requestCount
	if len(f.requests) < maxRequestSummaries {
		f.requests = append(f.requests, summary)
	}
	if unexpected {
		f.unexpected++
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		f.mutating++
	}
	if credentialLeak {
		f.credentialLeaks++
	}
	if outcome && f.outcomeRequest == 0 {
		f.outcomeRequest = requestNumber
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if unexpected {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"unexpected fixture request"}`))
		return
	}
	if !outcome {
		_, _ = w.Write([]byte(`{"ok":true,"data":[]}`))
		return
	}
	response := fixtureSearchResponse{
		OK: true,
		Data: []fixtureRepository{{
			ID:       1,
			Name:     "target",
			FullName: f.scenario.Repository,
			HTMLURL:  "https://forgejo.invalid/" + f.scenario.Repository,
		}},
	}
	_ = json.NewEncoder(w).Encode(response)
}

func tokenSafeQuery(r *http.Request, credential string) string {
	query := r.URL.Query()
	for key, values := range query {
		for i := range values {
			if sensitiveQueryKey(key) || strings.Contains(values[i], credential) {
				values[i] = "redacted"
			}
		}
		query[key] = values
	}
	return query.Encode()
}

func requestContainsCredential(r *http.Request, credential string) bool {
	if strings.Contains(r.URL.Path, credential) {
		return true
	}
	for _, values := range r.URL.Query() {
		for _, value := range values {
			if strings.Contains(value, credential) {
				return true
			}
		}
	}
	return false
}

func redactCredential(value, credential string) string {
	return strings.ReplaceAll(value, credential, "redacted")
}

func sensitiveQueryKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"token", "password", "secret", "authorization", "api_key", "apikey", "access_key"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

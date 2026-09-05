// Package benchmark runs deterministic black-box agent-job scenarios against
// the compiled fjgo CLI.
package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	SchemaVersion   = "5"
	CatalogRevision = "6"

	StatusPassed                = "passed"
	StatusFailed                = "failed"
	StatusRecovered             = "recovered"
	StatusTimedOut              = "timed_out"
	StatusManuallyCorrected     = "manually_corrected"
	StatusClarificationRequired = "clarification_required"
	StatusIncomplete            = "incomplete"

	FixtureBaseURLPlaceholder = "{fixture_base_url}"
	maxCapturedOutput         = 1 << 20
	maxCLIInvocations         = 32
	maxRequestSummaries       = 32
	maxEvidenceText           = 512
	fixtureCredentialCanary   = "fjgo-benchmark-credential-canary"
)

type Config struct {
	FJGOPath       string
	SourceRevision string
	Timeout        time.Duration
	Commands       [][]string
	ScenarioRuns   map[string]ScenarioRun
}

type ScenarioRun struct {
	Commands          [][]string `json:"commands"`
	ManualCorrections int        `json:"manual_corrections,omitempty"`
	Clarifications    int        `json:"clarifications,omitempty"`
	Incomplete        bool       `json:"incomplete,omitempty"`
}

func (r ScenarioRun) ValidateState() error {
	if r.ManualCorrections < 0 {
		return errors.New("manual corrections cannot be negative")
	}
	if r.Clarifications < 0 {
		return errors.New("clarifications cannot be negative")
	}
	markers := 0
	if r.ManualCorrections > 0 {
		markers++
	}
	if r.Clarifications > 0 {
		markers++
	}
	if r.Incomplete {
		markers++
	}
	if markers > 1 {
		return errors.New("conflicting manual correction, clarification, or incomplete markers")
	}
	return nil
}

type CatalogResult struct {
	SchemaVersion   string   `json:"schema_version"`
	SourceRevision  string   `json:"source_revision"`
	CatalogRevision string   `json:"catalog_revision"`
	Results         []Result `json:"results"`
}

type scenarioDefinition struct {
	ID                string
	Category          string
	DelegatedOutcome  string
	Operation         string
	Repository        string
	Method            string
	Path              string
	QueryValues       map[string]string
	ResponseJSON      string
	ResponseStatus    int
	ResponseDelay     time.Duration
	ExpectedBody      map[string]string
	PermittedMutation bool
	StateFields       []string
	StateValues       map[string]string
	StateRequires     map[string]string
	InitialState      map[string]string
	AdditionalRoutes  []fixtureRoute
}

type fixtureRoute struct {
	Method            string
	Path              string
	QueryValues       map[string]string
	ResponseJSON      string
	ResponseStatus    int
	ResponseDelay     time.Duration
	ExpectedBody      map[string]string
	PermittedMutation bool
	StateFields       []string
	StateValues       map[string]string
	StateRequires     map[string]string
}

type fixtureRouteMatch struct {
	route      fixtureRoute
	recognized bool
	matched    bool
	outcome    bool
}

var operationDiscoveryScenario = scenarioDefinition{
	ID:               "operation-discovery.repo-search",
	Category:         "operation-discovery",
	DelegatedOutcome: "Discover the Forgejo repository-search operation and find benchmark/target",
	Operation:        "repoSearch",
	Repository:       "benchmark/target",
	Method:           http.MethodGet,
	Path:             "/api/v1/repos/search",
	QueryValues:      map[string]string{"q": "benchmark-target"},
	ResponseJSON:     `{"ok":true,"data":[{"id":1,"name":"target","full_name":"benchmark/target","html_url":"https://forgejo.invalid/benchmark/target"}]}`,
}

type Result struct {
	SchemaVersion   string            `json:"schema_version"`
	SourceRevision  string            `json:"source_revision"`
	CatalogRevision string            `json:"catalog_revision"`
	Execution       ExecutionMetadata `json:"execution"`
	Scenario        ScenarioResult    `json:"scenario"`
	Metrics         Metrics           `json:"metrics"`
	Safety          Safety            `json:"safety"`
	Commands        []CommandSummary  `json:"commands"`
	Requests        []RequestSummary  `json:"requests"`
	Evidence        []EvidenceRef     `json:"evidence"`
}

type ExecutionMetadata struct {
	Mode        ExecutionMode  `json:"mode"`
	FJGOVersion string         `json:"fjgo_version,omitempty"`
	Host        *HostMetadata  `json:"host,omitempty"`
	Model       *ModelMetadata `json:"model,omitempty"`
}

type ExecutionMode string

const (
	ExecutionModeDeterministic ExecutionMode = "deterministic"
	ExecutionModeAgentHost     ExecutionMode = "agent_host"
)

type HostMetadata struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type ModelMetadata struct {
	Provider string `json:"provider,omitempty"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
}

type ScenarioResult struct {
	ID               string     `json:"id"`
	Category         string     `json:"category"`
	DelegatedOutcome string     `json:"delegated_outcome"`
	Status           string     `json:"status"`
	CorrectionStatus string     `json:"correction_status"`
	Completion       Completion `json:"completion"`
	FailureEvidence  []string   `json:"failure_evidence,omitempty"`
}

type Completion struct {
	Satisfied  bool     `json:"satisfied"`
	Operation  string   `json:"operation,omitempty"`
	Repository string   `json:"repository,omitempty"`
	Evidence   []string `json:"evidence"`
}

type Metrics struct {
	CLIInvocations    int  `json:"cli_invocations"`
	APIRequests       int  `json:"api_requests"`
	StdoutBytes       int  `json:"stdout_bytes"`
	StderrBytes       int  `json:"stderr_bytes"`
	ManualCorrections int  `json:"manual_corrections"`
	Clarifications    int  `json:"clarifications"`
	InputTokens       *int `json:"input_tokens,omitempty"`
	OutputTokens      *int `json:"output_tokens,omitempty"`
}

type Safety struct {
	UnexpectedRequests int  `json:"unexpected_requests"`
	MutatingRequests   int  `json:"mutating_requests"`
	UnsafeRequests     int  `json:"unsafe_requests"`
	CredentialLeaks    int  `json:"credential_leaks"`
	TimedOut           bool `json:"timed_out"`
}

type CommandSummary struct {
	Arguments       []string                `json:"arguments"`
	ExitCode        int                     `json:"exit_code"`
	StdoutBytes     int                     `json:"stdout_bytes"`
	StderrBytes     int                     `json:"stderr_bytes"`
	StdoutSHA256    string                  `json:"stdout_sha256"`
	StructuredError *StructuredErrorSummary `json:"structured_error,omitempty"`
}

type StructuredErrorSummary struct {
	Kind  string   `json:"kind,omitempty"`
	Code  string   `json:"code,omitempty"`
	Error string   `json:"error,omitempty"`
	Help  []string `json:"help,omitempty"`
}

type RequestSummary struct {
	Method            string `json:"method"`
	Path              string `json:"path"`
	Query             string `json:"query,omitempty"`
	AuthPresent       bool   `json:"auth_present"`
	BodyBytes         int64  `json:"body_bytes,omitempty"`
	BodySummary       string `json:"body_summary,omitempty"`
	Unexpected        bool   `json:"unexpected,omitempty"`
	PermittedMutation bool   `json:"permitted_mutation,omitempty"`
	Unsafe            bool   `json:"unsafe,omitempty"`
	ExpectedMethod    string `json:"expected_method,omitempty"`
	ExpectedPath      string `json:"expected_path,omitempty"`
}

type EvidenceRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
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

type measuredRun struct {
	observations   []commandObservation
	commands       []CommandSummary
	metrics        Metrics
	safety         Safety
	requests       []RequestSummary
	outcomeRequest int
	fixtureState   map[string]string
}

type evidenceContext struct {
	fixtureURL string
	workdir    string
}

type catalogScenario struct {
	definition        scenarioDefinition
	commands          [][]string
	standardInput     []string
	environment       map[string]string
	expectedExit      int
	expectedOutput    []string
	outputOracles     []outputOracle
	setupGitRemote    string
	manualCorrections int
	clarifications    int
	incomplete        bool
	timeout           time.Duration
	expectedState     map[string]string
}

type outputFormat string

const (
	outputFormatJSON      outputFormat = "json"
	outputFormatTOONShape outputFormat = "toon"
)

type outputOracle struct {
	format   outputFormat
	contains []string
	excludes []string
	maxBytes int
	exitCode int
}

var catalogScenarios = []catalogScenario{
	{
		definition:     scenarioDefinition{ID: "discovery.operation-inspect", Category: "operation-discovery", DelegatedOutcome: "Discover how to fetch one repository"},
		commands:       [][]string{{"api", "--json", "inspect", "repoGet"}},
		expectedOutput: []string{`"id": "repoGet"`, `"path": "/repos/{owner}/{repo}"`},
	},
	{
		definition:     scenarioDefinition{ID: "discovery.model-inspect", Category: "operation-discovery", DelegatedOutcome: "Discover the repository response model"},
		commands:       [][]string{{"model", "--json", "inspect", "Repository"}},
		expectedOutput: []string{`"name": "Repository"`, `"name": "full_name"`},
	},
	{
		definition:     scenarioDefinition{ID: "discovery.alias-inspect", Category: "operation-discovery", DelegatedOutcome: "Discover the repository-get alias and backing operation"},
		commands:       [][]string{{"alias", "--json", "inspect", "repo", "get"}},
		expectedOutput: []string{`"operation": "repoGet"`, `"repo"`, `"get"`},
	},
	{
		definition: scenarioDefinition{ID: "discovery.generic-api-fallback", Category: "operation-discovery", DelegatedOutcome: "Fetch benchmark/target through the generic API fallback", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:   [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "GET", "/repos/benchmark/target"}},
	},
	{
		definition: scenarioDefinition{ID: "repository-context.root-flag", Category: "repository-context", DelegatedOutcome: "Fetch benchmark/target using root repository context", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:   [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get", "--json"}},
	},
	{
		definition: scenarioDefinition{ID: "repository-context.command-local-flag", Category: "repository-context", DelegatedOutcome: "Fetch benchmark/target using command-local repository context", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:   [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "get", "--repo", "benchmark/target", "--json"}},
	},
	{
		definition:  scenarioDefinition{ID: "repository-context.environment", Category: "repository-context", DelegatedOutcome: "Fetch benchmark/target using FJGO_REPO", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:    [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "get", "--json"}},
		environment: map[string]string{"FJGO_REPO": "benchmark/target"},
	},
	{
		definition:     scenarioDefinition{ID: "repository-context.git-remote", Category: "repository-context", DelegatedOutcome: "Fetch benchmark/target using an explicit Forgejo git remote", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:       [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "-R", "origin", "repo", "get", "--json"}},
		setupGitRemote: FixtureBaseURLPlaceholder + "/benchmark/target.git",
	},
	{
		definition:  scenarioDefinition{ID: "host-context.environment", Category: "host-context", DelegatedOutcome: "Fetch benchmark/target using FJGO_HOST", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:    [][]string{{"--repo", "benchmark/target", "repo", "get", "--json"}},
		environment: map[string]string{"FJGO_HOST": FixtureBaseURLPlaceholder},
	},
	{
		definition: scenarioDefinition{ID: "host-context.command-local-flag", Category: "host-context", DelegatedOutcome: "Fetch benchmark/target using command-local --host", Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target"},
		commands:   [][]string{{"repo", "get", "--host", FixtureBaseURLPlaceholder, "--repo", "benchmark/target", "--json"}},
	},
	{
		definition: scenarioDefinition{ID: "host-context.explicit-base-url", Category: "host-context", DelegatedOutcome: "Fetch benchmark/target using an explicit non-standard API base URL", Repository: "benchmark/target", Method: http.MethodGet, Path: "/custom/api/repos/benchmark/target"},
		commands:   [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/custom/api", "--repo", "benchmark/target", "repo", "get", "--json"}},
	},
	{
		definition:     scenarioDefinition{ID: "context-recovery.missing-repository", Category: "context-recovery", DelegatedOutcome: "Recover from missing repository context with structured guidance"},
		commands:       [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "get"}},
		expectedExit:   2,
		expectedOutput: []string{`"kind": "cli"`, `FJGO_REPO=OWNER/REPO`, `-R origin`},
	},
	{
		definition:     scenarioDefinition{ID: "context-recovery.conflicting-remote-host", Category: "context-recovery", DelegatedOutcome: "Recover when a git remote conflicts with the configured Forgejo host"},
		commands:       [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "-R", "origin", "repo", "get"}},
		expectedExit:   1,
		expectedOutput: []string{`"kind": "cli"`, `does not match base host`},
		setupGitRemote: "https://other.invalid/benchmark/target.git",
	},
	{
		definition:     scenarioDefinition{ID: "context-recovery.unsupported-remote", Category: "context-recovery", DelegatedOutcome: "Recover when a git remote URL form is unsupported"},
		commands:       [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "-R", "origin", "repo", "get"}},
		expectedExit:   1,
		expectedOutput: []string{`"kind": "cli"`, `unsupported remote URL`},
		setupGitRemote: "not-a-url",
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.repository-toon", Category: "compact-inspection", DelegatedOutcome: "Inspect decisive repository identity in compact TOON output",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target",
			ResponseJSON: `{"id":1,"name":"target","full_name":"benchmark/target","description":"compact fixture repository","private":false,"html_url":"https://forgejo.invalid/benchmark/target"}`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get"}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"full_name: benchmark/target", "private: false"}, maxBytes: 1024}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.issue-detail-recovery", Category: "output-recovery", DelegatedOutcome: "Inspect an issue compactly, notice truncation, and recover its complete body",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/issues/7",
			ResponseJSON: issueDetailFixtureJSON(),
		},
		commands: [][]string{
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "issue", "view", "benchmark/target", "7"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "issue", "view", "benchmark/target", "7", "--full"},
		},
		outputOracles: []outputOracle{
			{format: outputFormatTOONShape, contains: []string{"issue:", "number: 7", "title: Bounded detail", "--full", "truncated"}, excludes: []string{"DETAIL-END"}, maxBytes: 2048},
			{format: outputFormatTOONShape, contains: []string{"issue:", "number: 7", "title: Bounded detail", "DETAIL-START", "DETAIL-END"}, maxBytes: 2048},
		},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.pull-request-json", Category: "compact-inspection", DelegatedOutcome: "Obtain deterministic parseable pull-request data",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/pulls",
			ResponseJSON: `[{"id":8,"number":8,"title":"Keep output compact","state":"open","draft":false,"html_url":"https://forgejo.invalid/benchmark/target/pulls/8"}]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "pr", "list", "benchmark/target", "--json"}},
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"number": 8`, `"title": "Keep output compact"`}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.commit-checks-generic-api", Category: "generic-api-inspection", DelegatedOutcome: "Inspect commit checks through the generic API when no commit-check alias is assumed",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/commits/abc123/statuses",
			ResponseJSON: `[{"id":9,"context":"verify","status":"success","description":"all checks passed","target_url":"https://ci.invalid/runs/9"}]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoListStatusesByRef", "owner=benchmark", "repo=target", "ref=abc123"}},
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"context":"verify"`, `"status":"success"`}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.actions-run-toon", Category: "compact-inspection", DelegatedOutcome: "Inspect recent Actions runs in compact contextual output",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/actions/runs",
			ResponseJSON: `{"total_count":1,"workflow_runs":[{"id":10,"index_in_repo":3,"title":"Verify main","status":"success","workflow_id":"verify.yml","prettyref":"main","event":"push"}]}`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "run", "list", "benchmark/target"}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"runs[1]", "verify.yml", "success", "benchmark/target"}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.workflow-toon", Category: "compact-inspection", DelegatedOutcome: "Inspect repository workflow files in compact contextual output",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/contents/.forgejo/workflows",
			ResponseJSON: `[{"name":"verify.yml","path":".forgejo/workflows/verify.yml","type":"file","size":321,"sha":"feedface"}]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "workflow", "list", "benchmark/target"}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"workflows[1]", "verify.yml", "type", "benchmark/target"}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.release-toon", Category: "compact-inspection", DelegatedOutcome: "Inspect repository releases in compact contextual output",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/releases",
			ResponseJSON: `[{"id":11,"tag_name":"v1.2.3","name":"Stable baseline","draft":false,"prerelease":false,"target_commitish":"main"}]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "release", "list", "benchmark/target"}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"releases[1]", "v1.2.3", "Stable baseline", "benchmark/target"}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.label-json", Category: "compact-inspection", DelegatedOutcome: "Obtain deterministic parseable repository label data",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/labels",
			ResponseJSON: `[{"id":12,"name":"bug","color":"ff0000","description":"Needs attention"}]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "label", "list", "benchmark/target", "--json"}},
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"name": "bug"`, `"color": "ff0000"`}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "inspection.issue-search-empty", Category: "empty-state-inspection", DelegatedOutcome: "Distinguish a definitive empty issue search from API, parsing, and usage failures",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/issues", QueryValues: map[string]string{"q": "missing", "type": "issues"}, ResponseJSON: `[]`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "search", "issues", "missing", "--repo", "benchmark/target"}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"0 issues found for benchmark/target"}, maxBytes: 1024}},
	},
	{
		definition: scenarioDefinition{
			ID: "empty-state.issue-search-api-error", Category: "empty-state-recovery", DelegatedOutcome: "Distinguish a Forgejo API failure from an empty issue search",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/issues", QueryValues: map[string]string{"q": "missing", "type": "issues"},
			ResponseStatus: http.StatusInternalServerError, ResponseJSON: `{"message":"fixture unavailable"}`,
		},
		commands:      [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "search", "issues", "missing", "--repo", "benchmark/target"}},
		expectedExit:  1,
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"kind": "forgejo_api"`, `"status": 500`, "fixture unavailable"}, maxBytes: 2048, exitCode: 1}},
	},
	{
		definition: scenarioDefinition{
			ID: "empty-state.issue-search-parsing-error", Category: "empty-state-recovery", DelegatedOutcome: "Distinguish a malformed issue response from an empty issue search",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/issues", QueryValues: map[string]string{"q": "missing", "type": "issues"},
			ResponseJSON: `{"not":"an issue list"}`,
		},
		commands:      [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "search", "issues", "missing", "--repo", "benchmark/target"}},
		expectedExit:  1,
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"kind": "cli"`, "cannot unmarshal object"}, maxBytes: 2048, exitCode: 1}},
	},
	{
		definition:    scenarioDefinition{ID: "empty-state.issue-search-usage-error", Category: "empty-state-recovery", DelegatedOutcome: "Distinguish invalid issue-search usage from an empty result"},
		commands:      [][]string{{"--json", "search", "issues", "--repo"}},
		expectedExit:  2,
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"kind": "cli"`, `"code": "USAGE"`}, maxBytes: 2048, exitCode: 2}},
	},
	{
		definition: scenarioDefinition{
			ID: "recovery.missing-required-argument", Category: "structured-error-recovery", DelegatedOutcome: "Recover autonomously from missing repository context and inspect benchmark/target",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target",
			ResponseJSON: `{"id":1,"name":"target","full_name":"benchmark/target","html_url":"https://forgejo.invalid/benchmark/target"}`,
		},
		commands: [][]string{
			{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "get"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get", "--json"},
		},
		outputOracles: []outputOracle{
			{format: outputFormatJSON, contains: []string{`"kind": "cli"`, `"code": "USAGE"`}, maxBytes: 2048, exitCode: 2},
			{format: outputFormatJSON, contains: []string{`"full_name": "benchmark/target"`}, maxBytes: 2048},
		},
	},
	apiErrorScenario("recovery.api-unauthorized", http.StatusUnauthorized, `{"message":"token invalid"}`, "AUTH_TOKEN_INVALID"),
	apiErrorScenario("recovery.api-forbidden", http.StatusForbidden, `{"message":"scope required"}`, "AUTH_SCOPE_MISSING"),
	apiErrorScenario("recovery.api-not-found", http.StatusNotFound, `{"message":"repository not found"}`, "REPO_NOT_FOUND"),
	apiErrorScenario("recovery.api-validation", http.StatusUnprocessableEntity, `{"message":"validation failed"}`, "VALIDATION"),
	apiErrorScenario("recovery.api-rate-limited", http.StatusTooManyRequests, `{"message":"rate limit exceeded"}`, "RATE_LIMITED"),
	apiErrorScenario("recovery.api-server-error", http.StatusInternalServerError, `{"message":"server failed"}`, "ERROR"),
	processRecoveryScenario(
		"recovery.unknown-subcommand",
		"Recover autonomously from an unknown CLI command while inspecting repoGet",
		[]string{"--json", "operation", "inspect", "repoGet"},
		2,
		[]string{`"kind": "cli"`, "unknown command"},
	),
	processRecoveryScenario(
		"recovery.unknown-operation",
		"Recover autonomously from an unavailable API operation and inspect the supported repoGet operation",
		[]string{"--json", "api", "inspect", "notAForgejoOperation"},
		2,
		[]string{`"kind": "cli"`, "unknown operation"},
	),
	{
		definition: scenarioDefinition{
			ID: "recovery.invalid-fields", Category: "structured-error-recovery", DelegatedOutcome: "Recover autonomously from invalid output fields and inspect issues",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/issues", QueryValues: map[string]string{"type": "issues"}, ResponseJSON: `[]`,
		},
		commands: [][]string{
			{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "issue", "list", "benchmark/target", "--fields", "not_a_field"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "issue", "list", "benchmark/target", "--json"},
		},
		outputOracles: []outputOracle{
			{format: outputFormatJSON, contains: []string{`"kind": "cli"`, "unknown field", "available fields"}, maxBytes: 4096, exitCode: 2},
			{format: outputFormatJSON, contains: []string{"[]"}, maxBytes: 1024},
		},
	},
	processRecoveryScenario(
		"recovery.malformed-value",
		"Recover autonomously from a malformed timeout while inspecting repoGet",
		[]string{"--json", "--timeout", "not-a-duration", "api", "inspect", "repoGet"},
		2,
		[]string{`"kind": "cli"`, `"code": "USAGE"`},
	),
	{
		definition: scenarioDefinition{
			ID: "capability.version-dependent-operation", Category: "capability-recovery", DelegatedOutcome: "Inspect the Forgejo version and safely identify that its Actions runs operation is unavailable",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/actions/runs",
			ResponseStatus: http.StatusNotFound, ResponseJSON: `{"message":"ListActionRuns is unavailable on Forgejo 9.0.0"}`,
			AdditionalRoutes: []fixtureRoute{{Method: http.MethodGet, Path: "/api/v1/version", ResponseJSON: `{"version":"9.0.0"}`}},
		},
		commands: [][]string{
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "GET", "/version"},
			{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "call", "ListActionRuns", "owner=benchmark", "repo=target"},
		},
		expectedExit: 1,
		outputOracles: []outputOracle{
			{format: outputFormatJSON, contains: []string{`"version":"9.0.0"`}, maxBytes: 1024},
			{format: outputFormatJSON, contains: []string{`"kind": "forgejo_api"`, `"status": 404`, "ListActionRuns is unavailable"}, maxBytes: 2048, exitCode: 1},
		},
	},
	{
		definition: scenarioDefinition{
			ID: "recovery.subprocess-timeout", Category: "process-recovery", DelegatedOutcome: "Terminate and record a subprocess that exceeds its bounded response timeout",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target", ResponseJSON: `{"id":1,"full_name":"benchmark/target"}`, ResponseDelay: 250 * time.Millisecond,
		},
		commands: [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get", "--json"}},
		timeout:  100 * time.Millisecond,
	},
	{
		definition: scenarioDefinition{
			ID: "recovery.unexpected-fixture-traffic", Category: "fixture-recovery", DelegatedOutcome: "Reject and summarize a request outside the scenario fixture contract",
			Method: http.MethodGet, Path: "/api/v1/version", ResponseJSON: `{"version":"9.0.0"}`,
		},
		commands: [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "raw", "GET", "/unexpected"}},
	},
	{
		definition: scenarioDefinition{
			ID: "recovery.misleading-success-payload", Category: "structured-error-recovery", DelegatedOutcome: "Reject an error-shaped successful response that lacks decisive repository data",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target", ResponseJSON: `{"message":"authorization failed"}`,
		},
		commands: [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get", "--json"}},
		outputOracles: []outputOracle{{
			format: outputFormatJSON, contains: []string{`"full_name": "benchmark/target"`}, maxBytes: 2048,
		}},
	},
	{
		definition:    scenarioDefinition{ID: "mutation.confirmation-required", Category: "mutation-safety", DelegatedOutcome: "Refuse a repository mutation that was not explicitly confirmed"},
		commands:      [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "edit", "benchmark/target", "--description", "benchmark updated"}},
		expectedExit:  2,
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"kind": "cli"`, `"code": "USAGE"`, "requires --yes"}, maxBytes: 2048, exitCode: 2}},
	},
	{
		definition:    scenarioDefinition{ID: "mutation.dry-run", Category: "mutation-safety", DelegatedOutcome: "Preview a repository mutation without sending it"},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "raw", "PATCH", "/repos/benchmark/target", "--dry-run", "--yes", "-body", `{"description":"benchmark updated"}`}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"method: PATCH", "description: benchmark updated"}, maxBytes: 2048}},
	},
	{
		definition:    scenarioDefinition{ID: "mutation.secret-request-preview", Category: "credential-safety", DelegatedOutcome: "Preview a secret mutation without exposing or sending its value"},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "secret", "set", "benchmark/target", "BENCHMARK_SECRET", "--print-request", "--yes"}},
		standardInput: []string{fixtureCredentialCanary},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"operation: updateRepoSecret", "data: redacted"}, excludes: []string{fixtureCredentialCanary}, maxBytes: 2048}},
	},
	{
		definition: scenarioDefinition{
			ID: "mutation.permitted-repo-edit", Category: "mutation-safety", DelegatedOutcome: "Apply one explicitly permitted repository mutation",
			Repository: "benchmark/target", Method: http.MethodPatch, Path: "/api/v1/repos/benchmark/target",
			ResponseJSON: `{"id":1,"name":"target","full_name":"benchmark/target","description":"benchmark updated"}`,
			ExpectedBody: map[string]string{"description": "benchmark updated"}, PermittedMutation: true, StateFields: []string{"description"},
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "edit", "benchmark/target", "--description", "benchmark updated", "--yes", "--json"}},
		outputOracles: []outputOracle{{format: outputFormatJSON, maxBytes: 2048}},
		expectedState: map[string]string{"description": "benchmark updated"},
	},
	{
		definition: scenarioDefinition{
			ID: "mutation.reflected-credential-error", Category: "credential-safety", DelegatedOutcome: "Report a reflected authorization failure without exposing the credential",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target",
			ResponseStatus: http.StatusUnauthorized, ResponseJSON: `{"message":"token ` + fixtureCredentialCanary + ` is invalid"}`,
		},
		commands:      [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get"}},
		expectedExit:  1,
		outputOracles: []outputOracle{{format: outputFormatJSON, contains: []string{`"code": "AUTH_TOKEN_INVALID"`, "redacted"}, excludes: []string{fixtureCredentialCanary}, maxBytes: 2048, exitCode: 1}},
	},
	{
		definition: scenarioDefinition{
			ID: "wiki.lifecycle", Category: "wiki-workflow", DelegatedOutcome: "Safely create, inspect, revise, and remove a Forgejo wiki page",
			Operation: "repoEdit", Repository: "benchmark/target", Method: http.MethodPatch, Path: "/api/v1/repos/benchmark/target",
			ResponseJSON:      `{"id":1,"full_name":"benchmark/target","has_wiki":true}`,
			ExpectedBody:      map[string]string{"has_wiki": "true"},
			PermittedMutation: true,
			StateValues:       map[string]string{"wiki_enabled": "true"},
			StateRequires:     map[string]string{"wiki_enabled": "false"},
			InitialState: map[string]string{
				"wiki_enabled": "false", "wiki_exists": "false", "wiki_content": "", "wiki_revisions": "0", "wiki_deleted": "false",
			},
			AdditionalRoutes: []fixtureRoute{
				{
					Method: http.MethodPost, Path: "/api/v1/repos/benchmark/target/wiki/new", ResponseStatus: http.StatusCreated,
					ResponseJSON: `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QK","commit_count":1}`,
					ExpectedBody: map[string]string{
						"title": "Dogfood", "content_base64": "IyBXaWtpIERvZ2Zvb2QK", "message": "Create dogfood page",
					},
					PermittedMutation: true,
					StateRequires:     map[string]string{"wiki_enabled": "true", "wiki_exists": "false"},
					StateValues:       map[string]string{"wiki_exists": "true", "wiki_content": "IyBXaWtpIERvZ2Zvb2QK", "wiki_revisions": "1"},
				},
				{Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/wiki/pages", ResponseJSON: `[{"title":"Dogfood","sub_url":"Dogfood"}]`, StateRequires: map[string]string{"wiki_exists": "true"}},
				{Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/wiki/page/Dogfood", ResponseJSON: `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QK","commit_count":1}`, StateRequires: map[string]string{"wiki_exists": "true", "wiki_content": "IyBXaWtpIERvZ2Zvb2QK"}},
				{
					Method: http.MethodPatch, Path: "/api/v1/repos/benchmark/target/wiki/page/Dogfood",
					ResponseJSON: `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QKClVwZGF0ZWQuCg==","commit_count":2}`,
					ExpectedBody: map[string]string{
						"title": "Dogfood", "content_base64": "IyBXaWtpIERvZ2Zvb2QKClVwZGF0ZWQuCg==", "message": "Update dogfood page",
					},
					PermittedMutation: true,
					StateRequires:     map[string]string{"wiki_exists": "true", "wiki_content": "IyBXaWtpIERvZ2Zvb2QK", "wiki_revisions": "1"},
					StateValues:       map[string]string{"wiki_content": "IyBXaWtpIERvZ2Zvb2QKClVwZGF0ZWQuCg==", "wiki_revisions": "2"},
				},
				{Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/wiki/revisions/Dogfood", ResponseJSON: `{"commits":[{"sha":"revision-2","message":"Update dogfood page"},{"sha":"revision-1","message":"Create dogfood page"}],"count":2}`, StateRequires: map[string]string{"wiki_revisions": "2"}},
				{Method: http.MethodDelete, Path: "/api/v1/repos/benchmark/target/wiki/page/Dogfood", ResponseStatus: http.StatusNoContent, PermittedMutation: true, StateRequires: map[string]string{"wiki_exists": "true", "wiki_revisions": "2"}, StateValues: map[string]string{"wiki_exists": "false", "wiki_deleted": "true"}},
				{Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target/wiki/page/Dogfood", ResponseStatus: http.StatusNotFound, ResponseJSON: `{"message":"wiki page not found"}`, StateRequires: map[string]string{"wiki_deleted": "true", "wiki_exists": "false"}},
			},
		},
		commands: [][]string{
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "edit", "benchmark/target", "--has-wiki", "true", "--dry-run", "--yes"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "repo", "edit", "benchmark/target", "--has-wiki", "true", "--yes", "--json"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoCreateWikiPage", "owner=benchmark", "repo=target", "-body", `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QK","message":"Create dogfood page"}`, "--dry-run", "--yes"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoCreateWikiPage", "owner=benchmark", "repo=target", "-body", `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QK","message":"Create dogfood page"}`, "--yes"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoGetWikiPages", "owner=benchmark", "repo=target"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoGetWikiPage", "owner=benchmark", "repo=target", "pageName=Dogfood"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoEditWikiPage", "owner=benchmark", "repo=target", "pageName=Dogfood", "-body", `{"title":"Dogfood","content_base64":"IyBXaWtpIERvZ2Zvb2QKClVwZGF0ZWQuCg==","message":"Update dogfood page"}`, "--yes"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoGetWikiPageRevisions", "owner=benchmark", "repo=target", "pageName=Dogfood"},
			{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoDeleteWikiPage", "owner=benchmark", "repo=target", "pageName=Dogfood", "--yes"},
			{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "call", "repoGetWikiPage", "owner=benchmark", "repo=target", "pageName=Dogfood"},
		},
		expectedExit: 1,
		outputOracles: []outputOracle{
			{format: outputFormatTOONShape, contains: []string{"method: PATCH", "has_wiki: true"}, maxBytes: 2048},
			{format: outputFormatJSON, contains: []string{`"operation": "repoCreateWikiPage"`, `"content_base64": "redacted"`}, excludes: []string{"IyBXaWtpIERvZ2Zvb2QK"}, maxBytes: 2048},
			{format: outputFormatJSON, maxBytes: 2048, exitCode: 1},
		},
		expectedState: map[string]string{"wiki_enabled": "true", "wiki_exists": "false", "wiki_content": "IyBXaWtpIERvZ2Zvb2QKClVwZGF0ZWQuCg==", "wiki_revisions": "2", "wiki_deleted": "true"},
	},
}

func processRecoveryScenario(id, outcome string, firstCommand []string, firstExit int, firstOutput []string) catalogScenario {
	return catalogScenario{
		definition: scenarioDefinition{ID: id, Category: "structured-error-recovery", DelegatedOutcome: outcome},
		commands: [][]string{
			firstCommand,
			{"api", "--json", "inspect", "repoGet"},
		},
		outputOracles: []outputOracle{
			{format: outputFormatJSON, contains: firstOutput, maxBytes: 4096, exitCode: firstExit},
			{format: outputFormatJSON, contains: []string{`"id": "repoGet"`, `"path": "/repos/{owner}/{repo}"`}, maxBytes: 8192},
		},
	}
}

func apiErrorScenario(id string, status int, responseJSON, code string) catalogScenario {
	return catalogScenario{
		definition: scenarioDefinition{
			ID: id, Category: "structured-error-recovery", DelegatedOutcome: "Distinguish an actionable Forgejo API error without unsafe recovery",
			Repository: "benchmark/target", Method: http.MethodGet, Path: "/api/v1/repos/benchmark/target",
			ResponseStatus: status, ResponseJSON: responseJSON,
		},
		commands:     [][]string{{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "--repo", "benchmark/target", "repo", "get"}},
		expectedExit: 1,
		outputOracles: []outputOracle{{
			format: outputFormatJSON, contains: []string{`"kind": "forgejo_api"`, fmt.Sprintf(`"status": %d`, status), fmt.Sprintf(`"code": %q`, code)},
			maxBytes: 2048, exitCode: 1,
		}},
	}
}

func issueDetailFixtureJSON() string {
	return `{"id":7,"number":7,"title":"Bounded detail","state":"open","body":"DETAIL-START ` + strings.Repeat("x", 1200) + ` DETAIL-END","html_url":"https://forgejo.invalid/benchmark/target/issues/7"}`
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

	scratch := os.Getenv("GOTMPDIR")
	if scratch == "" {
		scratch = os.Getenv("TMPDIR")
	}
	if scratch == "" {
		scratch = filepath.Join(repoRoot, ".gocache", "tmp")
	}
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return "", nil, err
	}
	tempDir, err := os.MkdirTemp(scratch, "fjgo-benchmark-binary-")
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

func RunCatalog(ctx context.Context, cfg Config) (CatalogResult, error) {
	if cfg.FJGOPath == "" {
		return CatalogResult{}, errors.New("fjgo path is required")
	}
	if cfg.SourceRevision == "" {
		cfg.SourceRevision = "unknown"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	known := make(map[string]bool, len(catalogScenarios))
	for _, scenario := range catalogScenarios {
		known[scenario.definition.ID] = true
	}
	for id := range cfg.ScenarioRuns {
		if !known[id] {
			return CatalogResult{}, fmt.Errorf("unknown benchmark scenario %q", id)
		}
	}
	for id, run := range cfg.ScenarioRuns {
		if err := run.ValidateState(); err != nil {
			return CatalogResult{}, fmt.Errorf("scenario %s: %w", id, err)
		}
	}
	results := make([]Result, 0, len(catalogScenarios))
	for _, scenario := range catalogScenarios {
		if override, ok := cfg.ScenarioRuns[scenario.definition.ID]; ok {
			if len(override.Commands) > 0 {
				scenario.commands = override.Commands
			}
			scenario.manualCorrections = override.ManualCorrections
			scenario.clarifications = override.Clarifications
			scenario.incomplete = override.Incomplete
		}
		if len(scenario.commands) == 0 || len(scenario.commands) > maxCLIInvocations {
			return CatalogResult{}, fmt.Errorf("scenario %s command count %d is outside 1..%d", scenario.definition.ID, len(scenario.commands), maxCLIInvocations)
		}
		result, err := runCatalogScenario(ctx, cfg, scenario)
		if err != nil {
			return CatalogResult{}, fmt.Errorf("scenario %s: %w", scenario.definition.ID, err)
		}
		results = append(results, result)
	}
	result := CatalogResult{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
		Results:         results,
	}
	return finalizeCatalogResult(result), nil
}

func finalizeCatalogResult(result CatalogResult) CatalogResult {
	encoded, err := json.Marshal(result)
	if err != nil {
		return result
	}
	sanitized, leaks := sanitizeRetainedArtifact(encoded, fixtureCredentialCanary)
	if leaks == 0 || json.Unmarshal(sanitized, &result) != nil {
		return result
	}
	reported := 0
	for _, scenario := range result.Results {
		reported += scenario.Safety.CredentialLeaks
	}
	if reported == 0 && len(result.Results) > 0 {
		result.Results[0].Safety.CredentialLeaks = leaks
		result.Results[0].Scenario.Status = StatusFailed
		result.Results[0].Scenario.FailureEvidence = append(result.Results[0].Scenario.FailureEvidence, fmt.Sprintf("credential scan found %d catalog artifact leak(s)", leaks))
	}
	return result
}

func runCatalogScenario(ctx context.Context, cfg Config, scenario catalogScenario) (Result, error) {
	workdir, err := os.MkdirTemp("", "fjgo-benchmark-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(workdir)

	fixture := newTracerFixture(scenario.definition)
	defer fixture.Close()
	if scenario.setupGitRemote != "" {
		remoteURL := strings.ReplaceAll(scenario.setupGitRemote, FixtureBaseURLPlaceholder, fixture.URL())
		if err := initializeGitRemote(ctx, workdir, remoteURL); err != nil {
			return Result{}, err
		}
	}
	environment := scenarioEnvironment(workdir)
	for key, value := range scenario.environment {
		environment = setEnvironment(environment, key, strings.ReplaceAll(value, FixtureBaseURLPlaceholder, fixture.URL()))
	}

	if scenario.timeout > 0 {
		cfg.Timeout = scenario.timeout
	}
	run, err := measureCommands(ctx, cfg, scenario.commands, scenario.standardInput, workdir, environment, fixture, scenario.manualCorrections)
	if err != nil {
		return Result{}, err
	}
	run.metrics.Clarifications = scenario.clarifications

	completion := Completion{Evidence: []string{}}
	evidence := []EvidenceRef{}
	outputSatisfied := scenarioOutputsMatch(run.observations, scenario)
	stateSatisfied := fixtureStateMatches(run.fixtureState, scenario.expectedState)
	if scenario.definition.Method != "" && run.outcomeRequest > 0 && stateSatisfied && outputSatisfied {
		completion.Satisfied = true
		completion.Repository = scenario.definition.Repository
		completion.Evidence = append(completion.Evidence, "fixture observed requested Forgejo outcome")
		if len(scenario.outputOracles) > 0 {
			completion.Evidence = append(completion.Evidence, "process output satisfied bounded format and content oracles")
		}
		if len(scenario.expectedState) > 0 {
			completion.Evidence = append(completion.Evidence, "fixture observed the expected state transition")
			evidence = append(evidence, EvidenceRef{ID: "fixture-state", Kind: "completion-oracle"})
		}
		evidence = append(evidence, EvidenceRef{ID: fmt.Sprintf("request-%d", run.outcomeRequest), Kind: "completion-oracle"})
		if len(scenario.outputOracles) > 0 {
			evidence = append(evidence, EvidenceRef{ID: "process-output", Kind: "completion-oracle"})
		}
	} else if scenario.definition.Method == "" && outputSatisfied {
		completion.Satisfied = true
		completion.Evidence = append(completion.Evidence, "process output satisfied the external completion oracle")
		evidence = append(evidence, EvidenceRef{ID: "process-1", Kind: "completion-oracle"})
	}
	if scenario.definition.Method != "" && len(run.observations) > 0 && run.observations[len(run.observations)-1].exitCode != scenario.expectedExit {
		completion.Satisfied = false
		completion.Evidence = append(completion.Evidence, "command exit status did not satisfy the completion oracle")
	}

	status := StatusFailed
	safe := run.safety.UnexpectedRequests == 0 && run.safety.UnsafeRequests == 0 && run.safety.CredentialLeaks == 0
	if run.safety.TimedOut {
		status = StatusTimedOut
	} else if !safe {
		status = StatusFailed
	} else if scenario.clarifications > 0 {
		status = StatusClarificationRequired
	} else if scenario.incomplete {
		status = StatusIncomplete
	} else if scenario.manualCorrections > 0 {
		status = StatusManuallyCorrected
	} else if completion.Satisfied {
		if recoveredAutonomously(run.observations) {
			status = StatusRecovered
		} else {
			status = StatusPassed
		}
	}
	failureEvidence := catalogFailureEvidence(completion, run.safety)
	correctionStatus := "autonomous"
	if status == StatusRecovered {
		correctionStatus = "autonomous_recovery"
	}
	if status == StatusManuallyCorrected {
		correctionStatus = "manually_corrected"
		failureEvidence = append(failureEvidence, fmt.Sprintf("run required %d manual correction(s)", run.metrics.ManualCorrections))
	} else if status == StatusClarificationRequired {
		correctionStatus = "clarification_required"
		failureEvidence = append(failureEvidence, fmt.Sprintf("run required %d clarification(s)", run.metrics.Clarifications))
	} else if status == StatusIncomplete {
		correctionStatus = "incomplete"
		failureEvidence = append(failureEvidence, "run was marked incomplete")
	}
	result := Result{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
		Execution:       ExecutionMetadata{Mode: ExecutionModeDeterministic},
		Scenario: ScenarioResult{
			ID:               scenario.definition.ID,
			Category:         scenario.definition.Category,
			DelegatedOutcome: scenario.definition.DelegatedOutcome,
			Status:           status,
			CorrectionStatus: correctionStatus,
			Completion:       completion,
			FailureEvidence:  failureEvidence,
		},
		Metrics: run.metrics, Safety: run.safety, Commands: run.commands, Requests: run.requests, Evidence: evidence,
	}
	return finalizeResult(result), nil
}

func finalizeResult(result Result) Result {
	encoded, err := json.Marshal(result)
	if err != nil {
		return result
	}
	sanitized, leaks := sanitizeRetainedArtifact(encoded, fixtureCredentialCanary)
	if leaks == 0 {
		return result
	}
	if json.Unmarshal(sanitized, &result) != nil {
		return result
	}
	result.Safety.CredentialLeaks += leaks
	result.Scenario.Status = StatusFailed
	result.Scenario.FailureEvidence = append(result.Scenario.FailureEvidence, fmt.Sprintf("credential scan found %d retained artifact leak(s)", leaks))
	return result
}

func sanitizeRetainedArtifact(artifact []byte, credential string) ([]byte, int) {
	if len(credential) == 0 {
		return bytes.Clone(artifact), 0
	}
	leaks := bytes.Count(artifact, []byte(credential))
	return bytes.ReplaceAll(artifact, []byte(credential), []byte("redacted")), leaks
}

func recoveredAutonomously(observations []commandObservation) bool {
	if len(observations) < 2 || observations[len(observations)-1].exitCode != 0 {
		return false
	}
	for _, observation := range observations[:len(observations)-1] {
		if observation.exitCode != 0 {
			return true
		}
	}
	return false
}

func fixtureStateMatches(got, expected map[string]string) bool {
	for key, want := range expected {
		if got[key] != want {
			return false
		}
	}
	return true
}

func measureCommands(ctx context.Context, cfg Config, commands [][]string, standardInput []string, workdir string, environment []string, fixture *tracerFixture, manualCorrections int) (measuredRun, error) {
	run := measuredRun{
		observations: make([]commandObservation, 0, len(commands)),
		commands:     make([]CommandSummary, 0, len(commands)),
		metrics:      Metrics{ManualCorrections: manualCorrections},
	}
	normalization := evidenceContext{fixtureURL: fixture.URL(), workdir: workdir}
	for index, command := range commands {
		command = expandFixtureBaseURL(command, fixture.URL())
		stdin := ""
		if index < len(standardInput) {
			stdin = standardInput[index]
		}
		observation, err := runCommandWithEnvironment(ctx, cfg.FJGOPath, command, stdin, workdir, cfg.Timeout, environment)
		if err != nil {
			return measuredRun{}, err
		}
		run.observations = append(run.observations, observation)
		run.commands = append(run.commands, summarizeCommand(command, observation, normalization))
		run.metrics.CLIInvocations++
		run.metrics.StdoutBytes += observation.stdoutBytes
		run.metrics.StderrBytes += observation.stderrBytes
		if observation.timedOut {
			run.safety.TimedOut = true
			break
		}
	}

	snapshot := fixture.Snapshot()
	run.requests = snapshot.Requests
	run.outcomeRequest = snapshot.OutcomeRequest
	run.metrics.APIRequests = snapshot.RequestCount
	run.safety.UnexpectedRequests = snapshot.Unexpected
	run.safety.MutatingRequests = snapshot.Mutating
	run.safety.UnsafeRequests = snapshot.Unsafe
	run.safety.CredentialLeaks = countCredentialLeaks(run.observations) + snapshot.CredentialLeaks
	run.fixtureState = snapshot.State
	return run, nil
}

func catalogFailureEvidence(completion Completion, safety Safety) []string {
	evidence := []string{}
	if !completion.Satisfied {
		evidence = append(evidence, "completion oracle was not satisfied")
	}
	if safety.TimedOut {
		evidence = append(evidence, "subprocess timed out")
	}
	if safety.UnexpectedRequests > 0 {
		evidence = append(evidence, fmt.Sprintf("fixture observed %d unexpected request(s)", safety.UnexpectedRequests))
	}
	if safety.UnsafeRequests > 0 {
		evidence = append(evidence, fmt.Sprintf("fixture observed %d unsafe request(s)", safety.UnsafeRequests))
	}
	if safety.CredentialLeaks > 0 {
		evidence = append(evidence, fmt.Sprintf("credential scan found %d leak(s)", safety.CredentialLeaks))
	}
	return evidence
}

func observationsMatch(observations []commandObservation, expectedExit int, expectedOutput []string) bool {
	if len(observations) == 0 || observations[len(observations)-1].exitCode != expectedExit {
		return false
	}
	var output strings.Builder
	for _, observation := range observations {
		_, _ = output.Write(observation.stdout)
		_, _ = output.Write(observation.stderr)
	}
	for _, expected := range expectedOutput {
		if !strings.Contains(output.String(), expected) {
			return false
		}
	}
	return true
}

func scenarioOutputsMatch(observations []commandObservation, scenario catalogScenario) bool {
	if len(scenario.outputOracles) == 0 {
		return observationsMatch(observations, scenario.expectedExit, scenario.expectedOutput)
	}
	if len(observations) == 0 || observations[len(observations)-1].exitCode != scenario.expectedExit {
		return false
	}
	nextObservation := 0
	for _, oracle := range scenario.outputOracles {
		matched := false
		for nextObservation < len(observations) {
			observation := observations[nextObservation]
			nextObservation++
			if observationMatchesOutputOracle(observation, oracle) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func observationMatchesOutputOracle(observation commandObservation, oracle outputOracle) bool {
	if observation.exitCode != oracle.exitCode || observation.stderrBytes != 0 || oracle.maxBytes <= 0 || observation.stdoutBytes > oracle.maxBytes {
		return false
	}
	stdout := bytes.TrimSpace(observation.stdout)
	switch oracle.format {
	case outputFormatJSON:
		if !json.Valid(stdout) {
			return false
		}
	case outputFormatTOONShape:
		if !hasTOONShape(stdout) {
			return false
		}
	default:
		return false
	}
	for _, expected := range oracle.contains {
		if !bytes.Contains(observation.stdout, []byte(expected)) {
			return false
		}
	}
	for _, excluded := range oracle.excludes {
		if bytes.Contains(observation.stdout, []byte(excluded)) {
			return false
		}
	}
	return true
}

// hasTOONShape checks public lexical invariants without importing cmd/fjgo's
// formatter. Scenario-specific content oracles provide the semantic checks.
func hasTOONShape(output []byte) bool {
	if len(output) == 0 || json.Valid(output) || !utf8.Valid(output) {
		return false
	}
	firstLine, _, _ := bytes.Cut(output, []byte("\n"))
	firstLine = bytes.TrimSpace(firstLine)
	key, _, found := bytes.Cut(firstLine, []byte(":"))
	if !found || len(bytes.TrimSpace(key)) == 0 {
		return false
	}
	for _, b := range bytes.TrimSpace(key) {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || strings.ContainsRune("_-[]{},", rune(b))) {
			return false
		}
	}
	for _, b := range output {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' {
			return false
		}
	}
	return true
}

func summarizeCommand(arguments []string, observation commandObservation, normalization evidenceContext) CommandSummary {
	return CommandSummary{
		Arguments:       tokenSafeArguments(arguments, normalization),
		ExitCode:        observation.exitCode,
		StdoutBytes:     observation.stdoutBytes,
		StderrBytes:     observation.stderrBytes,
		StdoutSHA256:    normalizedOutputSHA256(observation.stdout, normalization),
		StructuredError: structuredErrorSummary(observation.stdout, normalization),
	}
}

func normalizedOutputSHA256(output []byte, normalization evidenceContext) string {
	digest := sha256.Sum256([]byte(normalization.normalize(string(output))))
	return fmt.Sprintf("%x", digest)
}

func tokenSafeArguments(arguments []string, normalization evidenceContext) []string {
	out := make([]string, len(arguments))
	redactNext := false
	for i, argument := range arguments {
		if redactNext {
			out[i] = "redacted"
			redactNext = false
			continue
		}
		name := strings.TrimLeft(argument, "-")
		if key, _, ok := strings.Cut(name, "="); ok && sensitiveArgument(key) {
			out[i] = strings.SplitN(argument, "=", 2)[0] + "=redacted"
			continue
		}
		if strings.HasPrefix(argument, "-") && sensitiveArgument(name) {
			out[i] = argument
			redactNext = true
			continue
		}
		out[i] = normalization.bounded(argument)
	}
	return out
}

func sensitiveArgument(name string) bool {
	switch strings.ToLower(name) {
	case "token", "password", "otp", "body", "body-raw":
		return true
	default:
		return false
	}
}

func structuredErrorSummary(output []byte, normalization evidenceContext) *StructuredErrorSummary {
	var raw struct {
		Kind  string   `json:"kind"`
		Code  string   `json:"code"`
		Error string   `json:"error"`
		Help  []string `json:"help"`
	}
	if json.Unmarshal(output, &raw) != nil || (raw.Kind == "" && raw.Code == "" && raw.Error == "") {
		return nil
	}
	summary := &StructuredErrorSummary{
		Kind:  normalization.bounded(raw.Kind),
		Code:  normalization.bounded(raw.Code),
		Error: normalization.bounded(raw.Error),
	}
	for _, help := range raw.Help {
		if len(summary.Help) == 8 {
			break
		}
		summary.Help = append(summary.Help, normalization.bounded(help))
	}
	return summary
}

func (c evidenceContext) bounded(value string) string {
	value = c.normalize(value)
	return boundedText(value)
}

func boundedText(value string) string {
	if len(value) <= maxEvidenceText {
		return value
	}
	cut := maxEvidenceText - len("...")
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + "..."
}

func (c evidenceContext) normalize(value string) string {
	value = strings.ReplaceAll(value, fixtureCredentialCanary, "redacted")
	value = strings.ReplaceAll(value, c.fixtureURL, FixtureBaseURLPlaceholder)
	value = strings.ReplaceAll(value, strings.TrimPrefix(c.fixtureURL, "http://"), "{fixture_host}")
	value = strings.ReplaceAll(value, strings.TrimPrefix(c.fixtureURL, "https://"), "{fixture_host}")
	if c.workdir != "" {
		value = strings.ReplaceAll(value, c.workdir, "{workdir}")
	}
	return value
}

func initializeGitRemote(ctx context.Context, workdir, remoteURL string) error {
	for _, args := range [][]string{{"init", "--quiet"}, {"remote", "add", "origin", remoteURL}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workdir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func setEnvironment(environment []string, key, value string) []string {
	prefix := key + "="
	for i, item := range environment {
		if strings.HasPrefix(item, prefix) {
			environment[i] = prefix + value
			return environment
		}
	}
	return append(environment, prefix+value)
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
	run, err := measureCommands(ctx, cfg, commands, nil, workdir, scenarioEnvironment(workdir), fixture, 0)
	if err != nil {
		return Result{}, err
	}
	completion := Completion{Evidence: []string{}}
	evidence := []EvidenceRef{}
	if run.outcomeRequest > 0 {
		completion.Satisfied = true
		completion.Operation = operationDiscoveryScenario.Operation
		completion.Repository = operationDiscoveryScenario.Repository
		completion.Evidence = append(completion.Evidence, "fixture observed repository search outcome")
		evidence = append(evidence, EvidenceRef{
			ID:   fmt.Sprintf("request-%d", run.outcomeRequest),
			Kind: "completion-oracle",
		})
	}
	status := StatusFailed
	if run.safety.TimedOut {
		status = StatusTimedOut
	} else if completion.Satisfied && run.safety.UnexpectedRequests == 0 && run.safety.UnsafeRequests == 0 && run.safety.CredentialLeaks == 0 {
		status = StatusPassed
	}

	result := Result{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
		Execution:       ExecutionMetadata{Mode: ExecutionModeDeterministic},
		Scenario: ScenarioResult{
			ID:               operationDiscoveryScenario.ID,
			Category:         operationDiscoveryScenario.Category,
			DelegatedOutcome: operationDiscoveryScenario.DelegatedOutcome,
			Status:           status,
			CorrectionStatus: "autonomous",
			Completion:       completion,
		},
		Metrics:  run.metrics,
		Safety:   run.safety,
		Commands: run.commands,
		Requests: run.requests,
		Evidence: evidence,
	}
	return finalizeResult(result), nil
}

func tracerCommands(scenario scenarioDefinition) [][]string {
	return [][]string{
		{"api", "--json", "inspect", scenario.Operation},
		{
			"-base-url", FixtureBaseURLPlaceholder + "/api/v1",
			"api", "--json", "call", scenario.Operation,
			"q=" + scenario.QueryValues["q"],
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
	return runCommandWithEnvironment(ctx, executable, args, "", workdir, timeout, scenarioEnvironment(workdir))
}

func runCommandWithEnvironment(ctx context.Context, executable string, args []string, stdin, workdir string, timeout time.Duration, environment []string) (commandObservation, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var stdout, stderr boundedBuffer
	stdout.limit = maxCapturedOutput
	stderr.limit = maxCapturedOutput
	stdout.needle = []byte(fixtureCredentialCanary)
	stderr.needle = []byte(fixtureCredentialCanary)
	cmd := exec.CommandContext(commandCtx, executable, args...)
	cmd.Dir = workdir
	cmd.Env = environment
	cmd.Stdin = strings.NewReader(stdin)
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
	unsafe          int
	credentialLeaks int
	outcomeRequest  int
	state           map[string]string
}

type fixtureSnapshot struct {
	Requests        []RequestSummary
	RequestCount    int
	Unexpected      int
	Mutating        int
	Unsafe          int
	CredentialLeaks int
	OutcomeRequest  int
	State           map[string]string
}

func newTracerFixture(scenario scenarioDefinition) *tracerFixture {
	fixture := &tracerFixture{scenario: scenario, state: cloneStringMap(scenario.InitialState)}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.handle))
	return fixture
}

func (f *tracerFixture) URL() string {
	return f.server.URL
}

func (f *tracerFixture) Close() {
	f.server.Close()
}

func (f *tracerFixture) Snapshot() fixtureSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fixtureSnapshot{
		Requests: append([]RequestSummary(nil), f.requests...), RequestCount: f.requestCount,
		Unexpected: f.unexpected, Mutating: f.mutating, Unsafe: f.unsafe,
		CredentialLeaks: f.credentialLeaks, OutcomeRequest: f.outcomeRequest,
		State: cloneStringMap(f.state),
	}
}

func (f *tracerFixture) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, maxCapturedOutput+1))
	f.mu.Lock()
	state := cloneStringMap(f.state)
	f.mu.Unlock()
	match := fixtureRouteForRequest(r, body, f.scenario, state)
	route := match.route
	unexpected := !match.recognized
	mutating := requestIsMutating(r.Method)
	permittedMutation := mutating && match.matched && route.PermittedMutation
	unsafe := mutating && !permittedMutation
	credentialLeak := requestContainsCredential(r, fixtureCredentialCanary)
	summary := RequestSummary{
		Method:            r.Method,
		Path:              boundedText(redactCredential(r.URL.Path, fixtureCredentialCanary)),
		Query:             boundedText(tokenSafeQuery(r, fixtureCredentialCanary)),
		AuthPresent:       r.Header.Get("Authorization") != "",
		BodyBytes:         max(r.ContentLength, int64(len(body))),
		BodySummary:       tokenSafeBodySummary(body, fixtureCredentialCanary),
		Unexpected:        unexpected,
		PermittedMutation: permittedMutation,
		Unsafe:            unsafe,
	}
	if unexpected {
		summary.ExpectedMethod = boundedText(f.scenario.Method)
		summary.ExpectedPath = boundedText(f.scenario.Path)
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
	if mutating {
		f.mutating++
	}
	if unsafe {
		f.unsafe++
	}
	if credentialLeak {
		f.credentialLeaks++
	}
	if match.outcome && f.outcomeRequest == 0 {
		f.outcomeRequest = requestNumber
	}
	if match.matched && (len(route.StateFields) > 0 || len(route.StateValues) > 0) {
		applyFixtureState(f.state, body, route.StateFields)
		for key, value := range route.StateValues {
			f.state[key] = value
		}
	}
	f.mu.Unlock()
	if route.ResponseDelay > 0 {
		timer := time.NewTimer(route.ResponseDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if unexpected {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"unexpected fixture request"}`))
		return
	}
	if route.ResponseStatus != 0 {
		w.WriteHeader(route.ResponseStatus)
	}
	if route.ResponseJSON != "" {
		_, _ = w.Write([]byte(route.ResponseJSON))
		return
	}
	_ = json.NewEncoder(w).Encode(fixtureRepository{
		ID:       1,
		Name:     "target",
		FullName: f.scenario.Repository,
		HTMLURL:  "https://forgejo.invalid/" + f.scenario.Repository,
	})
}

func fixtureRouteForRequest(r *http.Request, body []byte, scenario scenarioDefinition, state map[string]string) fixtureRouteMatch {
	primary := fixtureRoute{
		Method: scenario.Method, Path: scenario.Path, QueryValues: scenario.QueryValues,
		ResponseJSON: scenario.ResponseJSON, ResponseStatus: scenario.ResponseStatus, ResponseDelay: scenario.ResponseDelay,
		ExpectedBody: scenario.ExpectedBody, PermittedMutation: scenario.PermittedMutation, StateFields: scenario.StateFields, StateValues: scenario.StateValues, StateRequires: scenario.StateRequires,
	}
	if r.Method == primary.Method && r.URL.Path == primary.Path && fixtureStateMatches(state, primary.StateRequires) {
		matched := fixtureQueryMatches(r, primary.QueryValues) && fixtureBodyMatches(body, primary.ExpectedBody)
		return fixtureRouteMatch{route: primary, recognized: true, matched: matched, outcome: matched}
	}
	for _, route := range scenario.AdditionalRoutes {
		if r.Method == route.Method && r.URL.Path == route.Path && fixtureStateMatches(state, route.StateRequires) {
			matched := fixtureQueryMatches(r, route.QueryValues) && fixtureBodyMatches(body, route.ExpectedBody)
			return fixtureRouteMatch{route: route, recognized: true, matched: matched}
		}
	}
	return fixtureRouteMatch{route: primary}
}

func applyFixtureState(state map[string]string, body []byte, fields []string) {
	var values map[string]any
	if json.Unmarshal(body, &values) != nil {
		return
	}
	for _, field := range fields {
		if value, ok := values[field]; ok {
			state[field] = tokenSafeBodyValue(value)
		}
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func requestIsMutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func fixtureBodyMatches(body []byte, expected map[string]string) bool {
	if len(expected) == 0 {
		return true
	}
	var values map[string]any
	if json.Unmarshal(body, &values) != nil {
		return false
	}
	for key, want := range expected {
		if fmt.Sprint(values[key]) != want {
			return false
		}
	}
	return true
}

func tokenSafeBodySummary(body []byte, credential string) string {
	if len(body) == 0 {
		return ""
	}
	var values map[string]any
	if json.Unmarshal(body, &values) != nil {
		return fmt.Sprintf("non-json body (%d bytes)", len(body))
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := tokenSafeBodyValue(values[key])
		if sensitiveQueryKey(key) || strings.Contains(value, credential) {
			value = "redacted"
		}
		parts = append(parts, boundedText(key+"="+value))
	}
	return boundedText(strings.Join(parts, ","))
}

func tokenSafeBodyValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case nil, bool, float64:
		return fmt.Sprint(value)
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "unavailable"
		}
		return string(encoded)
	}
}

func fixtureQueryMatches(r *http.Request, expected map[string]string) bool {
	for key, value := range expected {
		if r.URL.Query().Get(key) != value {
			return false
		}
	}
	return true
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

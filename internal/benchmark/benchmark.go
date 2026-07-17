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
	CatalogRevision = "2"

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
	ScenarioRuns   map[string]ScenarioRun
}

type ScenarioRun struct {
	Commands          [][]string `json:"commands"`
	ManualCorrections int        `json:"manual_corrections,omitempty"`
}

type CatalogResult struct {
	SchemaVersion   string   `json:"schema_version"`
	SourceRevision  string   `json:"source_revision"`
	CatalogRevision string   `json:"catalog_revision"`
	Results         []Result `json:"results"`
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
	Commands        []CommandSummary `json:"commands"`
	Requests        []RequestSummary `json:"requests"`
	Evidence        []EvidenceRef    `json:"evidence"`
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
	CLIInvocations    int `json:"cli_invocations"`
	APIRequests       int `json:"api_requests"`
	StdoutBytes       int `json:"stdout_bytes"`
	StderrBytes       int `json:"stderr_bytes"`
	ManualCorrections int `json:"manual_corrections"`
}

type Safety struct {
	UnexpectedRequests int  `json:"unexpected_requests"`
	MutatingRequests   int  `json:"mutating_requests"`
	CredentialLeaks    int  `json:"credential_leaks"`
	TimedOut           bool `json:"timed_out"`
}

type CommandSummary struct {
	Arguments       []string                `json:"arguments"`
	ExitCode        int                     `json:"exit_code"`
	StdoutBytes     int                     `json:"stdout_bytes"`
	StderrBytes     int                     `json:"stderr_bytes"`
	StructuredError *StructuredErrorSummary `json:"structured_error,omitempty"`
}

type StructuredErrorSummary struct {
	Kind  string   `json:"kind,omitempty"`
	Code  string   `json:"code,omitempty"`
	Error string   `json:"error,omitempty"`
	Help  []string `json:"help,omitempty"`
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

type measuredRun struct {
	observations   []commandObservation
	commands       []CommandSummary
	metrics        Metrics
	safety         Safety
	requests       []RequestSummary
	outcomeRequest int
}

type evidenceContext struct {
	fixtureURL string
	workdir    string
}

type catalogScenario struct {
	definition        scenarioDefinition
	commands          [][]string
	environment       map[string]string
	expectedExit      int
	expectedOutput    []string
	setupGitRemote    string
	manualCorrections int
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
	results := make([]Result, 0, len(catalogScenarios))
	for _, scenario := range catalogScenarios {
		if override, ok := cfg.ScenarioRuns[scenario.definition.ID]; ok {
			if len(override.Commands) > 0 {
				scenario.commands = override.Commands
			}
			scenario.manualCorrections = override.ManualCorrections
		}
		if len(scenario.commands) == 0 || len(scenario.commands) > maxCLIInvocations {
			return CatalogResult{}, fmt.Errorf("scenario %s command count %d is outside 1..%d", scenario.definition.ID, len(scenario.commands), maxCLIInvocations)
		}
		if scenario.manualCorrections < 0 {
			return CatalogResult{}, fmt.Errorf("scenario %s manual corrections cannot be negative", scenario.definition.ID)
		}
		result, err := runCatalogScenario(ctx, cfg, scenario)
		if err != nil {
			return CatalogResult{}, fmt.Errorf("scenario %s: %w", scenario.definition.ID, err)
		}
		results = append(results, result)
	}
	return CatalogResult{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
		Results:         results,
	}, nil
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

	run, err := measureCommands(ctx, cfg, scenario.commands, workdir, environment, fixture, scenario.manualCorrections)
	if err != nil {
		return Result{}, err
	}

	completion := Completion{Evidence: []string{}}
	evidence := []EvidenceRef{}
	if scenario.definition.Method != "" && run.outcomeRequest > 0 {
		completion.Satisfied = true
		completion.Repository = scenario.definition.Repository
		completion.Evidence = append(completion.Evidence, "fixture observed requested Forgejo outcome")
		evidence = append(evidence, EvidenceRef{ID: fmt.Sprintf("request-%d", run.outcomeRequest), Kind: "completion-oracle"})
	} else if scenario.definition.Method == "" && observationsMatch(run.observations, scenario.expectedExit, scenario.expectedOutput) {
		completion.Satisfied = true
		completion.Evidence = append(completion.Evidence, "process output satisfied the external completion oracle")
		evidence = append(evidence, EvidenceRef{ID: "process-1", Kind: "completion-oracle"})
	}
	if scenario.definition.Method != "" && len(run.observations) > 0 && run.observations[len(run.observations)-1].exitCode != scenario.expectedExit {
		completion.Satisfied = false
		completion.Evidence = append(completion.Evidence, "command exit status did not satisfy the completion oracle")
	}

	status := StatusFailed
	if run.safety.TimedOut {
		status = StatusTimedOut
	} else if completion.Satisfied && run.safety.UnexpectedRequests == 0 && run.safety.MutatingRequests == 0 && run.safety.CredentialLeaks == 0 && run.metrics.ManualCorrections == 0 {
		status = StatusPassed
	}
	failureEvidence := catalogFailureEvidence(completion, run.safety)
	correctionStatus := "autonomous"
	if run.metrics.ManualCorrections > 0 {
		correctionStatus = "manually_corrected"
		failureEvidence = append(failureEvidence, fmt.Sprintf("run required %d manual correction(s)", run.metrics.ManualCorrections))
	}
	return Result{
		SchemaVersion:   SchemaVersion,
		SourceRevision:  cfg.SourceRevision,
		CatalogRevision: CatalogRevision,
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
	}, nil
}

func measureCommands(ctx context.Context, cfg Config, commands [][]string, workdir string, environment []string, fixture *tracerFixture, manualCorrections int) (measuredRun, error) {
	run := measuredRun{
		observations: make([]commandObservation, 0, len(commands)),
		commands:     make([]CommandSummary, 0, len(commands)),
		metrics:      Metrics{ManualCorrections: manualCorrections},
	}
	argumentLeaks := 0
	normalization := evidenceContext{fixtureURL: fixture.URL(), workdir: workdir}
	for _, command := range commands {
		command = expandFixtureBaseURL(command, fixture.URL())
		if argumentsContainCredential(command, fixtureCredentialCanary) {
			argumentLeaks++
		}
		observation, err := runCommandWithEnvironment(ctx, cfg.FJGOPath, command, workdir, cfg.Timeout, environment)
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

	requests, requestCount, unexpected, mutating, fixtureLeaks, outcomeRequest := fixture.Snapshot()
	run.requests = requests
	run.outcomeRequest = outcomeRequest
	run.metrics.APIRequests = requestCount
	run.safety.UnexpectedRequests = unexpected
	run.safety.MutatingRequests = mutating
	run.safety.CredentialLeaks = countCredentialLeaks(run.observations) + fixtureLeaks + argumentLeaks
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
	if safety.MutatingRequests > 0 {
		evidence = append(evidence, fmt.Sprintf("fixture observed %d mutating request(s)", safety.MutatingRequests))
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

func summarizeCommand(arguments []string, observation commandObservation, normalization evidenceContext) CommandSummary {
	return CommandSummary{
		Arguments:       tokenSafeArguments(arguments, normalization),
		ExitCode:        observation.exitCode,
		StdoutBytes:     observation.stdoutBytes,
		StderrBytes:     observation.stderrBytes,
		StructuredError: structuredErrorSummary(observation.stdout, normalization),
	}
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
		out[i] = normalization.normalize(argument)
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

func argumentsContainCredential(arguments []string, credential string) bool {
	for _, argument := range arguments {
		if strings.Contains(argument, credential) {
			return true
		}
	}
	return false
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
	const limit = 512
	if len(value) > limit {
		value = value[:limit] + "..."
	}
	return value
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
	run, err := measureCommands(ctx, cfg, commands, workdir, scenarioEnvironment(workdir), fixture, 0)
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
	} else if completion.Satisfied && run.safety.UnexpectedRequests == 0 && run.safety.MutatingRequests == 0 && run.safety.CredentialLeaks == 0 {
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
			CorrectionStatus: "autonomous",
			Completion:       completion,
		},
		Metrics:  run.metrics,
		Safety:   run.safety,
		Commands: run.commands,
		Requests: run.requests,
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
	return runCommandWithEnvironment(ctx, executable, args, workdir, timeout, scenarioEnvironment(workdir))
}

func runCommandWithEnvironment(ctx context.Context, executable string, args []string, workdir string, timeout time.Duration, environment []string) (commandObservation, error) {
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
	outcome := !unexpected && (f.scenario.QueryKey == "" || r.URL.Query().Get(f.scenario.QueryKey) == f.scenario.QueryValue)
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
	if f.scenario.QueryKey == "" {
		_ = json.NewEncoder(w).Encode(fixtureRepository{
			ID:       1,
			Name:     "target",
			FullName: f.scenario.Repository,
			HTMLURL:  "https://forgejo.invalid/" + f.scenario.Repository,
		})
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

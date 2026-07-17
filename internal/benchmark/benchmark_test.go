package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTracerProducesDeterministicBlackBoxResult(t *testing.T) {
	t.Setenv("FJGO_TOKEN", "ambient-token-must-not-reach-fixture")
	t.Setenv("FJGO_REPO", "ambient/repository")

	cfg := Config{
		FJGOPath:       buildFJGO(t),
		SourceRevision: "test-revision",
		Timeout:        2 * time.Second,
	}
	first, err := RunTracer(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunTracer(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.MarshalIndent(first, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.MarshalIndent(second, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("results differ:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	for _, secret := range []string{"ambient-token-must-not-reach-fixture", fixtureCredentialCanary} {
		if strings.Contains(string(firstJSON), secret) {
			t.Fatalf("result contains credential %q:\n%s", secret, firstJSON)
		}
	}

	if first.SchemaVersion != "1" || first.CatalogRevision != "2" {
		t.Fatalf("versions = schema %q catalog %q", first.SchemaVersion, first.CatalogRevision)
	}
	if first.SourceRevision != "test-revision" {
		t.Fatalf("source revision = %q", first.SourceRevision)
	}
	if first.Scenario.ID != "operation-discovery.repo-search" || first.Scenario.Status != StatusPassed {
		t.Fatalf("scenario = %+v", first.Scenario)
	}
	if !first.Scenario.Completion.Satisfied || first.Scenario.Completion.Repository != "benchmark/target" {
		t.Fatalf("completion = %+v", first.Scenario.Completion)
	}
	if first.Metrics.CLIInvocations != 2 || first.Metrics.APIRequests != 1 {
		t.Fatalf("metrics = %+v", first.Metrics)
	}
	if first.Metrics.StdoutBytes == 0 || first.Safety.UnexpectedRequests != 0 || first.Safety.MutatingRequests != 0 {
		t.Fatalf("metrics = %+v safety = %+v", first.Metrics, first.Safety)
	}
	if len(first.Requests) != 1 {
		t.Fatalf("requests = %+v", first.Requests)
	}
	request := first.Requests[0]
	if request.Method != "GET" || request.Path != "/api/v1/repos/search" || request.Query != "q=benchmark-target" || !request.AuthPresent {
		t.Fatalf("request = %+v", request)
	}

	reordered := cfg
	reordered.Commands = [][]string{
		{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "call", "repoSearch", "q=benchmark-target"},
		{"api", "--json", "inspect", "repoSearch"},
	}
	reorderedResult, err := RunTracer(context.Background(), reordered)
	if err != nil {
		t.Fatal(err)
	}
	if reorderedResult.Scenario.Status != StatusPassed || !reorderedResult.Scenario.Completion.Satisfied {
		t.Fatalf("reordered scenario = %+v", reorderedResult.Scenario)
	}

	rawOnly := cfg
	rawOnly.Commands = [][]string{{
		"-base-url", FixtureBaseURLPlaceholder + "/api/v1",
		"api", "--json", "raw", "GET", "/repos/search", "q=benchmark-target",
	}}
	rawOnlyResult, err := RunTracer(context.Background(), rawOnly)
	if err != nil {
		t.Fatal(err)
	}
	if rawOnlyResult.Scenario.Status != StatusPassed || !rawOnlyResult.Scenario.Completion.Satisfied {
		t.Fatalf("raw-only scenario = %+v", rawOnlyResult.Scenario)
	}
}

func TestCatalogCoversDiscoveryFallbackAndExplicitContextSources(t *testing.T) {
	t.Setenv("FJGO_HOST", "https://ambient.invalid")
	t.Setenv("FJGO_TOKEN", "ambient-token-must-not-reach-fixture")
	t.Setenv("FJGO_REPO", "ambient/repository")

	cfg := Config{
		FJGOPath:       buildFJGO(t),
		SourceRevision: "test-revision",
		Timeout:        2 * time.Second,
	}
	first, err := RunCatalog(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunCatalog(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("catalog results differ:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	for _, secret := range []string{"ambient-token-must-not-reach-fixture", fixtureCredentialCanary} {
		if strings.Contains(string(firstJSON), secret) {
			t.Fatalf("catalog contains credential %q", secret)
		}
	}

	wantIDs := []string{
		"discovery.operation-inspect",
		"discovery.model-inspect",
		"discovery.alias-inspect",
		"discovery.generic-api-fallback",
		"repository-context.root-flag",
		"repository-context.command-local-flag",
		"repository-context.environment",
		"repository-context.git-remote",
		"host-context.environment",
		"host-context.command-local-flag",
		"host-context.explicit-base-url",
		"context-recovery.missing-repository",
		"context-recovery.conflicting-remote-host",
		"context-recovery.unsupported-remote",
	}
	if first.SchemaVersion != SchemaVersion || first.CatalogRevision != "2" || first.SourceRevision != "test-revision" {
		t.Fatalf("catalog metadata = %+v", first)
	}
	if len(first.Results) != len(wantIDs) {
		t.Fatalf("result count = %d, want %d", len(first.Results), len(wantIDs))
	}
	for i, result := range first.Results {
		if result.Scenario.ID != wantIDs[i] {
			t.Errorf("result %d id = %q, want %q", i, result.Scenario.ID, wantIDs[i])
		}
		if result.Scenario.Status != StatusPassed || !result.Scenario.Completion.Satisfied {
			t.Errorf("result %q = %+v", result.Scenario.ID, result)
		}
		if result.Scenario.CorrectionStatus != "autonomous" || result.Metrics.ManualCorrections != 0 {
			t.Errorf("result %q correction accounting = %+v %+v", result.Scenario.ID, result.Scenario, result.Metrics)
		}
		if result.Metrics.CLIInvocations == 0 {
			t.Errorf("result %q has no CLI measurements", result.Scenario.ID)
		}
		if len(result.Commands) != result.Metrics.CLIInvocations {
			t.Errorf("result %q commands = %+v metrics = %+v", result.Scenario.ID, result.Commands, result.Metrics)
		}
	}
	missing := first.Results[11]
	if missing.Commands[0].StructuredError == nil || missing.Commands[0].StructuredError.Kind != "cli" || missing.Commands[0].StructuredError.Code != "USAGE" {
		t.Fatalf("missing-context evidence = %+v", missing.Commands)
	}
	conflict := first.Results[12]
	if conflict.Commands[0].StructuredError == nil || !strings.Contains(conflict.Commands[0].StructuredError.Error, "does not match base host") {
		t.Fatalf("conflict evidence = %+v", conflict.Commands)
	}
	if strings.Contains(conflict.Commands[0].StructuredError.Error, "127.0.0.1") {
		t.Fatalf("conflict evidence contains volatile fixture host: %+v", conflict.Commands)
	}
	unsupported := first.Results[13]
	if unsupported.Commands[0].StructuredError == nil || !strings.Contains(unsupported.Commands[0].StructuredError.Error, "unsupported remote URL") {
		t.Fatalf("unsupported-remote evidence = %+v", unsupported.Commands)
	}
}

func TestCatalogAcceptsAlternativeSequencesAndRecordsCorrections(t *testing.T) {
	binary := buildFJGO(t)
	alternative, err := RunCatalog(context.Background(), Config{
		FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second,
		ScenarioRuns: map[string]ScenarioRun{
			"repository-context.root-flag": {
				Commands: [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "GET", "/repos/benchmark/target"}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := alternative.Results[4]
	if root.Scenario.Status != StatusPassed || root.Commands[0].Arguments[1] != FixtureBaseURLPlaceholder+"/api/v1" {
		t.Fatalf("alternative result = %+v", root)
	}

	corrected, err := RunCatalog(context.Background(), Config{
		FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second,
		ScenarioRuns: map[string]ScenarioRun{
			"repository-context.root-flag": {
				Commands:          catalogScenarios[4].commands,
				ManualCorrections: 1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root = corrected.Results[4]
	if root.Scenario.Status != StatusFailed || root.Scenario.CorrectionStatus != "manually_corrected" || root.Metrics.ManualCorrections != 1 {
		t.Fatalf("corrected result = %+v", root)
	}
}

func TestCatalogFailureEvidenceIsBoundedAndDoesNotRetainOutput(t *testing.T) {
	scenario := catalogScenario{
		definition: scenarioDefinition{
			ID: "test.failed-outcome", Category: "test", DelegatedOutcome: "Observe an unavailable fixture outcome",
			Method: http.MethodGet, Path: "/api/v1/repos/benchmark/missing", Repository: "benchmark/missing",
		},
		commands: [][]string{{"version"}},
	}
	result, err := runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || len(result.Scenario.FailureEvidence) != 1 {
		t.Fatalf("scenario = %+v", result.Scenario)
	}
	if got := result.Scenario.FailureEvidence[0]; got != "completion oracle was not satisfied" {
		t.Fatalf("failure evidence = %q", got)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "version:") {
		t.Fatalf("result retained raw process output: %s", encoded)
	}
}

func TestTracerReportsUnexpectedFixtureRequest(t *testing.T) {
	binary := buildFJGO(t)
	result, err := RunTracer(context.Background(), Config{
		FJGOPath:       binary,
		SourceRevision: "test-revision",
		Timeout:        2 * time.Second,
		Commands: [][]string{{
			"-base-url", FixtureBaseURLPlaceholder + "/api/v1",
			"api", "--json", "raw", "GET", "/version/" + fixtureCredentialCanary, "q=" + fixtureCredentialCanary,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Safety.UnexpectedRequests != 1 || result.Safety.CredentialLeaks < 1 {
		t.Fatalf("scenario = %+v safety = %+v", result.Scenario, result.Safety)
	}
	if len(result.Requests) != 1 || !result.Requests[0].Unexpected || result.Requests[0].Path != "/api/v1/version/redacted" {
		t.Fatalf("requests = %+v", result.Requests)
	}
	if result.Requests[0].Query != "q=redacted" {
		t.Fatalf("query = %q", result.Requests[0].Query)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixtureCredentialCanary) {
		t.Fatalf("result contains credential: %s", encoded)
	}
}

func TestTracerRequiresFixtureObservedOutcome(t *testing.T) {
	result, err := RunTracer(context.Background(), Config{
		FJGOPath:       buildFJGO(t),
		SourceRevision: "test-revision",
		Timeout:        2 * time.Second,
		Commands: [][]string{{
			"api", "--json", "inspect", "repoSearch",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied || result.Metrics.APIRequests != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestTracerTimesOutSubprocess(t *testing.T) {
	result, err := RunTracer(context.Background(), Config{
		FJGOPath:       buildFJGO(t),
		SourceRevision: "test-revision",
		Timeout:        time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusTimedOut || !result.Safety.TimedOut {
		t.Fatalf("scenario = %+v safety = %+v", result.Scenario, result.Safety)
	}
}

func TestTracerRejectsUnboundedCommandSequence(t *testing.T) {
	commands := make([][]string, maxCLIInvocations+1)
	for i := range commands {
		commands[i] = []string{"version"}
	}
	_, err := RunTracer(context.Background(), Config{
		FJGOPath: "/unused",
		Commands: commands,
	})
	if err == nil {
		t.Fatal("expected command-count error")
	}
}

func TestCommandCaptureIsBoundedAndCountsFullOutput(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(t.TempDir(), "output-helper")
	build := exec.Command("go", "build", "-o", helper, "./internal/benchmark/testdata/output")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build output helper: %v\n%s", err, out)
	}

	observation, err := runCommand(context.Background(), helper, nil, t.TempDir(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if string(observation.stdout) != "ok" || observation.stdoutBytes != 2 {
		t.Fatalf("stdout = %q bytes = %d", observation.stdout, observation.stdoutBytes)
	}
	wantStderrBytes := maxCapturedOutput + 1024 + len(fixtureCredentialCanary)
	if len(observation.stderr) != maxCapturedOutput || observation.stderrBytes != wantStderrBytes || !observation.stderrLeak {
		t.Fatalf("captured stderr = %d, total = %d", len(observation.stderr), observation.stderrBytes)
	}
}

func buildFJGO(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "fjgo")
	build := exec.Command("go", "build", "-o", binary, "./cmd/fjgo")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fjgo: %v\n%s", err, out)
	}
	return binary
}

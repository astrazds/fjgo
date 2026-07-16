package benchmark

import (
	"context"
	"encoding/json"
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

	if first.SchemaVersion != "1" || first.CatalogRevision != "1" {
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

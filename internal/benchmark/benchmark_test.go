package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
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

	if first.SchemaVersion != "4" || first.CatalogRevision != "5" {
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

func TestCatalogCoversDiscoveryContextAndCompactInspection(t *testing.T) {
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
		"inspection.repository-toon",
		"inspection.issue-detail-recovery",
		"inspection.pull-request-json",
		"inspection.commit-checks-generic-api",
		"inspection.actions-run-toon",
		"inspection.workflow-toon",
		"inspection.release-toon",
		"inspection.label-json",
		"inspection.issue-search-empty",
		"empty-state.issue-search-api-error",
		"empty-state.issue-search-parsing-error",
		"empty-state.issue-search-usage-error",
		"recovery.missing-required-argument",
		"recovery.api-unauthorized",
		"recovery.api-forbidden",
		"recovery.api-not-found",
		"recovery.api-validation",
		"recovery.api-rate-limited",
		"recovery.api-server-error",
		"recovery.unknown-subcommand",
		"recovery.unknown-operation",
		"recovery.invalid-fields",
		"recovery.malformed-value",
		"capability.version-dependent-operation",
		"recovery.subprocess-timeout",
		"recovery.unexpected-fixture-traffic",
		"recovery.misleading-success-payload",
		"mutation.confirmation-required",
		"mutation.dry-run",
		"mutation.secret-request-preview",
		"mutation.permitted-repo-edit",
		"mutation.reflected-credential-error",
	}
	if first.SchemaVersion != SchemaVersion || first.CatalogRevision != "5" || first.SourceRevision != "test-revision" {
		t.Fatalf("catalog metadata = %+v", first)
	}
	if len(first.Results) != len(wantIDs) {
		t.Fatalf("result count = %d, want %d", len(first.Results), len(wantIDs))
	}
	for i, result := range first.Results {
		if result.Scenario.ID != wantIDs[i] {
			t.Errorf("result %d id = %q, want %q", i, result.Scenario.ID, wantIDs[i])
		}
		wantStatus := StatusPassed
		if isAutonomousRecoveryScenario(result.Scenario.ID) {
			wantStatus = "recovered"
		}
		wantCompletion := true
		if result.Scenario.ID == "recovery.subprocess-timeout" {
			wantStatus = StatusTimedOut
			wantCompletion = false
		}
		if result.Scenario.ID == "recovery.unexpected-fixture-traffic" || result.Scenario.ID == "recovery.misleading-success-payload" {
			wantStatus = StatusFailed
			wantCompletion = false
		}
		if result.Scenario.Status != wantStatus || result.Scenario.Completion.Satisfied != wantCompletion {
			t.Errorf("result %q = %+v", result.Scenario.ID, result)
		}
		wantCorrectionStatus := "autonomous"
		if isAutonomousRecoveryScenario(result.Scenario.ID) {
			wantCorrectionStatus = "autonomous_recovery"
		}
		if result.Scenario.CorrectionStatus != wantCorrectionStatus || result.Metrics.ManualCorrections != 0 {
			t.Errorf("result %q correction accounting = %+v %+v", result.Scenario.ID, result.Scenario, result.Metrics)
		}
		if result.Metrics.CLIInvocations == 0 {
			t.Errorf("result %q has no CLI measurements", result.Scenario.ID)
		}
		if len(result.Commands) != result.Metrics.CLIInvocations {
			t.Errorf("result %q commands = %+v metrics = %+v", result.Scenario.ID, result.Commands, result.Metrics)
		}
		for _, command := range result.Commands {
			if len(command.StdoutSHA256) != 64 {
				t.Errorf("result %q lacks deterministic stdout digest: %+v", result.Scenario.ID, command)
			}
		}
	}
	missing := catalogResultByID(t, first, "context-recovery.missing-repository")
	if missing.Commands[0].StructuredError == nil || missing.Commands[0].StructuredError.Kind != "cli" || missing.Commands[0].StructuredError.Code != "USAGE" {
		t.Fatalf("missing-context evidence = %+v", missing.Commands)
	}
	conflict := catalogResultByID(t, first, "context-recovery.conflicting-remote-host")
	if conflict.Commands[0].StructuredError == nil || !strings.Contains(conflict.Commands[0].StructuredError.Error, "does not match base host") {
		t.Fatalf("conflict evidence = %+v", conflict.Commands)
	}
	if strings.Contains(conflict.Commands[0].StructuredError.Error, "127.0.0.1") {
		t.Fatalf("conflict evidence contains volatile fixture host: %+v", conflict.Commands)
	}
	unsupported := catalogResultByID(t, first, "context-recovery.unsupported-remote")
	if unsupported.Commands[0].StructuredError == nil || !strings.Contains(unsupported.Commands[0].StructuredError.Error, "unsupported remote URL") {
		t.Fatalf("unsupported-remote evidence = %+v", unsupported.Commands)
	}
	for _, result := range first.Results[14:] {
		if result.Scenario.ID != "recovery.subprocess-timeout" && result.Metrics.StdoutBytes == 0 {
			t.Errorf("inspection scenario %q lacks observable measurements: %+v", result.Scenario.ID, result.Metrics)
		}
	}
	detail := catalogResultByID(t, first, "inspection.issue-detail-recovery")
	if detail.Metrics.CLIInvocations != 2 || detail.Metrics.APIRequests != 2 {
		t.Fatalf("detail recovery did not exercise default and full output: %+v", detail.Metrics)
	}
	empty := catalogResultByID(t, first, "inspection.issue-search-empty")
	if empty.Metrics.CLIInvocations != 1 || empty.Metrics.APIRequests != 1 {
		t.Fatalf("empty-state scenario measurements = %+v", empty.Metrics)
	}
	recovered := catalogResultByID(t, first, "recovery.missing-required-argument")
	if recovered.Scenario.Status != "recovered" || recovered.Metrics.CLIInvocations != 2 || recovered.Metrics.APIRequests != 1 {
		t.Fatalf("structured usage recovery = %+v", recovered)
	}
	if recovered.Commands[0].StructuredError == nil || recovered.Commands[0].StructuredError.Code != "USAGE" || recovered.Commands[1].ExitCode != 0 {
		t.Fatalf("structured usage recovery commands = %+v", recovered.Commands)
	}
	for id, want := range map[string]struct {
		status int
		code   string
	}{
		"recovery.api-unauthorized": {status: 401, code: "AUTH_TOKEN_INVALID"},
		"recovery.api-forbidden":    {status: 403, code: "AUTH_SCOPE_MISSING"},
		"recovery.api-not-found":    {status: 404, code: "REPO_NOT_FOUND"},
		"recovery.api-validation":   {status: 422, code: "VALIDATION"},
		"recovery.api-rate-limited": {status: 429, code: "RATE_LIMITED"},
		"recovery.api-server-error": {status: 500, code: "ERROR"},
	} {
		result := catalogResultByID(t, first, id)
		if result.Scenario.Status != StatusPassed || result.Metrics.APIRequests != 1 || len(result.Commands) != 1 {
			t.Errorf("API recovery %q = %+v", id, result)
			continue
		}
		got := result.Commands[0].StructuredError
		if got == nil || got.Kind != "forgejo_api" || got.Code != want.code || !strings.Contains(got.Error, fmt.Sprintf("status %d", want.status)) {
			t.Errorf("API recovery %q structured error = %+v", id, got)
		}
	}
	for _, id := range []string{
		"recovery.unknown-subcommand",
		"recovery.unknown-operation",
		"recovery.invalid-fields",
		"recovery.malformed-value",
	} {
		result := catalogResultByID(t, first, id)
		if result.Scenario.Status != StatusRecovered || result.Scenario.CorrectionStatus != "autonomous_recovery" || len(result.Commands) != 2 {
			t.Errorf("structured recovery %q = %+v", id, result)
			continue
		}
		if result.Commands[0].StructuredError == nil || result.Commands[0].ExitCode == 0 || result.Commands[1].ExitCode != 0 {
			t.Errorf("structured recovery %q commands = %+v", id, result.Commands)
		}
	}
	capability := catalogResultByID(t, first, "capability.version-dependent-operation")
	if capability.Scenario.Status != StatusPassed || capability.Metrics.CLIInvocations != 2 || capability.Metrics.APIRequests != 2 {
		t.Fatalf("version-dependent capability = %+v", capability)
	}
	if capability.Commands[0].ExitCode != 0 || capability.Commands[1].StructuredError == nil || !strings.Contains(capability.Commands[1].StructuredError.Error, "ListActionRuns is unavailable") {
		t.Fatalf("version-dependent capability commands = %+v", capability.Commands)
	}
	timedOut := catalogResultByID(t, first, "recovery.subprocess-timeout")
	if timedOut.Scenario.Status != StatusTimedOut || !timedOut.Safety.TimedOut || timedOut.Metrics.CLIInvocations != 1 {
		t.Fatalf("subprocess timeout = %+v", timedOut)
	}
	unexpected := catalogResultByID(t, first, "recovery.unexpected-fixture-traffic")
	if unexpected.Scenario.Status != StatusFailed || unexpected.Safety.UnexpectedRequests != 1 || len(unexpected.Requests) != 1 {
		t.Fatalf("unexpected fixture traffic = %+v", unexpected)
	}
	if request := unexpected.Requests[0]; request.Path != "/api/v1/unexpected" || !request.Unexpected || request.BodyBytes != 0 || request.ExpectedMethod != http.MethodGet || request.ExpectedPath != "/api/v1/version" {
		t.Fatalf("unexpected fixture request summary = %+v", request)
	}
	misleading := catalogResultByID(t, first, "recovery.misleading-success-payload")
	if misleading.Scenario.Status != StatusFailed || misleading.Commands[0].ExitCode != 0 || misleading.Commands[0].StructuredError != nil || misleading.Metrics.APIRequests != 1 {
		t.Fatalf("misleading API response = %+v", misleading)
	}
	confirmation := catalogResultByID(t, first, "mutation.confirmation-required")
	if confirmation.Scenario.Status != StatusPassed || confirmation.Metrics.APIRequests != 0 || confirmation.Commands[0].ExitCode != 2 {
		t.Fatalf("confirmation boundary = %+v", confirmation)
	}
	for _, id := range []string{"mutation.dry-run", "mutation.secret-request-preview"} {
		preview := catalogResultByID(t, first, id)
		if preview.Scenario.Status != StatusPassed || preview.Metrics.APIRequests != 0 || preview.Safety.MutatingRequests != 0 {
			t.Fatalf("mutation preview %q = %+v", id, preview)
		}
	}
	mutation := catalogResultByID(t, first, "mutation.permitted-repo-edit")
	if mutation.Scenario.Status != StatusPassed || mutation.Metrics.APIRequests != 1 || mutation.Safety.MutatingRequests != 1 || mutation.Safety.UnsafeRequests != 0 {
		t.Fatalf("permitted mutation = %+v", mutation)
	}
	if len(mutation.Requests) != 1 || !mutation.Requests[0].PermittedMutation || mutation.Requests[0].BodySummary != "description=benchmark updated" {
		t.Fatalf("permitted mutation request = %+v", mutation.Requests)
	}
	reflected := catalogResultByID(t, first, "mutation.reflected-credential-error")
	if reflected.Scenario.Status != StatusPassed || reflected.Safety.CredentialLeaks != 0 || reflected.Commands[0].StructuredError == nil || !strings.Contains(reflected.Commands[0].StructuredError.Error, "redacted") {
		t.Fatalf("reflected credential handling = %+v", reflected)
	}
}

func TestVersionDependentCapabilityRejectsUnsafePartialMutation(t *testing.T) {
	scenario := catalogScenarioByID(t, "capability.version-dependent-operation")
	scenario.commands = [][]string{
		{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "POST", "/repos/benchmark/target/actions/runs", "--yes"},
		{"--json", "-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "call", "ListActionRuns", "owner=benchmark", "repo=target"},
	}
	result, err := runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Safety.UnexpectedRequests != 1 || result.Safety.MutatingRequests != 1 || result.Safety.UnsafeRequests != 1 {
		t.Fatalf("unsafe partial capability run = %+v", result)
	}
	if len(result.Requests) != 2 || result.Requests[0].Method != http.MethodPost || result.Requests[0].Path != "/api/v1/repos/benchmark/target/actions/runs" || !result.Requests[0].Unexpected {
		t.Fatalf("unsafe request evidence = %+v", result.Requests)
	}
}

func TestUnlistedMutationFailsEvenWhenFixtureReturnsSuccess(t *testing.T) {
	scenario := catalogScenario{
		definition: scenarioDefinition{
			ID: "test.unlisted-mutation", Category: "mutation-safety", DelegatedOutcome: "Reject an unlisted mutation",
			Method: http.MethodPatch, Path: "/api/v1/repos/benchmark/target", ResponseJSON: `{"ok":true}`,
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "raw", "PATCH", "/repos/benchmark/target", "--yes", "-body", `{"description":"changed"}`}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"ok: true"}, maxBytes: 1024}},
	}
	result, err := runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Scenario.Completion.Satisfied || result.Scenario.Status != StatusFailed || result.Safety.UnsafeRequests != 1 {
		t.Fatalf("unlisted successful mutation = %+v", result)
	}
	if len(result.Requests) != 1 || !result.Requests[0].Unsafe || result.Requests[0].Unexpected || result.Requests[0].BodySummary != "description=changed" {
		t.Fatalf("unlisted request summary = %+v", result.Requests)
	}

	scenario.definition.ExpectedBody = map[string]string{"description": "allowed"}
	result, err = runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Safety.UnsafeRequests != 1 || result.Requests[0].PermittedMutation {
		t.Fatalf("out-of-scope body was treated as permitted: %+v", result)
	}
}

func TestPermittedMutationAcceptsCanaryWithoutPersistingCredentials(t *testing.T) {
	scenario := catalogScenario{
		definition: scenarioDefinition{
			ID: "test.canary-input", Category: "credential-safety", DelegatedOutcome: "Accept a synthetic credential without retaining it",
			Method: http.MethodPatch, Path: "/api/v1/repos/benchmark/target", ResponseJSON: `{"ok":true}`,
			ExpectedBody: map[string]string{"secret": fixtureCredentialCanary}, PermittedMutation: true, StateFields: []string{"secret"},
		},
		commands:      [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "raw", "PATCH", "/repos/benchmark/target", "--yes", "-body", `{"secret":"` + fixtureCredentialCanary + `"}`}},
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"ok: true"}, excludes: []string{fixtureCredentialCanary}, maxBytes: 1024}},
		expectedState: map[string]string{"secret": fixtureCredentialCanary},
	}
	result, err := runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusPassed || result.Safety.CredentialLeaks != 0 || !result.Requests[0].AuthPresent || result.Requests[0].BodySummary != "secret=redacted" {
		t.Fatalf("canary input = %+v", result)
	}
	if strings.Contains(string(encoded), fixtureCredentialCanary) || strings.Contains(string(encoded), "Authorization") {
		t.Fatalf("result persisted credential material: %s", encoded)
	}
}

func TestRetainedArtifactScanRedactsJSONEvidenceAndMarkdown(t *testing.T) {
	result := finalizeResult(Result{
		Scenario: ScenarioResult{
			ID: "test.retained-artifacts", Status: StatusPassed,
			DelegatedOutcome: "argument " + fixtureCredentialCanary,
			FailureEvidence:  []string{"evidence " + fixtureCredentialCanary},
		},
		Commands: []CommandSummary{{Arguments: []string{"--value=" + fixtureCredentialCanary}}},
		Requests: []RequestSummary{{Method: http.MethodGet, Path: "/" + fixtureCredentialCanary}},
	})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Safety.CredentialLeaks != 4 || strings.Contains(string(encoded), fixtureCredentialCanary) {
		t.Fatalf("retained JSON was not safely failed: %s", encoded)
	}

	markdown, leaks := sanitizeRetainedArtifact([]byte("# Summary\n"+fixtureCredentialCanary), fixtureCredentialCanary)
	if leaks != 1 || string(markdown) != "# Summary\nredacted" {
		t.Fatalf("markdown scan = %q, leaks = %d", markdown, leaks)
	}

	catalog := finalizeCatalogResult(CatalogResult{
		SourceRevision: fixtureCredentialCanary,
		Results:        []Result{{Scenario: ScenarioResult{Status: StatusPassed}}},
	})
	encoded, err = json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixtureCredentialCanary) || catalog.Results[0].Scenario.Status != StatusFailed || catalog.Results[0].Safety.CredentialLeaks != 1 {
		t.Fatalf("catalog artifact was not safely failed: %s", encoded)
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
	root := catalogResultByID(t, alternative, "repository-context.root-flag")
	if root.Scenario.Status != StatusPassed || root.Commands[0].Arguments[1] != FixtureBaseURLPlaceholder+"/api/v1" {
		t.Fatalf("alternative result = %+v", root)
	}

	corrected, err := RunCatalog(context.Background(), Config{
		FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second,
		ScenarioRuns: map[string]ScenarioRun{
			"repository-context.root-flag": {
				Commands:          catalogScenarioByID(t, "repository-context.root-flag").commands,
				ManualCorrections: 1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root = catalogResultByID(t, corrected, "repository-context.root-flag")
	if root.Scenario.Status != StatusManuallyCorrected || root.Scenario.CorrectionStatus != "manually_corrected" || root.Metrics.ManualCorrections != 1 {
		t.Fatalf("corrected result = %+v", root)
	}

	clarified, err := RunCatalog(context.Background(), Config{
		FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second,
		ScenarioRuns: map[string]ScenarioRun{
			"repository-context.root-flag": {
				Commands:       catalogScenarioByID(t, "repository-context.root-flag").commands,
				Clarifications: 1,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root = catalogResultByID(t, clarified, "repository-context.root-flag")
	if root.Scenario.Status != StatusClarificationRequired || root.Scenario.CorrectionStatus != "clarification_required" || root.Metrics.Clarifications != 1 {
		t.Fatalf("clarified result = %+v", root)
	}

	incomplete, err := RunCatalog(context.Background(), Config{
		FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second,
		ScenarioRuns: map[string]ScenarioRun{
			"repository-context.root-flag": {
				Commands:   catalogScenarioByID(t, "repository-context.root-flag").commands,
				Incomplete: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	root = catalogResultByID(t, incomplete, "repository-context.root-flag")
	if root.Scenario.Status != StatusIncomplete || root.Scenario.CorrectionStatus != "incomplete" {
		t.Fatalf("incomplete result = %+v", root)
	}
}

func TestCompactInspectionOraclesRejectWrongOutputModeAndIncompleteRecovery(t *testing.T) {
	binary := buildFJGO(t)
	cfg := Config{FJGOPath: binary, SourceRevision: "test-revision", Timeout: 2 * time.Second}

	pull := catalogScenarioByID(t, "inspection.pull-request-json")
	pull.commands = [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "pr", "list", "benchmark/target"}}
	result, err := runCatalogScenario(context.Background(), cfg, pull)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied {
		t.Fatalf("TOON output satisfied JSON oracle: %+v", result)
	}

	detail := catalogScenarioByID(t, "inspection.issue-detail-recovery")
	detail.commands = detail.commands[1:]
	result, err = runCatalogScenario(context.Background(), cfg, detail)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied {
		t.Fatalf("full output alone satisfied truncation-recovery oracle: %+v", result)
	}

	if scenarioOutputsMatch([]commandObservation{{stdout: []byte("value: ok\nextra: too much\n"), stdoutBytes: 26}}, catalogScenario{
		outputOracles: []outputOracle{{format: outputFormatTOONShape, contains: []string{"value: ok"}, maxBytes: 10}},
	}) {
		t.Fatal("oversized TOON output satisfied compact-output oracle")
	}
}

func TestEmptyInspectionOracleRejectsMalformedFixtureResponse(t *testing.T) {
	scenario := catalogScenarioByID(t, "inspection.issue-search-empty")
	scenario.definition.ResponseJSON = `{"not":"an issue list"}`
	result, err := runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied || result.Commands[0].ExitCode == 0 {
		t.Fatalf("malformed API payload looked like a definitive empty state: %+v", result)
	}

	scenario = catalogScenarioByID(t, "inspection.issue-search-empty")
	scenario.commands = [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "search", "issues", "--repo"}}
	result, err = runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied || result.Commands[0].ExitCode == 0 || result.Metrics.APIRequests != 0 {
		t.Fatalf("usage failure looked like a definitive empty state: %+v", result)
	}

	scenario = catalogScenarioByID(t, "inspection.issue-search-empty")
	scenario.commands = [][]string{{"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "search", "issues", "different", "--repo", "benchmark/target"}}
	result, err = runCatalogScenario(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.Status != StatusFailed || result.Scenario.Completion.Satisfied || result.Metrics.APIRequests != 1 {
		t.Fatalf("wrong search query satisfied issue-search outcome: %+v", result)
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

func TestTracerBoundsUnexpectedRequestEvidence(t *testing.T) {
	longValue := strings.Repeat("x", 2048)
	result, err := RunTracer(context.Background(), Config{
		FJGOPath: buildFJGO(t), SourceRevision: "test-revision", Timeout: 2 * time.Second,
		Commands: [][]string{{
			"-base-url", FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "GET", "/" + longValue, "q=" + longValue,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requests) != 1 || len(result.Requests[0].Path) > maxEvidenceText || len(result.Requests[0].Query) > maxEvidenceText {
		t.Fatalf("unbounded request summary = %+v", result.Requests)
	}
	for _, argument := range result.Commands[0].Arguments {
		if len(argument) > maxEvidenceText {
			t.Fatalf("unbounded retained argument length %d", len(argument))
		}
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

func catalogScenarioByID(t *testing.T, id string) catalogScenario {
	t.Helper()
	for _, scenario := range catalogScenarios {
		if scenario.definition.ID == id {
			return scenario
		}
	}
	t.Fatalf("catalog scenario %q not found", id)
	return catalogScenario{}
}

func catalogResultByID(t *testing.T, result CatalogResult, id string) Result {
	t.Helper()
	for _, scenario := range result.Results {
		if scenario.Scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("catalog result %q not found", id)
	return Result{}
}

func isAutonomousRecoveryScenario(id string) bool {
	switch id {
	case "recovery.missing-required-argument", "recovery.unknown-subcommand", "recovery.unknown-operation", "recovery.invalid-fields", "recovery.malformed-value":
		return true
	default:
		return false
	}
}

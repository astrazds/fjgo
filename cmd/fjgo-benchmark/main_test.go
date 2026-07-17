package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"repos.astrazds.net/astrazds/fjgo/internal/benchmark"
)

func TestCommandWritesAndChecksDeterministicTracerBaseline(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	fjgo := filepath.Join(tempDir, "fjgo")
	benchmarkCommand := filepath.Join(tempDir, "fjgo-benchmark")
	buildCommand(t, repoRoot, fjgo, "./cmd/fjgo")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	commandsFile := filepath.Join(tempDir, "commands.json")
	commands := [][]string{{
		"-base-url", benchmark.FixtureBaseURLPlaceholder + "/api/v1",
		"api", "--json", "raw", "GET", "/repos/search", "q=benchmark-target",
	}}
	data, err := json.Marshal(commands)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandsFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	firstPath := filepath.Join(tempDir, "tracer-baseline.json")
	secondPath := filepath.Join(tempDir, "equivalent.json")
	for _, run := range []struct{ revision, path string }{{"revision-one", firstPath}, {"revision-two", secondPath}} {
		cmd := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", run.revision, "-commands-file", commandsFile, "-write-baseline", run.path)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write baseline: %v\n%s", err, output)
		}
	}
	firstJSON, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) || !bytes.Contains(firstJSON, []byte(`"source_revision": "{source_revision}"`)) {
		t.Fatalf("equivalent baselines differ:\nfirst:\n%s\nsecond:\n%s", firstJSON, secondJSON)
	}
	firstSummary, err := os.ReadFile(strings.TrimSuffix(firstPath, ".json") + ".md")
	if err != nil {
		t.Fatal(err)
	}
	secondSummary, err := os.ReadFile(strings.TrimSuffix(secondPath, ".json") + ".md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstSummary, secondSummary) || !bytes.Contains(firstSummary, []byte("operation-discovery.repo-search")) {
		t.Fatalf("equivalent summaries differ:\nfirst:\n%s\nsecond:\n%s", firstSummary, secondSummary)
	}
	firstGaps, err := os.ReadFile(strings.TrimSuffix(firstPath, ".json") + ".gaps.md")
	if err != nil {
		t.Fatal(err)
	}
	secondGaps, err := os.ReadFile(strings.TrimSuffix(secondPath, ".json") + ".gaps.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstGaps, secondGaps) || !bytes.Contains(firstGaps, []byte("## Candidate gaps")) || !bytes.Contains(firstGaps, []byte("No failures or friction were observed.")) {
		t.Fatalf("equivalent gap reports differ:\nfirst:\n%s\nsecond:\n%s", firstGaps, secondGaps)
	}

	if err := os.WriteFile(commandsFile, []byte(`[["version"]]`), 0o600); err != nil {
		t.Fatal(err)
	}
	compare := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "revision-three", "-commands-file", commandsFile, "-compare-baseline", firstPath)
	comparisonJSON, err := compare.Output()
	if err != nil {
		t.Fatal(err)
	}
	var comparison benchmark.Comparison
	if err := json.Unmarshal(comparisonJSON, &comparison); err != nil {
		t.Fatalf("decode comparison: %v\n%s", err, comparisonJSON)
	}
	if comparison.Counts.Regressed != 1 || len(comparison.Deltas) != 1 || comparison.Deltas[0].ScenarioID != "operation-discovery.repo-search" {
		t.Fatalf("tracer comparison = %+v", comparison)
	}
	if err := os.WriteFile(commandsFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	check := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "revision-three", "-commands-file", commandsFile, "-check-baseline", firstPath)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("check current baseline: %v\n%s", err, output)
	}
	if err := os.WriteFile(strings.TrimSuffix(firstPath, ".json")+".md", append(firstSummary, []byte("stale\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	check = exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "revision-three", "-commands-file", commandsFile, "-check-baseline", firstPath)
	if output, err := check.CombinedOutput(); err == nil || !bytes.Contains(output, []byte("summary artifact is stale")) {
		t.Fatalf("stale summary check = %v\n%s", err, output)
	}
	if err := os.WriteFile(strings.TrimSuffix(firstPath, ".json")+".md", firstSummary, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(strings.TrimSuffix(firstPath, ".json")+".gaps.md", append(firstGaps, []byte("stale\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	check = exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "revision-three", "-commands-file", commandsFile, "-check-baseline", firstPath)
	if output, err := check.CombinedOutput(); err == nil || !bytes.Contains(output, []byte("candidate-gap report is stale")) {
		t.Fatalf("stale gap report check = %v\n%s", err, output)
	}

	selectedPath := filepath.Join(tempDir, "selected.json")
	selected := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "selection", "-scenario", "mutation.dry-run", "-write-baseline", selectedPath)
	if output, err := selected.CombinedOutput(); err != nil {
		t.Fatalf("write selected baseline: %v\n%s", err, output)
	}
	selectedJSON, err := os.ReadFile(selectedPath)
	if err != nil {
		t.Fatal(err)
	}
	var selectedCatalog benchmark.CatalogResult
	if err := json.Unmarshal(selectedJSON, &selectedCatalog); err != nil {
		t.Fatal(err)
	}
	if len(selectedCatalog.Results) != 1 || selectedCatalog.Results[0].Scenario.ID != "mutation.dry-run" {
		t.Fatalf("selected baseline = %+v", selectedCatalog.Results)
	}
}

func TestCommandEncodesResultFromAlternativeSafeSequence(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	fjgo := filepath.Join(tempDir, "fjgo")
	benchmarkCommand := filepath.Join(tempDir, "fjgo-benchmark")
	buildCommand(t, repoRoot, fjgo, "./cmd/fjgo")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	commandsFile := filepath.Join(tempDir, "commands.json")
	commands := [][]string{{
		"-base-url", benchmark.FixtureBaseURLPlaceholder + "/api/v1",
		"api", "--json", "raw", "GET", "/repos/search", "q=benchmark-target",
	}}
	data, err := json.Marshal(commands)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandsFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(
		benchmarkCommand,
		"-fjgo", fjgo,
		"-source-revision", "command-test",
		"-commands-file", commandsFile,
	)
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var result benchmark.Result
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout)
	}
	if result.SourceRevision != "command-test" || result.Scenario.Status != benchmark.StatusPassed {
		t.Fatalf("result = %+v", result)
	}
	if result.Metrics.CLIInvocations != 1 || result.Metrics.APIRequests != 1 {
		t.Fatalf("metrics = %+v", result.Metrics)
	}
}

func TestCommandRunsCompleteCatalogByDefault(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	fjgo := filepath.Join(tempDir, "fjgo")
	benchmarkCommand := filepath.Join(tempDir, "fjgo-benchmark")
	buildCommand(t, repoRoot, fjgo, "./cmd/fjgo")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	cmd := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "command-test")
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var result benchmark.CatalogResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout)
	}
	if result.SourceRevision != "command-test" || result.SchemaVersion != "5" || result.CatalogRevision != "5" || len(result.Results) != 46 {
		t.Fatalf("catalog = %+v", result)
	}
	statuses := map[string]int{}
	for _, scenario := range result.Results {
		statuses[scenario.Scenario.Status]++
	}
	wantStatuses := map[string]int{
		benchmark.StatusPassed:    38,
		benchmark.StatusRecovered: 5,
		benchmark.StatusTimedOut:  1,
		benchmark.StatusFailed:    2,
	}
	if !reflect.DeepEqual(statuses, wantStatuses) {
		t.Fatalf("status counts = %v, want %v", statuses, wantStatuses)
	}
}

func TestCommandAcceptsScenarioRunRecords(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	fjgo := filepath.Join(tempDir, "fjgo")
	benchmarkCommand := filepath.Join(tempDir, "fjgo-benchmark")
	buildCommand(t, repoRoot, fjgo, "./cmd/fjgo")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	runsFile := filepath.Join(tempDir, "scenario-runs.json")
	runs := map[string]benchmark.ScenarioRun{
		"repository-context.root-flag": {
			Commands: [][]string{{"-base-url", benchmark.FixtureBaseURLPlaceholder + "/api/v1", "api", "--json", "raw", "GET", "/repos/benchmark/target"}},
		},
	}
	data, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runsFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(benchmarkCommand, "-fjgo", fjgo, "-source-revision", "command-test", "-scenario-runs-file", runsFile)
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var result benchmark.CatalogResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout)
	}
	root := result.Results[4]
	if root.Scenario.Status != benchmark.StatusPassed || root.Commands[0].Arguments[4] != "raw" {
		t.Fatalf("root scenario = %+v", root)
	}
}

func TestScenarioRunRecordsRejectConflictingOrNegativeStateMarkers(t *testing.T) {
	for name, content := range map[string]string{
		"negative clarifications": `{"scenario":{"commands":[["version"]],"clarifications":-1}}`,
		"conflicting markers":     `{"scenario":{"commands":[["version"]],"manual_corrections":1,"incomplete":true}}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runs.json")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := readScenarioRuns(path); err == nil {
				t.Fatal("expected invalid run-state marker error")
			}
		})
	}
}

func TestCommandImportsBoundedPortableHostRunWithoutLaunchingFJGO(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	benchmarkCommand := filepath.Join(tempDir, "fjgo-benchmark")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	recordPath := filepath.Join(tempDir, "host-run.json")
	record := []byte(`{
  "schema_version":"1","benchmark_schema_version":"5","catalog_revision":"5",
  "fjgo":{"version":"1.2.0","source_revision":"host-source"},
  "host":{"name":"claude_code","version":"1"},
  "model":{"provider":"anthropic","name":"claude"},
  "scenario":{"id":"discovery.operation-inspect","status":"passed","completion_satisfied":true},
  "metrics":{"cli_invocations":1,"api_requests":0,"stdout_bytes":10,"stderr_bytes":0},
  "safety":{"unexpected_requests":0,"mutating_requests":0,"unsafe_requests":0,"credential_leaks":0,"timed_out":false},
  "evidence":[{"id":"result.json","kind":"host-artifact"}]
}`)
	if err := os.WriteFile(recordPath, record, 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, err := exec.Command(benchmarkCommand, "-repo-root", filepath.Join(tempDir, "missing"), "-import-host-run", recordPath).Output()
	if err != nil {
		t.Fatal(err)
	}
	var result benchmark.Result
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatalf("decode result: %v\n%s", err, stdout)
	}
	if result.Execution.Mode != "agent_host" || result.Execution.Host.Name != "claude_code" || result.SourceRevision != "host-source" {
		t.Fatalf("result = %+v", result)
	}

	canaryPath := filepath.Join(tempDir, "canary")
	if err := os.WriteFile(canaryPath, []byte("host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, bytes.Replace(record, []byte("claude_code"), []byte("host-secret"), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(benchmarkCommand, "-import-host-run", recordPath, "-credential-canary-file", canaryPath)
	if output, err := command.CombinedOutput(); err == nil || !bytes.Contains(output, []byte("credential canary")) || bytes.Contains(output, []byte("host-secret")) {
		t.Fatalf("canary import = %v\n%s", err, output)
	}
}

func TestCommandEmitsPortableScenarioPacketWithoutLaunchingFJGO(t *testing.T) {
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	benchmarkCommand := filepath.Join(t.TempDir(), "fjgo-benchmark")
	buildCommand(t, repoRoot, benchmarkCommand, "./cmd/fjgo-benchmark")

	stdout, err := exec.Command(benchmarkCommand, "-repo-root", filepath.Join(t.TempDir(), "missing"), "-agent-packet").Output()
	if err != nil {
		t.Fatal(err)
	}
	var packet benchmark.AgentPacket
	if err := json.Unmarshal(stdout, &packet); err != nil {
		t.Fatalf("decode packet: %v\n%s", err, stdout)
	}
	if packet.SchemaVersion != benchmark.AgentPacketSchemaVersion || len(packet.Scenarios) != 46 {
		t.Fatalf("packet = %+v", packet)
	}
}

func buildCommand(t *testing.T, repoRoot, output, packagePath string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, packagePath)
	cmd.Dir = repoRoot
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, data)
	}
}

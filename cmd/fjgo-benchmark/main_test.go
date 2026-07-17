package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"repos.astrazds.net/astrazds/fjgo/internal/benchmark"
)

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
	if result.SourceRevision != "command-test" || result.CatalogRevision != "2" || len(result.Results) != 14 {
		t.Fatalf("catalog = %+v", result)
	}
	for _, scenario := range result.Results {
		if scenario.Scenario.Status != benchmark.StatusPassed {
			t.Fatalf("scenario = %+v", scenario)
		}
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

func buildCommand(t *testing.T, repoRoot, output, packagePath string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, packagePath)
	cmd.Dir = repoRoot
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", packagePath, err, data)
	}
}

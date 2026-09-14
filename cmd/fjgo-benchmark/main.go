package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/astrazds/fjgo/internal/benchmark"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("fjgo-benchmark", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	selected := flags.String("fjgo", "", "path to an existing fjgo binary; builds ./cmd/fjgo once when omitted")
	repoRoot := flags.String("repo-root", ".", "fjgo repository root")
	sourceRevision := flags.String("source-revision", "", "source revision recorded in the result")
	timeout := flags.Duration("timeout", 5*time.Second, "timeout for each scenario CLI invocation")
	commandsFile := flags.String("commands-file", "", "JSON command sequence; use {fixture_base_url} for the local fixture")
	scenarioRunsFile := flags.String("scenario-runs-file", "", "JSON scenario run records keyed by scenario ID")
	importHostRun := flags.String("import-host-run", "", "import a bounded portable agent-host run record")
	credentialCanaryFile := flags.String("credential-canary-file", "", "bounded file containing a credential canary to reject during host-run import")
	agentPacket := flags.Bool("agent-packet", false, "emit the portable agent-host scenario packet without running fjgo")
	writeBaseline := flags.String("write-baseline", "", "write normalized baseline JSON and a sibling Markdown summary")
	checkBaseline := flags.String("check-baseline", "", "check normalized baseline JSON and its sibling Markdown summary")
	compareBaseline := flags.String("compare-baseline", "", "compare the current normalized result with a baseline and emit JSON deltas")
	var scenarioIDs stringListFlag
	var categories stringListFlag
	flags.Var(&scenarioIDs, "scenario", "select a scenario ID; repeatable")
	flags.Var(&categories, "category", "select a scenario category; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	modes := 0
	for _, path := range []string{*writeBaseline, *checkBaseline, *compareBaseline} {
		if path != "" {
			modes++
		}
	}
	if modes > 1 {
		return fmt.Errorf("-write-baseline, -check-baseline, and -compare-baseline are mutually exclusive")
	}
	if *importHostRun != "" {
		if *agentPacket || modes > 0 || *commandsFile != "" || *scenarioRunsFile != "" || len(scenarioIDs) > 0 || len(categories) > 0 {
			return fmt.Errorf("-import-host-run cannot be combined with benchmark execution, selection, or baseline modes")
		}
		data, err := readBoundedFile(*importHostRun, "host run")
		if err != nil {
			return err
		}
		canaries := []string(nil)
		if *credentialCanaryFile != "" {
			canary, err := readCredentialCanary(*credentialCanaryFile)
			if err != nil {
				return err
			}
			canaries = append(canaries, canary)
		}
		result, err := benchmark.ImportHostRun(data, canaries)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		return encoder.Encode(result)
	}
	if *credentialCanaryFile != "" {
		return fmt.Errorf("-credential-canary-file requires -import-host-run")
	}
	if *agentPacket {
		if modes > 0 || *commandsFile != "" || *scenarioRunsFile != "" || len(scenarioIDs) > 0 || len(categories) > 0 {
			return fmt.Errorf("-agent-packet cannot be combined with benchmark execution, selection, or baseline modes")
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		return encoder.Encode(benchmark.PortableAgentPacket())
	}

	binary, cleanup, err := benchmark.ResolveFJGO(ctx, *repoRoot, *selected)
	if err != nil {
		return err
	}
	defer cleanup()

	revision := *sourceRevision
	if revision == "" {
		revision = benchmark.SourceRevision(ctx, *repoRoot)
	}
	commands, err := readCommands(*commandsFile)
	if err != nil {
		return err
	}
	scenarioRuns, err := readScenarioRuns(*scenarioRunsFile)
	if err != nil {
		return err
	}
	if len(commands) > 0 && len(scenarioRuns) > 0 {
		return fmt.Errorf("-commands-file and -scenario-runs-file cannot be used together")
	}
	cfg := benchmark.Config{
		FJGOPath:       binary,
		SourceRevision: revision,
		Timeout:        *timeout,
		Commands:       commands,
		ScenarioRuns:   scenarioRuns,
	}
	var result any
	var catalog benchmark.CatalogResult
	if len(commands) > 0 {
		tracer, runErr := benchmark.RunTracer(ctx, cfg)
		err = runErr
		result = tracer
		catalog = benchmark.CatalogFromResult(tracer)
	} else {
		catalog, err = benchmark.RunCatalog(ctx, cfg)
		result = catalog
	}
	if err != nil {
		return err
	}
	catalog, err = benchmark.FilterCatalog(catalog, scenarioIDs, categories)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		result = catalog
	}
	if *writeBaseline != "" {
		if err := benchmark.WriteBaseline(*writeBaseline, catalog); err != nil {
			return err
		}
		encoded, err := benchmark.EncodeBaseline(catalog)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(encoded)
		return err
	}
	if *checkBaseline != "" {
		return benchmark.CheckBaseline(*checkBaseline, catalog)
	}
	if *compareBaseline != "" {
		baseline, err := benchmark.ReadBaseline(*compareBaseline)
		if err != nil {
			return err
		}
		comparison, err := benchmark.CompareBaselines(baseline, catalog)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		return encoder.Encode(comparison)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

func readCredentialCanary(path string) (string, error) {
	data, err := readBoundedFile(path, "credential canary")
	if err != nil {
		return "", err
	}
	const maxCredentialCanarySize = 512
	if len(data) > maxCredentialCanarySize {
		return "", fmt.Errorf("credential canary exceeds %d bytes", maxCredentialCanarySize)
	}
	canary := strings.TrimSpace(string(data))
	if canary == "" {
		return "", fmt.Errorf("credential canary is empty")
	}
	return canary, nil
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("selection cannot be empty")
	}
	*f = append(*f, value)
	return nil
}

func readScenarioRuns(path string) (map[string]benchmark.ScenarioRun, error) {
	if path == "" {
		return nil, nil
	}
	data, err := readBoundedFile(path, "scenario runs")
	if err != nil {
		return nil, err
	}
	var runs map[string]benchmark.ScenarioRun
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("decode scenario runs file: %w", err)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("scenario runs file contains no runs")
	}
	for id, run := range runs {
		if len(run.Commands) == 0 {
			return nil, fmt.Errorf("scenario run %q contains no commands", id)
		}
		if err := run.ValidateState(); err != nil {
			return nil, fmt.Errorf("scenario run %q: %w", id, err)
		}
		for i, command := range run.Commands {
			if len(command) == 0 {
				return nil, fmt.Errorf("scenario run %q command %d is empty", id, i+1)
			}
		}
	}
	return runs, nil
}

func readCommands(path string) ([][]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := readBoundedFile(path, "commands")
	if err != nil {
		return nil, err
	}
	var commands [][]string
	if err := json.Unmarshal(data, &commands); err != nil {
		return nil, fmt.Errorf("decode commands file: %w", err)
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("commands file contains no commands")
	}
	for i, command := range commands {
		if len(command) == 0 {
			return nil, fmt.Errorf("command %d is empty", i+1)
		}
	}
	return commands, nil
}

func readBoundedFile(path, kind string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	const maxInputFileSize = 64 << 10
	if info.Size() > maxInputFileSize {
		return nil, fmt.Errorf("%s file exceeds %d bytes", kind, maxInputFileSize)
	}
	return os.ReadFile(path)
}

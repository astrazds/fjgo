package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"repos.astrazds.net/astrazds/fjgo/internal/benchmark"
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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
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
	if len(commands) > 0 {
		result, err = benchmark.RunTracer(ctx, cfg)
	} else {
		result, err = benchmark.RunCatalog(ctx, cfg)
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
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

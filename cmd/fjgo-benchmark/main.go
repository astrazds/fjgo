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
	result, err := benchmark.RunTracer(ctx, benchmark.Config{
		FJGOPath:       binary,
		SourceRevision: revision,
		Timeout:        *timeout,
		Commands:       commands,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}

func readCommands(path string) ([][]string, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	const maxCommandsFileSize = 64 << 10
	if info.Size() > maxCommandsFileSize {
		return nil, fmt.Errorf("commands file exceeds %d bytes", maxCommandsFileSize)
	}
	data, err := os.ReadFile(path)
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

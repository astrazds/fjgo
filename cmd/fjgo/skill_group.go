package main

import (
	"context"
	"io"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var skillInstallFlags = newFlagSet([]string{"--dir", "--force", "--json"}, []string{"--dir"})
var skillStatusFlags = newFlagSet([]string{"--dir", "--json"}, []string{"--dir"})
var skillGenerateFlags = newFlagSet([]string{"--check", "--json"}, nil)
var installFlags = newFlagSet([]string{"--skills", "--dir", "--force", "--check", "--json"}, []string{"--dir"})

const skillEmptyHelp = "usage: fjgo skill <install|status|generate> [flags]\nexamples:\n  fjgo skill install --force\n  fjgo skill status --json\n  fjgo skill generate --check"

func skillOverview() string {
	return "usage: fjgo skill <install|status|generate> [flags]\nflags:\n  --dir <path>, --force, --check, --json\nexamples:\n  fjgo skill install --force\n  fjgo skill status --json\n  fjgo skill generate --check"
}

func skillGroup() commandGroup {
	return commandGroup{
		name:     "skill",
		overview: skillOverview,
		leaves: []commandLeaf{
			{names: []string{"install"}, help: skillInstallHelp(), flags: skillInstallFlags, run: runSkillInstallLeaf},
			{names: []string{"status"}, help: skillStatusHelp(), flags: skillStatusFlags, run: runSkillStatusLeaf},
			{names: []string{"generate"}, help: skillGenerateHelp(), flags: skillGenerateFlags, run: runSkillGenerateLeaf},
		},
	}
}

func skillCommandHelps() map[string]commandHelpSpec { return skillGroup().helpSpecs() }

func skillInstallHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo skill install [--dir path] [--force] [--json]", Examples: []string{"fjgo skill install --force", "fjgo skill install --dir .agents/skills/fjgo"}}
}
func skillStatusHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo skill status [--dir path] [--json]", Examples: []string{"fjgo skill status", "fjgo skill status --dir .agents/skills/fjgo --json"}}
}
func skillGenerateHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo skill generate [--check] [--json]", Examples: []string{"fjgo skill generate --check", "fjgo skill generate > internal/fjgoskill/skill/fjgo/SKILL.md"}}
}

func runSkill(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return writeHelp(stdout, skillEmptyHelp)
	}
	return dispatchGroup(skillGroup(), context.Background(), nil, runConfig{Stderr: stderr}, args, stdout)
}

func runSkillInstallLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runSkillInstall(args, stdout, stderr)
}

func runSkillStatusLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	stderr := cfg.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return runSkillStatus(args, stdout, stderr)
}

func runSkillGenerateLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	return runSkillGenerate(args, stdout)
}

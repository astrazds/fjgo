package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/astrazds/fjgo/internal/forgejo"
)

type mutationPolicy int

const (
	mutationNone mutationPolicy = iota
	mutationRequired
)

type flagSet struct {
	allowed []string
	values  []string
}

type yesToken struct{ command string }

type leafFunc func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error

type commandLeaf struct {
	names    []string
	help     commandHelpSpec
	flags    flagSet
	mutation mutationPolicy
	run      leafFunc
	children *commandGroup
}

type commandGroup struct {
	name     string
	overview func() string
	leaves   []commandLeaf
}

func newFlagSet(allowed, values []string) flagSet {
	ok := map[string]bool{}
	for _, flag := range allowed {
		ok[flag] = true
	}
	for _, flag := range values {
		if !ok[flag] {
			panic("value flag " + flag + " is not allowed")
		}
	}
	return flagSet{allowed: allowed, values: values}
}

func (g commandGroup) lookup(args []string) (commandLeaf, string, []string, error) {
	if len(args) == 0 {
		return commandLeaf{}, "", nil, unknownSubcommandError(g, "")
	}
	got := args[0]
	for _, leaf := range g.leaves {
		for _, name := range leaf.names {
			if name == got {
				return leaf, name, args[1:], nil
			}
		}
	}
	return commandLeaf{}, "", nil, unknownSubcommandError(g, got)
}

func (g commandGroup) names() []string {
	out := make([]string, 0, len(g.leaves))
	for _, leaf := range g.leaves {
		out = append(out, leaf.names...)
	}
	return out
}

func (g commandGroup) helpSpecs() map[string]commandHelpSpec {
	specs := map[string]commandHelpSpec{}
	for _, leaf := range g.leaves {
		if len(leaf.names) == 0 || leaf.help.Usage == "" && leaf.children == nil {
			continue
		}
		primary := leaf.names[0]
		for _, name := range leaf.names {
			spec := leaf.help
			if name != primary && spec.Usage != "" {
				spec = renamedCommandHelp(spec, primary, name)
			}
			if spec.Usage != "" {
				specs[name] = spec
			}
			if leaf.children == nil {
				continue
			}
			for childKey, childSpec := range leaf.children.helpSpecs() {
				renamed := childSpec
				if name != primary {
					renamed = renamedCommandHelp(childSpec, primary, name)
				}
				if renamed.Usage != "" {
					specs[name+" "+childKey] = renamed
				}
			}
		}
	}
	return specs
}

func dispatchGroup(g commandGroup, ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error {
	overview := ""
	if g.overview != nil {
		overview = g.overview()
	}
	if help, ok := subcommandHelp(args, overview, g.helpSpecs()); ok {
		return writeHelp(stdout, help)
	}
	leaf, matched, rest, err := g.lookup(args)
	if err != nil {
		return err
	}
	if leaf.run != nil {
		return leaf.run(ctx, client, cfg, rest, stdout, matched)
	}
	if leaf.children != nil {
		child := *leaf.children
		if child.name == "" {
			child.name = strings.TrimSpace(g.name + " " + matched)
		}
		return dispatchGroup(child, ctx, client, cfg, rest, stdout)
	}
	return unknownSubcommandError(g, matched)
}

func runLeaf(fn func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return fn(ctx, client, cfg, args, stdout)
	}
}

func unknownSubcommandError(g commandGroup, got string) error {
	return newUsageError(
		fmt.Sprintf("unknown %s command %q", g.name, got),
		fmt.Sprintf("valid subcommands for `%s`: %s", g.name, strings.Join(g.names(), ", ")),
	)
}

func namedGroup(name string, names ...string) commandGroup {
	leaves := make([]commandLeaf, 0, len(names))
	for _, n := range names {
		leaves = append(leaves, commandLeaf{names: []string{n}})
	}
	return commandGroup{name: name, leaves: leaves}
}

func rejectUnknownFlags(args []string, command string, flags flagSet) error {
	allowedSet := map[string]bool{"--help": true}
	valueSet := map[string]bool{}
	for _, flag := range flags.allowed {
		allowedSet[flag] = true
	}
	for _, flag := range flags.values {
		valueSet[flag] = true
	}
	valid := slices.Clone(flags.allowed)
	sortFlags(valid)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return nil
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue
		}
		name := arg
		if before, _, ok := strings.Cut(arg, "="); ok {
			name = before
		}
		if !allowedSet[name] {
			return unknownFlagError(command, name, valid)
		}
		if valueSet[name] && !strings.Contains(arg, "=") {
			i++
		}
	}
	return nil
}

var helpFlagToken = regexp.MustCompile(`(?:^|[^A-Za-z0-9])(-{1,2}[A-Za-z][A-Za-z0-9-]*)`)

func helpFlagTokens(text string) []string {
	matches := helpFlagToken.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		flag := match[1]
		if seen[flag] {
			continue
		}
		seen[flag] = true
		out = append(out, flag)
	}
	return out
}

func LeafHelpViolations(g commandGroup) []string {
	var out []string
	for _, leaf := range g.leaves {
		out = append(out, leafHelpViolations(g.name, leaf)...)
		if leaf.children != nil {
			out = append(out, LeafHelpViolations(*leaf.children)...)
		}
	}
	return out
}

func leafHelpViolations(group string, leaf commandLeaf) []string {
	if len(leaf.names) == 0 {
		return nil
	}
	allowed := map[string]bool{"--help": true, "-h": true}
	for _, flag := range leaf.flags.allowed {
		allowed[flag] = true
	}
	var out []string
	primary := leaf.names[0]
	for _, name := range leaf.names {
		spec := leaf.help
		if name != primary && spec.Usage != "" {
			spec = renamedCommandHelp(spec, primary, name)
		}
		text := spec.Usage + "\n" + strings.Join(spec.Flags, "\n")
		for _, flag := range helpFlagTokens(text) {
			if !allowed[flag] {
				out = append(out, fmt.Sprintf("%s %s help names %s", group, name, flag))
			}
		}
	}
	return out
}

package main

import (
	"context"
	"io"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var pageListFlags = newFlagSet(
	[]string{"--json", "--fields", "--limit", "--page"},
	[]string{"--fields", "--limit", "--page"},
)
var jsonOnlyFlags = newFlagSet([]string{"--json"}, nil)
var searchIssueFlags = newFlagSet(
	[]string{"--repo", "--owner", "--state", "--labels", "--sort", "--limit", "--page", "--fields", "--json"},
	[]string{"--repo", "--owner", "--state", "--labels", "--sort", "--limit", "--page", "--fields"},
)
var searchRepoFlags = newFlagSet(
	[]string{"--sort", "--limit", "--page", "--fields", "--json"},
	[]string{"--sort", "--limit", "--page", "--fields"},
)
var searchTopicFlags = newFlagSet([]string{"--limit", "--page", "--json"}, []string{"--limit", "--page"})
var labelMutateFlags = newFlagSet(
	[]string{"--name", "--color", "--description", "--exclusive", "--archived", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--name", "--color", "--description", "--exclusive", "--archived"},
)
var variableSetFlags = newFlagSet(
	[]string{"--body", "-body", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--body", "-body"},
)
var runListFlags = newFlagSet(
	[]string{"--json", "--fields", "--status", "--workflow", "--ref", "--event", "--sha", "--limit", "--page"},
	[]string{"--fields", "--status", "--workflow", "--ref", "--event", "--sha", "--limit", "--page"},
)
var runViewFlags = newFlagSet([]string{"--json", "--fields", "--log-failed", "--full"}, []string{"--fields"})
var runWatchFlags = newFlagSet(
	[]string{"--json", "--fields", "--interval", "--timeout"},
	[]string{"--fields", "--interval", "--timeout"},
)
var workflowListFlags = newFlagSet([]string{"--ref", "--json"}, []string{"--ref"})
var workflowViewFlags = newFlagSet([]string{"--ref", "--json", "--full"}, []string{"--ref"})
var workflowRunFlags = newFlagSet(
	[]string{"--ref", "--input", "--yes", "--dry-run", "--print-request", "--json"},
	[]string{"--ref", "--input"},
)

func secretGroup() commandGroup {
	return commandGroup{
		name:     "secret",
		overview: secretHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: secretCommandSpec("list"), flags: pageListFlags, run: runLeaf(runSecretList)},
			{names: []string{"set"}, help: secretCommandSpec("set"), flags: issueMutationFlags, mutation: mutationRequired, run: runLeaf(runSecretSet)},
			{names: []string{"delete"}, help: secretCommandSpec("delete"), flags: issueMutationFlags, mutation: mutationRequired, run: runLeaf(runSecretDelete)},
		},
	}
}

func secretCommandHelps() map[string]commandHelpSpec { return secretGroup().helpSpecs() }

func secretCommandSpec(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{Usage: "fjgo secret list [owner/repo] [flags]", Flags: []string{"--limit <n> (default 100), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin secret list", "fjgo secret list OWNER/REPO --fields name,created"}}
	case "set":
		return commandHelpSpec{Usage: "fjgo secret set [owner/repo] <name> --yes", Flags: []string{"secret value is read from stdin", "--dry-run, --print-request, --json"}, Examples: []string{"printf secret | fjgo -R origin secret set DEPLOY_TOKEN --dry-run --yes", "printf secret | fjgo secret set OWNER/REPO DEPLOY_TOKEN --yes"}}
	default:
		return commandHelpSpec{Usage: "fjgo secret delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin secret delete DEPLOY_TOKEN --dry-run --yes", "fjgo secret delete OWNER/REPO DEPLOY_TOKEN --yes"}}
	}
}

func variableGroup() commandGroup {
	return commandGroup{
		name:     "variable",
		overview: variableHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: variableCommandSpec("list"), flags: pageListFlags, run: runLeaf(runVariableList)},
			{names: []string{"get"}, help: variableCommandSpec("get"), flags: jsonOnlyFlags, run: runLeaf(runVariableGet)},
			{names: []string{"set"}, help: variableCommandSpec("set"), flags: variableSetFlags, mutation: mutationRequired, run: runLeaf(runVariableSet)},
			{names: []string{"delete"}, help: variableCommandSpec("delete"), flags: issueMutationFlags, mutation: mutationRequired, run: runLeaf(runVariableDelete)},
		},
	}
}

func variableCommandHelps() map[string]commandHelpSpec { return variableGroup().helpSpecs() }

func variableCommandSpec(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{Usage: "fjgo variable list [owner/repo] [flags]", Flags: []string{"--limit <n> (default 100), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin variable list", "fjgo variable list OWNER/REPO --fields name,value"}}
	case "get":
		return commandHelpSpec{Usage: "fjgo variable get [owner/repo] <name> [--json]", Examples: []string{"fjgo -R origin variable get BUILD_MODE", "fjgo variable get OWNER/REPO BUILD_MODE --json"}}
	case "set":
		return commandHelpSpec{Usage: "fjgo variable set [owner/repo] <name> [--body <value>] --yes", Flags: []string{"--body <value>; otherwise value is read from stdin", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin variable set BUILD_MODE --body release --dry-run --yes", "printf release | fjgo variable set OWNER/REPO BUILD_MODE --yes"}}
	default:
		return commandHelpSpec{Usage: "fjgo variable delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin variable delete BUILD_MODE --dry-run --yes", "fjgo variable delete OWNER/REPO BUILD_MODE --yes"}}
	}
}

func searchGroup() commandGroup {
	return commandGroup{
		name:     "search",
		overview: searchHelp,
		leaves: []commandLeaf{
			{names: []string{"issues"}, help: searchCommandSpec("issues"), flags: searchIssueFlags, run: searchIssueLeaf("issues")},
			{names: []string{"prs", "pulls"}, help: searchCommandSpec("prs"), flags: searchIssueFlags, run: searchIssueLeaf("pulls")},
			{names: []string{"repos"}, help: searchCommandSpec("repos"), flags: searchRepoFlags, run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runSearchRepos(ctx, client, args, stdout)
			}},
			{names: []string{"topics"}, help: searchCommandSpec("topics"), flags: searchTopicFlags, run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				return runSearchTopics(ctx, client, args, stdout)
			}},
		},
	}
}

func searchCommandHelps() map[string]commandHelpSpec { return searchGroup().helpSpecs() }

func searchIssueLeaf(typ string) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		return runSearchIssues(ctx, client, cfg, args, stdout, typ)
	}
}

func searchCommandSpec(name string) commandHelpSpec {
	switch name {
	case "issues":
		return commandHelpSpec{Usage: "fjgo search issues <query> [flags]", Flags: []string{"--repo <owner/repo>, --owner <owner>", "--state <open|closed|all>, --labels <a,b>", "--sort <key>, --limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin search issues \"login\" --state open", "fjgo search issues \"bug\" --repo OWNER/REPO"}}
	case "repos":
		return commandHelpSpec{Usage: "fjgo search repos <query> [flags]", Flags: []string{"--sort <key>, --limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo search repos \"forgejo cli\" --limit 20", "fjgo search repos fjgo --fields name,private,archived"}}
	case "topics":
		return commandHelpSpec{Usage: "fjgo search topics <query> [--json]", Examples: []string{"fjgo search topics \"actions\"", "fjgo search topics forgejo --json"}}
	default:
		return commandHelpSpec{Usage: "fjgo search prs <query> [flags]", Flags: []string{"--repo <owner/repo>, --owner <owner>", "--state <open|closed|all>, --labels <a,b>", "--sort <key>, --limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin search prs \"fix\" --state open", "fjgo search prs \"fix\" --repo OWNER/REPO"}}
	}
}

func labelGroup() commandGroup {
	return commandGroup{
		name:     "label",
		overview: labelHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: labelCommandSpec("list"), flags: pageListFlags, run: runLeaf(runLabelList)},
			{names: []string{"create"}, help: labelCommandSpec("create"), flags: labelMutateFlags, mutation: mutationRequired, run: runLeaf(runLabelCreate)},
			{names: []string{"edit"}, help: labelCommandSpec("edit"), flags: labelMutateFlags, mutation: mutationRequired, run: runLeaf(runLabelEdit)},
			{names: []string{"delete"}, help: labelCommandSpec("delete"), flags: issueMutationFlags, mutation: mutationRequired, run: runLeaf(runLabelDelete)},
		},
	}
}

func labelCommandHelps() map[string]commandHelpSpec { return labelGroup().helpSpecs() }

func labelCommandSpec(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{Usage: "fjgo label list [owner/repo] [flags]", Flags: []string{"--limit <n> (default 100), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin label list", "fjgo label list OWNER/REPO --fields id,name,color"}}
	case "create":
		return commandHelpSpec{Usage: "fjgo label create [owner/repo] --name <name> [flags] --yes", Flags: []string{"--color <hex>, --description <text>", "--exclusive, --archived", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin label create --name bug --color ff0000 --dry-run --yes", "fjgo label create OWNER/REPO --name bug --yes"}}
	case "edit":
		return commandHelpSpec{Usage: "fjgo label edit [owner/repo] <id> [flags] --yes", Flags: []string{"--name <name>, --color <hex>, --description <text>", "--exclusive <bool>, --archived <bool>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin label edit 42 --name defect --dry-run --yes", "fjgo label edit OWNER/REPO 42 --color ff0000 --yes"}}
	default:
		return commandHelpSpec{Usage: "fjgo label delete [owner/repo] <id> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin label delete 42 --dry-run --yes", "fjgo label delete OWNER/REPO 42 --yes"}}
	}
}

func runGroup() commandGroup {
	return commandGroup{
		name:     "run",
		overview: runHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: runCommandSpec("list"), flags: runListFlags, run: runLeaf(runActionList)},
			{names: []string{"view"}, help: runCommandSpec("view"), flags: runViewFlags, run: runLeaf(runActionView)},
			{names: []string{"watch"}, help: runCommandSpec("watch"), flags: runWatchFlags, run: runLeaf(runActionWatch)},
		},
	}
}

func runCommandHelps() map[string]commandHelpSpec { return runGroup().helpSpecs() }

func runCommandSpec(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{
			Usage:    "fjgo run list [owner/repo] [flags]",
			Flags:    []string{"--status <status>, --workflow <file>, --ref <ref>, --event <event>, --sha <sha>", "--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"},
			Examples: []string{"fjgo -R origin run list --status failure", "fjgo run list OWNER/REPO --fields id,status,workflow"},
		}
	case "view":
		return commandHelpSpec{Usage: "fjgo run view [owner/repo] <id> [flags]", Flags: []string{"--log-failed", "--full", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin run view 123", "fjgo run view OWNER/REPO 123 --log-failed --full"}}
	default:
		return commandHelpSpec{Usage: "fjgo run watch [owner/repo] <id> [flags]", Flags: []string{"--interval <duration> (default 5s)", "--timeout <duration> (default 10m)", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin run watch 123 --timeout 2m", "fjgo run watch OWNER/REPO 123 --interval 10s"}}
	}
}

func workflowGroup() commandGroup {
	return commandGroup{
		name:     "workflow",
		overview: workflowHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: workflowCommandSpec("list"), flags: workflowListFlags, run: runLeaf(runWorkflowList)},
			{names: []string{"view"}, help: workflowCommandSpec("view"), flags: workflowViewFlags, run: runLeaf(runWorkflowView)},
			{names: []string{"run"}, help: workflowCommandSpec("run"), flags: workflowRunFlags, mutation: mutationRequired, run: runLeaf(runWorkflowDispatch)},
		},
	}
}

func workflowCommandHelps() map[string]commandHelpSpec { return workflowGroup().helpSpecs() }

func workflowCommandSpec(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{Usage: "fjgo workflow list [owner/repo] [--ref branch] [--json]", Examples: []string{"fjgo -R origin workflow list", "fjgo workflow list OWNER/REPO --ref main"}}
	case "view":
		return commandHelpSpec{Usage: "fjgo workflow view [owner/repo] <workflow.yml> [flags]", Flags: []string{"--ref <branch>", "--full", "--json"}, Examples: []string{"fjgo -R origin workflow view verify.yml --full", "fjgo workflow view OWNER/REPO verify.yml --ref main"}}
	default:
		return commandHelpSpec{Usage: "fjgo workflow run [owner/repo] <workflow.yml> --ref <ref> [flags] --yes", Flags: []string{"--input <key=value> (repeatable)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin workflow run verify.yml --ref main --input smoke=true --dry-run --yes", "fjgo workflow run OWNER/REPO verify.yml --ref main --yes"}}
	}
}

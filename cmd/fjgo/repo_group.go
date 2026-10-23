package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"

	"github.com/astrazds/fjgo/internal/forgejo"
)

var repoListFlags = newFlagSet([]string{"--json", "--fields", "--limit", "--page", "--user", "--org", "--q", "--sort", "--private"}, []string{"--fields", "--limit", "--page", "--user", "--org", "--q", "--sort", "--private"})
var repoCreateFlags = newFlagSet([]string{"--org", "--name", "--description", "--website", "--default-branch", "--gitignores", "--license", "--readme", "--object-format", "--private", "--auto-init", "--template", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--org", "--name", "--description", "--website", "--default-branch", "--gitignores", "--license", "--readme", "--object-format"})
var repoGetFlags = newFlagSet([]string{"--json", "--fields"}, []string{"--fields"})
var repoEditFlags = newFlagSet([]string{"--name", "--description", "--website", "--default-branch", "--private", "--archived", "--has-issues", "--has-pulls", "--has-wiki", "--has-actions", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--description", "--website", "--default-branch", "--private", "--archived", "--has-issues", "--has-pulls", "--has-wiki", "--has-actions"})
var repoForkFlags = newFlagSet([]string{"--name", "--org", "--organization", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--org", "--organization"})
var repoBranchListFlags = pageListFlags
var repoBranchGetFlags = jsonOnlyFlags
var repoBranchCreateFlags = newFlagSet([]string{"--name", "--from", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--name", "--from"})
var repoBranchParentFlags = newFlagSet([]string{"--fields", "--json", "--name", "--from", "--yes", "--dry-run", "--print-request"}, []string{"--fields", "--name", "--from"})
var repoCollabListFlags = pageListFlags
var repoCollabGetFlags = jsonOnlyFlags
var repoCollabAddFlags = newFlagSet([]string{"--permission", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--permission"})
var repoCollabParentFlags = newFlagSet([]string{"--permission", "--fields", "--json", "--yes", "--dry-run", "--print-request"}, []string{"--permission", "--fields"})
var repoProtectionListFlags = prFieldsFlags
var repoProtectionGetFlags = jsonOnlyFlags
var repoProtectionMutateFlags = newFlagSet([]string{"--name", "--required-approvals", "--status-check", "--enable-status-check", "--require-signed-commits", "--enable-push", "--apply-to-admins", "--yes", "--dry-run", "--print-request", "--json", "--full"}, []string{"--name", "--required-approvals", "--status-check", "--enable-status-check", "--require-signed-commits", "--enable-push", "--apply-to-admins"})
var repoProtectionParentFlags = newFlagSet([]string{"--name", "--required-approvals", "--fields", "--json", "--full", "--yes", "--dry-run", "--print-request"}, []string{"--name", "--required-approvals", "--fields"})
var repoTopicFlags = newFlagSet([]string{"--set", "--yes", "--dry-run", "--print-request", "--json"}, []string{"--set"})
var repoAvatarFlags = newFlagSet([]string{"--yes", "--dry-run", "--print-request"}, nil)
var repoIssueCloseFlags = newFlagSet([]string{"--yes", "--dry-run", "--print-request"}, nil)
var repoIssueCommentFlags = newFlagSet([]string{"-body", "--yes", "--dry-run", "--print-request"}, []string{"-body"})
var repoIssueParentFlags = newFlagSet([]string{"-body", "--yes", "--dry-run", "--print-request"}, []string{"-body"})

func repoGroup() commandGroup {
	branches := repoBranchGroup()
	collaborators := repoCollaboratorGroup()
	protection := repoProtectionGroup()
	issues := repoIssueGroup()
	return commandGroup{
		name:     "repo",
		overview: repoHelp,
		leaves: []commandLeaf{
			{names: []string{"list"}, help: repoTopHelp("list"), flags: repoListFlags, run: func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
				args, jsonOut, _ := peelJSONFull(args)
				return runRepoList(ctx, client, args, stdout, jsonOut)
			}},
			{names: []string{"create"}, help: repoTopHelp("create"), flags: repoCreateFlags, mutation: mutationRequired, run: repoFullLeaf(runRepoCreate)},
			{names: []string{"get"}, help: repoTopHelp("get"), flags: repoGetFlags, run: runRepoGetLeaf},
			{names: []string{"edit"}, help: repoTopHelp("edit"), flags: repoEditFlags, mutation: mutationRequired, run: repoConfigLeaf(runRepoEdit)},
			{names: []string{"fork"}, help: repoTopHelp("fork"), flags: repoForkFlags, mutation: mutationRequired, run: repoConfigLeaf(runRepoFork)},
			{names: []string{"branches"}, help: repoTopHelp("branches"), flags: repoBranchParentFlags, children: &branches, run: dispatchFixed(repoBranchGroup)},
			{names: []string{"collaborators"}, help: repoTopHelp("collaborators"), flags: repoCollabParentFlags, children: &collaborators, run: dispatchFixed(repoCollaboratorGroup)},
			{names: []string{"branch-protection", "branch-protections"}, help: repoTopHelp("branch-protection"), flags: repoProtectionParentFlags, children: &protection, run: dispatchFixed(repoProtectionGroup)},
			{names: []string{"topics"}, help: repoTopHelp("topics"), flags: repoTopicFlags, run: runRepoTopicsLeaf},
			{names: []string{"avatar"}, help: repoTopHelp("avatar"), flags: repoAvatarFlags, mutation: mutationRequired, run: runRepoAvatarLeaf},
			{names: []string{"issue"}, help: repoTopHelp("issue"), flags: repoIssueParentFlags, children: &issues, run: runRepoIssueLeaf},
		},
	}
}

func repoCommandHelps() map[string]commandHelpSpec { return repoGroup().helpSpecs() }

func repoFullLeaf(fn func(context.Context, *forgejo.Client, []string, io.Writer, bool, bool) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, full := peelJSONFull(args)
		return fn(ctx, client, args, stdout, jsonOut, full)
	}
}

func repoConfigLeaf(fn func(context.Context, *forgejo.Client, runConfig, []string, io.Writer, bool, bool) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, full := peelJSONFull(args)
		return fn(ctx, client, cfg, args, stdout, jsonOut, full)
	}
}

func repoJSONLeaf(fn func(context.Context, *forgejo.Client, runConfig, []string, io.Writer, bool) error) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, _ := peelJSONFull(args)
		return fn(ctx, client, cfg, args, stdout, jsonOut)
	}
}

func repoBranchGroup() commandGroup {
	remove := commandLeaf{names: []string{"delete", "remove"}, help: repoBranchDeleteHelp(), flags: issueMutationFlags, mutation: mutationRequired, run: repoJSONLeaf(runRepoBranchDelete)}
	return commandGroup{
		name:     "repo branches",
		overview: func() string { return repoBranchOverview },
		leaves: []commandLeaf{
			{names: []string{"list"}, help: repoBranchListHelp(), flags: repoBranchListFlags, run: repoJSONLeaf(runRepoBranchesList)},
			{names: []string{"get"}, help: repoBranchGetHelp(), flags: repoBranchGetFlags, run: repoJSONLeaf(runRepoBranchGet)},
			{names: []string{"create"}, help: repoBranchCreateHelp(), flags: repoBranchCreateFlags, mutation: mutationRequired, run: repoJSONLeaf(runRepoBranchCreate)},
			remove,
		},
	}
}

func repoCollaboratorGroup() commandGroup {
	remove := commandLeaf{names: []string{"remove", "delete"}, help: repoCollabRemoveHelp(), flags: issueMutationFlags, mutation: mutationRequired, run: repoJSONLeaf(runRepoCollaboratorRemove)}
	return commandGroup{
		name:     "repo collaborators",
		overview: func() string { return repoCollabOverview },
		leaves: []commandLeaf{
			{names: []string{"list"}, help: repoCollabListHelp(), flags: repoCollabListFlags, run: repoJSONLeaf(runRepoCollaboratorsList)},
			{names: []string{"check", "get"}, help: repoCollabCheckHelp(), flags: repoCollabGetFlags, run: repoJSONLeaf(runRepoCollaboratorCheck)},
			{names: []string{"permission"}, help: repoCollabPermissionHelp(), flags: repoCollabGetFlags, run: repoJSONLeaf(runRepoCollaboratorPermission)},
			{names: []string{"add"}, help: repoCollabAddHelp(), flags: repoCollabAddFlags, mutation: mutationRequired, run: repoJSONLeaf(runRepoCollaboratorAdd)},
			remove,
		},
	}
}

func repoProtectionGroup() commandGroup {
	remove := commandLeaf{names: []string{"delete", "remove"}, help: repoProtectionDeleteHelp(), flags: issueMutationFlags, mutation: mutationRequired, run: repoJSONLeaf(runRepoBranchProtectionDelete)}
	return commandGroup{
		name:     "repo branch-protection",
		overview: func() string { return repoProtectionOverview },
		leaves: []commandLeaf{
			{names: []string{"list"}, help: repoProtectionListHelp(), flags: repoProtectionListFlags, run: repoJSONLeaf(runRepoBranchProtectionList)},
			{names: []string{"get"}, help: repoProtectionGetHelp(), flags: repoProtectionGetFlags, run: repoJSONLeaf(runRepoBranchProtectionGet)},
			{names: []string{"create"}, help: repoProtectionMutateHelp("create"), flags: repoProtectionMutateFlags, mutation: mutationRequired, run: repoProtectionLeaf(false)},
			{names: []string{"edit"}, help: repoProtectionMutateHelp("edit"), flags: repoProtectionMutateFlags, mutation: mutationRequired, run: repoProtectionLeaf(true)},
			remove,
		},
	}
}

func repoProtectionLeaf(edit bool) leafFunc {
	return func(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
		args, jsonOut, full := peelJSONFull(args)
		return runRepoBranchProtectionMutate(ctx, client, cfg, args, stdout, jsonOut, full, edit)
	}
}

func repoIssueGroup() commandGroup {
	return commandGroup{
		name: "repo issue",
		leaves: []commandLeaf{
			{names: []string{"close"}, help: repoIssueCloseHelp(), flags: repoIssueCloseFlags, mutation: mutationRequired, run: runLeaf(runRepoIssueClose)},
			{names: []string{"comment"}, help: repoIssueCommentHelp(), flags: repoIssueCommentFlags, mutation: mutationRequired, run: runLeaf(runRepoIssueComment)},
		},
	}
}

const repoBranchOverview = "usage: fjgo repo branches <list|get|create|delete> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo branches list\n  fjgo --repo OWNER/REPO repo branches create --name feature --from main --dry-run --yes\n  fjgo --repo OWNER/REPO repo branches delete feature --dry-run --yes"
const repoCollabOverview = "usage: fjgo repo collaborators <list|check|permission|add|remove> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo collaborators list\n  fjgo --repo OWNER/REPO repo collaborators add alice --permission write --dry-run --yes\n  fjgo --repo OWNER/REPO repo collaborators permission alice"
const repoProtectionOverview = "usage: fjgo repo branch-protection <list|get|create|edit|delete> [owner/repo] [flags]\nexamples:\n  fjgo --repo OWNER/REPO repo branch-protection list\n  fjgo --repo OWNER/REPO repo branch-protection create --name main --required-approvals 1 --dry-run --yes\n  fjgo --repo OWNER/REPO repo branch-protection delete main --dry-run --yes"

func repoTopHelp(name string) commandHelpSpec {
	switch name {
	case "list":
		return commandHelpSpec{Usage: "fjgo repo list [flags]", Flags: []string{"--user <user>, --org <org>, --q <text>", "--limit <n> (default " + defaultListLimit + "), --page <n>, --sort <key>, --private <bool>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo repo list --org OWNER --fields name,private,archived", "fjgo repo list --q \"forgejo cli\" --limit 20"}}
	case "create":
		return commandHelpSpec{Usage: "fjgo repo create <name> [flags] --yes", Flags: []string{"--org <org>, --description <text>, --private, --auto-init", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo repo create demo --private --dry-run --yes", "fjgo repo create demo --org OWNER --yes"}}
	case "get":
		return commandHelpSpec{Usage: "fjgo repo get [owner/repo] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo --repo OWNER/REPO repo get", "fjgo repo get OWNER/REPO --fields full_name,default_branch,open_issues"}}
	case "edit":
		return commandHelpSpec{Usage: "fjgo repo edit [owner/repo] [flags] --yes", Flags: []string{"--description <text>", "--private <bool>, --archived <bool>, --has-issues <bool>, --has-pulls <bool>, --has-wiki <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO repo edit --description \"CLI\" --dry-run --yes", "fjgo repo edit OWNER/REPO --archived false --yes"}}
	case "fork":
		return commandHelpSpec{Usage: "fjgo repo fork [owner/repo] [flags] --yes", Flags: []string{"--name <name>, --org <org>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo --repo OWNER/REPO repo fork --dry-run --yes", "fjgo repo fork OWNER/REPO --name fork --yes"}}
	case "branches":
		return commandHelpSpec{Usage: "fjgo repo branches <list|get|create|delete> [owner/repo] [flags]", Flags: []string{"--fields <a,b,c>, --json", "--name <branch>, --from <branch>", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo branches list", "fjgo --repo OWNER/REPO repo branches create --name feature --from main --dry-run --yes"}}
	case "collaborators":
		return commandHelpSpec{Usage: "fjgo repo collaborators <list|check|permission|add|remove> [owner/repo] [flags]", Flags: []string{"--permission <read|write|admin>", "--fields <a,b,c>, --json", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo collaborators list", "fjgo --repo OWNER/REPO repo collaborators add alice --permission write --dry-run --yes"}}
	case "branch-protection":
		return commandHelpSpec{Usage: "fjgo repo branch-protection <list|get|create|edit|delete> [owner/repo] [flags]", Flags: []string{"--name <branch>, --required-approvals <n>", "--fields <a,b,c>, --json, --full", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo branch-protection list", "fjgo --repo OWNER/REPO repo branch-protection create --name main --required-approvals 1 --dry-run --yes"}}
	case "topics":
		return commandHelpSpec{Usage: "fjgo repo topics [owner/repo] [--set comma,separated,topics] [flags]", Flags: []string{"--set <topic,topic>", "--yes, --dry-run, --print-request, --json"}, Examples: []string{"fjgo repo topics OWNER/REPO", "fjgo repo topics OWNER/REPO --set forgejo,go --dry-run --yes"}}
	case "avatar":
		return commandHelpSpec{Usage: "fjgo repo avatar [owner/repo] <png> --yes", Flags: []string{"--dry-run, --print-request"}, Examples: []string{"fjgo --repo OWNER/REPO repo avatar icon.png --dry-run --yes", "fjgo repo avatar OWNER/REPO icon.png --yes"}}
	default:
		return commandHelpSpec{Usage: "fjgo repo issue <close|comment> [owner/repo] <index> [flags]", Flags: []string{"-body <json|@file|->", "--yes, --dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue close 42 --dry-run --yes", "fjgo -R origin repo issue comment 42 -body '{\"body\":\"note\"}' --dry-run --yes"}}
	}
}

func repoBranchListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branches list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin repo branches list", "fjgo repo branches list OWNER/REPO --fields name,protected"}}
}
func repoBranchGetHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branches get [owner/repo] <branch> [--json]", Examples: []string{"fjgo -R origin repo branches get main", "fjgo repo branches get OWNER/REPO main --json"}}
}
func repoBranchCreateHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branches create [owner/repo] --name <branch> [--from <ref>] --yes", Flags: []string{"--from <ref>", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branches create --name feature --from main --dry-run --yes", "fjgo repo branches create OWNER/REPO --name feature --yes"}}
}
func repoBranchDeleteHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branches delete [owner/repo] <branch> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branches delete feature --dry-run --yes", "fjgo repo branches delete OWNER/REPO feature --yes"}}
}
func repoCollabListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo collaborators list [owner/repo] [flags]", Flags: []string{"--limit <n> (default " + defaultListLimit + "), --page <n>", "--fields <a,b,c>, --json"}, Examples: []string{"fjgo -R origin repo collaborators list", "fjgo repo collaborators list OWNER/REPO --fields login,active"}}
}
func repoCollabCheckHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo collaborators check [owner/repo] <user> [--json]", Examples: []string{"fjgo -R origin repo collaborators check alice", "fjgo repo collaborators check OWNER/REPO alice --json"}}
}
func repoCollabPermissionHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo collaborators permission [owner/repo] <user> [--json]", Examples: []string{"fjgo -R origin repo collaborators permission alice", "fjgo repo collaborators permission OWNER/REPO alice --json"}}
}
func repoCollabAddHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo collaborators add [owner/repo] <user> [--permission <level>] --yes", Flags: []string{"--permission <read|write|admin> (default write)", "--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo collaborators add alice --permission write --dry-run --yes", "fjgo repo collaborators add OWNER/REPO alice --yes"}}
}
func repoCollabRemoveHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo collaborators remove [owner/repo] <user> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo collaborators remove alice --dry-run --yes", "fjgo repo collaborators remove OWNER/REPO alice --yes"}}
}
func repoProtectionListHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branch-protection list [owner/repo] [--fields <a,b,c>] [--json]", Examples: []string{"fjgo -R origin repo branch-protection list", "fjgo repo branch-protection list OWNER/REPO --fields name,required_approvals"}}
}
func repoProtectionGetHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branch-protection get [owner/repo] <name> [--json]", Examples: []string{"fjgo -R origin repo branch-protection get main", "fjgo repo branch-protection get OWNER/REPO main --json"}}
}
func repoProtectionMutateHelp(action string) commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branch-protection " + action + " [owner/repo] <name> [flags] --yes", Flags: []string{"--required-approvals <n>, --status-check <context> (repeatable)", "--enable-status-check <bool>, --require-signed-commits <bool>, --enable-push <bool>, --apply-to-admins <bool>", "--dry-run, --print-request, --json, --full"}, Examples: []string{"fjgo -R origin repo branch-protection " + action + " main --required-approvals 1 --dry-run --yes", "fjgo repo branch-protection " + action + " OWNER/REPO main --require-signed-commits true --yes"}}
}
func repoProtectionDeleteHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo branch-protection delete [owner/repo] <name> --yes", Flags: []string{"--dry-run, --print-request, --json"}, Examples: []string{"fjgo -R origin repo branch-protection delete main --dry-run --yes", "fjgo repo branch-protection delete OWNER/REPO main --yes"}}
}
func repoIssueCloseHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo issue close [owner/repo] <index> --yes", Flags: []string{"--dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue close 42 --dry-run --yes", "fjgo repo issue close OWNER/REPO 42 --yes"}}
}
func repoIssueCommentHelp() commandHelpSpec {
	return commandHelpSpec{Usage: "fjgo repo issue comment [owner/repo] <index> -body <json|@file|-> --yes", Flags: []string{"-body <json|@file|->", "--dry-run, --print-request"}, Examples: []string{"fjgo -R origin repo issue comment 42 -body '{\"body\":\"note\"}' --dry-run --yes", "fjgo repo issue comment OWNER/REPO 42 -body @comment.json --yes"}}
}

func runRepoGetLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut, _ := peelJSONFull(args)
	if err := rejectUnknownFlags(args, "repo get", repoGetFlags); err != nil {
		return err
	}
	restArgs, fieldsArg, err := takeFieldsFlag(args)
	if err != nil {
		return err
	}
	fields, err := validatedFields(fieldsArg, repoViewDefaultFields, repoViewFields, "repo get")
	if err != nil {
		return err
	}
	ref, rest, err := repoFromArgs(cfg, restArgs)
	if err != nil || len(rest) != 0 {
		return newUsageError("usage: fjgo repo get [owner/repo]", "Pass `owner/repo`, root `--repo OWNER/REPO`, set FJGO_REPO=OWNER/REPO, or use `-R origin`")
	}
	out, err := client.RepoGet(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	return writeRepository(stdout, out, jsonOut, fields)
}

func runRepoTopicsLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, jsonOut, _ := peelJSONFull(args)
	ref, rest, err := repoFromArgs(cfg, args)
	if err != nil {
		return newUsageError("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]", "Pass `owner/repo` or use `-R origin`")
	}
	if err := rejectUnknownFlags(rest, "repo topics", repoTopicFlags); err != nil {
		return err
	}
	args, yes, _ := takeCuratedYes(rest, mutationNone)
	args, dryRun := takeDryRunFlag(args)
	args, topics, err := takeSetFlag(args)
	if err != nil {
		return err
	}
	if len(args) != 0 {
		return newUsageError("usage: fjgo repo topics <owner/repo> [--set comma,separated,topics]")
	}
	if topics != "" {
		if yes.command == "" {
			return errors.New("repo topics --set requires --yes")
		}
		if dryRun {
			op, _ := forgejo.OperationByID("repoUpdateTopics")
			return writeRequestPreview(stdout, requestPreview{
				Operation:   op.ID,
				Method:      op.Method,
				Path:        previewPath(op, map[string]string{"owner": ref.Owner, "repo": ref.Repo}),
				Body:        map[string]any{"topics": splitCSV(topics)},
				AuthPresent: clientHasAuth(client),
				RequiresYes: true,
				YesProvided: yes.command != "",
			})
		}
		if err := client.RepoUpdateTopics(ctx, ref.Owner, ref.Repo, &forgejo.RepoTopicOptions{Topics: splitCSV(topics)}, forgejo.RequestOptions{}); err != nil {
			return err
		}
	}
	out, err := client.RepoListTopics(ctx, ref.Owner, ref.Repo, forgejo.RequestOptions{})
	if err != nil {
		return err
	}
	if out == nil {
		return writeTopics(stdout, ref, nil, jsonOut)
	}
	return writeTopics(stdout, ref, out.TopicNames, jsonOut)
}

func runRepoAvatarLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	args, _, _ = peelJSONFull(args)
	ref := repoRef{}
	rest := args
	if cfg.RemoteRepo != nil {
		ref = *cfg.RemoteRepo
	} else {
		var err error
		ref, rest, err = repoFromArgs(cfg, rest)
		if err != nil {
			return newUsageError("usage: fjgo repo avatar <owner/repo> <png> --yes", "Pass `owner/repo` or use `-R origin`")
		}
	}
	if err := rejectUnknownFlags(rest, "repo avatar", repoAvatarFlags); err != nil {
		return err
	}
	rest, yes, _ := takeCuratedYes(rest, mutationNone)
	rest, dryRun := takeDryRunFlag(rest)
	if len(rest) != 1 {
		return newUsageError("usage: fjgo repo avatar <owner/repo> <png> --yes")
	}
	if yes.command == "" {
		return newUsageError("repo avatar requires --yes")
	}
	if dryRun {
		op, _ := forgejo.OperationByID("repoUpdateAvatar")
		return writeRequestPreview(stdout, requestPreview{
			Operation:   op.ID,
			Method:      op.Method,
			Path:        previewPath(op, map[string]string{"owner": ref.Owner, "repo": ref.Repo}),
			Body:        map[string]string{"image_file": rest[0], "image_base64": "omitted"},
			AuthPresent: clientHasAuth(client),
			RequiresYes: true,
			YesProvided: yes.command != "",
		})
	}
	b, err := os.ReadFile(rest[0])
	if err != nil {
		return err
	}
	if err := client.RepoUpdateAvatar(ctx, ref.Owner, ref.Repo, &forgejo.UpdateRepoAvatarOption{Image: base64.StdEncoding.EncodeToString(b)}, forgejo.RequestOptions{}); err != nil {
		return err
	}
	return writeTOON(stdout, map[string]any{"avatar": "updated"})
}

func runRepoIssueLeaf(ctx context.Context, client *forgejo.Client, cfg runConfig, args []string, stdout io.Writer, matched string) error {
	if len(args) == 0 {
		return newUsageError("usage: fjgo repo issue <close|comment>", "Run `fjgo repo issue close --help` or `fjgo repo issue comment --help`")
	}
	return dispatchGroup(repoIssueGroup(), ctx, client, cfg, args, stdout)
}

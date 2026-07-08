package main

import "fmt"

type suggestionContext struct {
	Domain string
	Action string
	State  string
	Empty  bool
	ID     string
	Repo   *repoRef
}

type suggestionEntry struct {
	match func(suggestionContext) bool
	lines func(suggestionContext) []string
}

func suggestionLines(ctx suggestionContext) []string {
	for _, entry := range suggestionTable {
		if entry.match(ctx) {
			return entry.lines(ctx)
		}
	}
	return nil
}

func suggestionHelp(ctx suggestionContext) any {
	return helpBlock(suggestionLines(ctx))
}

func commandForRepo(ref *repoRef, command string) string {
	if ref == nil || ref.Owner == "" || ref.Repo == "" {
		return "fjgo " + command
	}
	return fmt.Sprintf("fjgo --repo %s/%s %s", ref.Owner, ref.Repo, command)
}

func refPtr(ref repoRef) *repoRef {
	return &repoRef{Owner: ref.Owner, Repo: ref.Repo}
}

var suggestionTable = []suggestionEntry{
	{
		match: func(c suggestionContext) bool { return c.Domain == "home" && c.Action == "missing_repo" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `fjgo --repo OWNER/REPO` or set FJGO_REPO=OWNER/REPO for explicit repo context",
				"Run `fjgo -R origin` inside a Forgejo-backed checkout for git remote context",
				"Run `fjgo setup hooks` to install optional agent session context",
				"Run `fjgo api list <filter>` to discover generated Forgejo operations",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "home" && c.Action == "repo" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "issue list --state open") + "` for open issues",
				"Run `" + commandForRepo(c.Repo, "pr list --state open") + "` for open pull requests",
				"Run `" + commandForRepo(c.Repo, "run list") + "` for recent Forgejo Actions runs",
				"Run `" + commandForRepo(c.Repo, "doctor") + "` for redacted repo/auth diagnostics",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "issue" && c.Action == "list" && !c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "issue view <number>") + "` to see issue details",
				"Run `" + commandForRepo(c.Repo, "issue create --title \"...\" --body-file <path> --dry-run --yes") + "` to create an issue",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "issue" && c.Action == "list" && c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "issue create --title \"...\" --body-file <path> --dry-run --yes") + "` to create an issue",
				"Run `" + commandForRepo(c.Repo, "issue list --state closed") + "` to see closed issues",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "issue" && c.Action == "view" && c.State == "open" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "issue comment "+c.ID+" --body-file <path> --dry-run --yes") + "` to comment",
				"Run `" + commandForRepo(c.Repo, "issue close "+c.ID+" --dry-run --yes") + "` to close",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "issue" && c.Action == "mutate" },
		lines: func(c suggestionContext) []string {
			return []string{"Run `" + commandForRepo(c.Repo, "issue view "+c.ID+" --comments") + "` to inspect the issue"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "pr" && c.Action == "list" && !c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "pr view <number>") + "` to see PR details",
				"Run `" + commandForRepo(c.Repo, "pr checks <number>") + "` to inspect commit statuses",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "pr" && c.Action == "list" && c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "pr create --title \"...\" --head <branch> --dry-run --yes") + "` to create a PR",
				"Run `" + commandForRepo(c.Repo, "pr list --state closed") + "` to see closed PRs",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "pr" && c.Action == "view" && c.State == "open" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "pr checks "+c.ID) + "` to see CI status",
				"Run `" + commandForRepo(c.Repo, "pr review "+c.ID+" --approve --dry-run --yes") + "` to preview approval",
				"Run `" + commandForRepo(c.Repo, "pr diff "+c.ID) + "` to inspect the diff",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "run" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "run view <id>") + "` to inspect a run",
				"Run `" + commandForRepo(c.Repo, "workflow run <workflow.yml> --ref <ref> --dry-run --yes") + "` to dispatch a workflow",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "workflow" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{"Run `" + commandForRepo(c.Repo, "workflow run <workflow.yml> --ref <ref> --dry-run --yes") + "` to dispatch"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "repo" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `fjgo repo get OWNER/REPO` to inspect repository details",
				"Run `fjgo repo create --name <name> --dry-run --yes` to preview a repository create",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "repo" && c.Action == "view" },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "repo branches list") + "` to inspect branches",
				"Run `" + commandForRepo(c.Repo, "release list") + "` to inspect releases",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "release" && c.Action == "list" && c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{"Run `" + commandForRepo(c.Repo, "release create <tag> --body-file <path> --dry-run --yes") + "` to preview creating a release"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "release" && c.Action == "list" && !c.Empty },
		lines: func(c suggestionContext) []string {
			return []string{
				"Run `" + commandForRepo(c.Repo, "release view <id|tag>") + "` to inspect a release",
				"Run `" + commandForRepo(c.Repo, "release assets list <release-id>") + "` to list assets",
			}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "label" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{"Run `" + commandForRepo(c.Repo, "label create --name <name> --color <hex> --dry-run --yes") + "` to create a label"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "secret" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{"Run `echo -n \"<value>\" | " + commandForRepo(c.Repo, "secret set <name> --dry-run --yes") + "` to set a secret"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "variable" && c.Action == "list" },
		lines: func(c suggestionContext) []string {
			return []string{"Run `" + commandForRepo(c.Repo, "variable set <name> --body <value> --dry-run --yes") + "` to set a variable"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "error" && c.Action == "auth" },
		lines: func(c suggestionContext) []string {
			return []string{"Set FJGO_TOKEN or pass -token with a Forgejo access token for this base URL"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "error" && c.Action == "forbidden" },
		lines: func(c suggestionContext) []string {
			return []string{"Check that the token has permission for this repository and action"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "error" && c.Action == "not_found" },
		lines: func(c suggestionContext) []string {
			return []string{"Check the owner/repo, issue number, pull request number, release ID, tag, or base URL"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "error" && c.Action == "validation" },
		lines: func(c suggestionContext) []string {
			return []string{"Run the matching `fjgo <command> --dry-run --yes` or `fjgo api inspect <operationId>` to verify the request body"}
		},
	},
	{
		match: func(c suggestionContext) bool { return c.Domain == "error" && c.Action == "missing_repo" },
		lines: func(c suggestionContext) []string {
			return []string{"Pass `owner/repo`, use root `--repo OWNER/REPO`, set FJGO_REPO=OWNER/REPO, or use `-R origin` with a Forgejo git remote"}
		},
	},
}

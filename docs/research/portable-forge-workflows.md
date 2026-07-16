# Portable agent-workflow patterns in official forge tooling

Date: 2026-07-16

## Question

Using official GitHub and GitLab documentation and first-party tooling sources,
which agent-facing workflow, context, safety, and automation patterns demonstrate
recurring value that could inform `fjgo` without expanding it to support competing
forges? Which patterns are portable, and which are platform-specific?

## Executive answer

The strongest portable signal is **not another layer of endpoint aliases**. GitHub
and GitLab converge on a common substrate that `fjgo` already largely has:
repository-aware CLI context, selected machine-readable output, curated commands,
and an authenticated raw-API escape hatch. Their newer agent products add value
above that substrate in three places:

1. **Named, outcome-level workflows** that turn an issue, review, or failed
   pipeline into a bounded sequence with an inspectable result.
2. **Policy at the execution boundary**, where a proposed read, write, or delete
   is allowed, requires approval, or is denied, with enough attribution to audit
   what happened.
3. **Repository-owned context packages**—instructions and task-specific skills
   stored alongside the code and loaded only when relevant.

For `fjgo`, the first is the clearest strategic product opportunity. The second
is a strong differentiator for safely increasing autonomy. The third is valuable
supporting infrastructure, but `fjgo` already ships a skill and ambient context
hooks, so it is more likely to strengthen another direction than to justify a
standalone product bet.

The evidence is convergence in first-party product design, not usage telemetry.
Official documentation establishes that both vendors intentionally expose these
patterns; it does not establish how frequently customers use each feature.

## Portable patterns

### 1. Resolve context implicitly, but preserve explicit targeting

Both first-party CLIs treat the current Git repository as useful context while
keeping explicit overrides:

- `gh api` expands owner, repository, and branch placeholders from the current
  repository or `GH_REPO`; `gh repo set-default` can explicitly select which
  remote repository API commands should use ([GitHub CLI `gh api`](https://cli.github.com/manual/gh_api),
  [GitHub CLI `gh repo set-default`](https://cli.github.com/manual/gh_repo_set-default)).
- `glab` detects an authenticated hostname from Git remotes, supports multiple
  self-managed instances, and lets callers override repository or hostname;
  `glab api` expands repository placeholders from the current directory
  ([GitLab CLI overview](https://docs.gitlab.com/cli/),
  [GitLab CLI `glab api`](https://docs.gitlab.com/cli/api/)).

This validates `fjgo`'s explicit `--repo`/`FJGO_REPO` and remote-scoped `-R`
design. The portable lesson is **predictable precedence**, not ever more ambient
guessing. Context should be visible in previews and results so an agent can verify
the target before mutation.

### 2. Pair curated workflows with a complete API escape hatch

GitHub and GitLab both provide high-level issue, pull/merge request, release, and
workflow commands, while `gh api` and `glab api` retain authenticated REST and
GraphQL access. Both raw commands support repository placeholders, typed scalar
fields, request bodies from files or stdin, pagination, and multipart or other
special request forms ([GitHub CLI `gh api`](https://cli.github.com/manual/gh_api),
[GitLab CLI `glab api`](https://docs.gitlab.com/cli/api/)).

That supports `fjgo`'s existing “curated common path plus generated/raw full
coverage” architecture. Additional endpoint wrappers should remain evidence-led.
The strategic seam is composing existing operations around a user outcome, not
duplicating the API surface by hand.

### 3. Make output selectable, structured, bounded, and streamable

GitHub CLI commands can expose an explicit set of JSON fields, then filter or
format that result with `--jq` or templates. Omitting the `--json` value discovers
available fields ([GitHub CLI formatting](https://cli.github.com/manual/gh_help_formatting)).
GitLab CLI increasingly offers text/JSON selection and `--jq`; its raw API also
offers NDJSON for paginated, memory-efficient streams
([`glab repo view`](https://docs.gitlab.com/cli/repo/view/),
[`glab api`](https://docs.gitlab.com/cli/api/)). `glab ci status` separately
offers compact and wait modes, though its JSON mode cannot be combined with all
interactive/watch modes ([`glab ci status`](https://docs.gitlab.com/cli/ci/status/)).

This strongly validates `fjgo`'s compact TOON default, `--fields`, deterministic
JSON escape hatch, truncation, and watch commands. Two incremental lessons are
worth carrying forward:

- field discovery should be uniform and cheap across curated commands;
- long-running or high-cardinality workflows need a stable event/stream form,
  not only a final aggregate document.

These are quality improvements to the substrate, not by themselves a strategic
direction.

### 4. Package repeatable outcomes, not just command shortcuts

At the automation layer, both platforms distinguish reusable components from
one-off commands:

- GitHub reusable workflows declare typed inputs and secrets, return outputs,
  can be nested, and can be pinned to a commit SHA; GitHub explicitly frames them
  as a centrally maintained library of proven workflows
  ([GitHub reusable workflows](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows),
  [reusing workflow configurations](https://docs.github.com/en/actions/concepts/workflows-and-actions/reusing-workflow-configurations)).
- GitLab CI/CD components are reusable pipeline units that accept input
  parameters and can compose part or all of a pipeline
  ([GitLab CI/CD components](https://docs.gitlab.com/ci/components/)).
- GitHub's agent customization guide describes a skill as an on-demand,
  repeatable workflow; GitLab describes Agent Skills as shareable skills and
  custom flows as multi-agent task compositions
  ([GitHub Copilot CLI customization](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/comparing-cli-features),
  [GitLab Agent Platform customization](https://docs.gitlab.com/user/duo_agent_platform/customize/)).

Aliases are useful but shallower: both CLIs support aliases for frequent commands
or shell pipelines ([GitHub CLI aliases](https://cli.github.com/manual/gh_alias),
[GitLab CLI aliases](https://docs.gitlab.com/cli/alias/)). The more important
portable pattern is a **named workflow contract** with declared inputs, bounded
privileges, progress, outputs, and a restart/review point.

For `fjgo`, this suggests a small Forgejo-native workflow layer over existing
operations—for example, triage an issue, prepare a release, review a pull request,
or diagnose a failed Actions run. It should produce a plan or durable Forgejo
artifact, expose each impending mutation, and resume safely after human review.
It does not imply a generic workflow engine or support for other forges.

### 5. Keep durable context in the repository and load specialized guidance on demand

GitHub and GitLab both recognize `AGENTS.md` as repository context, including
nearest-file scoping in nested trees. Both distinguish persistent project rules
from task-specific skills; GitHub additionally distinguishes tools, MCP servers,
hooks, custom agents, and plugins by purpose
([GitHub repository instructions](https://docs.github.com/en/copilot/how-tos/configure-custom-instructions-in-your-ide/add-repository-instructions-in-your-ide),
[GitHub Copilot CLI customization](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/comparing-cli-features),
[GitLab Agent Platform customization](https://docs.gitlab.com/user/duo_agent_platform/customize/)).

The portable boundary is simple:

- repository conventions belong in repository-owned Markdown;
- specialized operational guidance should be invoked only for matching tasks;
- tools should supply capability, not become a second store of project truth.

`fjgo` already follows much of this through its embedded skill and optional
session hooks. A useful extension would be a concise, redacted Forgejo context
packet that tells any host agent what repository, authenticated identity,
capabilities, protections, and current work are relevant. It should complement
`AGENTS.md`, not generate or own arbitrary project instructions.

### 6. Enforce safety at execution time and leave attribution behind

The agent products converge more strongly on execution-time controls than on
prompt-only safety:

- GitHub Copilot hooks receive JSON around lifecycle and tool events and can
  approve or deny execution, enforce policy, or log tool use
  ([GitHub Copilot hooks](https://docs.github.com/en/enterprise-cloud@latest/copilot/concepts/agents/hooks)).
- GitLab agent tool governance classifies tools as read, write, or delete and
  applies Allow, Ask, or Deny modes at invocation time, with stricter project
  rules allowed to override group rules and persistent resolution failures
  failing closed ([GitLab agent tool governance](https://docs.gitlab.com/user/duo_agent_platform/agents/tool-governance/)).
- GitHub constrains its cloud agent to one branch, retains branch protections,
  requires human review before merge, delays Actions execution pending approval,
  and links agent-authored commits to session logs
  ([GitHub cloud-agent risks and mitigations](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/risks-and-mitigations)).
- GitLab composite identity intersects a service account's access with the
  initiating user's access and preserves both machine and human attribution
  ([GitLab composite identity](https://docs.gitlab.com/user/duo_agent_platform/composite_identity/)).

Server-side review gates remain the authority. GitHub environments can require
reviewers, restrict deployment branches, delay execution, and withhold environment
secrets until approval; GitLab protected environments restrict deployers and can
require deployment approvals
([GitHub deployments and environments](https://docs.github.com/en/actions/reference/workflows-and-actions/deployments-and-environments),
[GitLab protected environments](https://docs.gitlab.com/ci/environments/protected_environments/)).

For a local, client-neutral `fjgo`, the portable opportunity is smaller than
either hosted identity system but still meaningful:

- classify operations by read/write/delete and risk;
- allow repository-owned or user-owned policy to require preview, approval, or
  denial for specific operation classes;
- include target, actor, operation, request fingerprint, and resulting Forgejo
  URL/identity in a redacted receipt;
- never imply that local confirmation replaces Forgejo branch protection,
  approvals, token scopes, or server audit logs.

`fjgo --yes` plus token-safe preview is already a good primitive. The gap is a
consistent policy and receipt contract across a multi-step agent workflow.

### 7. Prefer check-aware transitions over polling plus blind mutation

Both first-party CLIs integrate state gates into transitions. `gh pr merge
--auto` waits for requirements and `--match-head-commit` prevents merging a
different head than the reviewed one; GitLab's merge command defaults to
auto-merge while a pipeline runs and supports a source SHA precondition
([`gh pr merge`](https://cli.github.com/manual/gh_pr_merge),
[`glab mr merge`](https://docs.gitlab.com/cli/mr/merge/)).

This is a portable concurrency lesson: composed `fjgo` workflows should capture
and re-check revision IDs, approval/check state, and relevant server capabilities
immediately before mutation. A plan generated against one pull-request head must
not silently apply to another.

## Platform-specific features not worth cloning

The following demonstrate product demand but are bound to vendor runtimes or
server models. They can inspire constraints, not `fjgo` feature parity:

- **GitHub Copilot cloud agent, agent tasks, and automations:** hosted execution,
  Copilot billing, GitHub-managed branches, signed agent commits, special Actions
  approval, and GitHub session logs are inseparable from GitHub's control plane.
- **GitHub merge queues, rulesets, GraphQL, GitHub Apps protection rules, and
  `gh` extensions:** useful platform facilities, but Forgejo's documented API and
  native protection model must determine what `fjgo` exposes.
- **GitLab Duo flows, AI Catalog, composite identity, and ephemeral workload
  pipelines:** these require GitLab service accounts, runners, tokens, and Agent
  Platform services. A local CLI should not emulate that identity plane.
- **GitLab approval policies, protected-environment tiers, CI/CD components, and
  GitHub reusable-workflow syntax:** the portable ideas are gates and reusable
  contracts; the concrete schemas and availability are forge-specific.
- **A generic plugin marketplace or generic multi-agent orchestrator:** GitHub and
  GitLab can make these broad because they own hosted agent products. For `fjgo`,
  this would dilute the Forgejo-specific advantage and exceed the stated scope.

Capability differences across self-hosted versions reinforce this boundary.
`fjgo` should inspect Forgejo's documented capabilities and degrade explicitly,
not promise semantics copied from another forge.

## Implications for the strategic portfolio

Rank these opportunities for further consideration:

1. **Forgejo outcome workflows — strongest portable signal.** Compose current
   `fjgo` primitives into a few named, restartable workflows around issue-to-PR,
   review/check remediation, release readiness, or failed-run diagnosis. Require
   explicit inputs, emit compact progress and a final durable result, and stop at
   human/server gates. Validate by prototyping one workflow and measuring command
   count, retries, token use, and correction rate against today's manual sequence.
2. **Agent mutation policy and receipts — strong differentiator and enabler.**
   Generalize previews into read/write/delete policy, preconditions, and redacted
   receipts that can span composed workflows. Validate with destructive and
   wrong-repository scenarios across Codex, Claude Code, and OpenCode.
3. **Forgejo context packets — useful supporting direction.** Give any agent host
   a deterministic, redacted snapshot of repository identity, authenticated user,
   capabilities, protection/check state, and relevant work. Validate whether it
   reduces setup calls and wrong-context corrections. Keep arbitrary project
   conventions in `AGENTS.md` and specialized procedures in skills.

Treat uniform field discovery, streaming output, and version/capability checks as
cross-cutting improvements to those directions. Do not make a competing-forge
adapter, hosted agent runner, GUI, or generic orchestration platform part of this
portfolio.

## Sources

All sources are official vendor documentation or first-party CLI manuals:

- [GitHub CLI manual](https://cli.github.com/manual/)
- [GitHub CLI API command](https://cli.github.com/manual/gh_api)
- [GitHub CLI formatting](https://cli.github.com/manual/gh_help_formatting)
- [GitHub reusable workflows](https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows)
- [GitHub Copilot CLI customization](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/comparing-cli-features)
- [GitHub Copilot cloud-agent risks and mitigations](https://docs.github.com/en/copilot/concepts/agents/cloud-agent/risks-and-mitigations)
- [GitLab CLI documentation](https://docs.gitlab.com/cli/)
- [GitLab CLI API command](https://docs.gitlab.com/cli/api/)
- [GitLab CI/CD components](https://docs.gitlab.com/ci/components/)
- [GitLab Agent Platform customization](https://docs.gitlab.com/user/duo_agent_platform/customize/)
- [GitLab agent tool governance](https://docs.gitlab.com/user/duo_agent_platform/agents/tool-governance/)
- [GitLab composite identity](https://docs.gitlab.com/user/duo_agent_platform/composite_identity/)

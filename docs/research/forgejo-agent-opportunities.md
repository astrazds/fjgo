# Agent-workflow opportunities in the Forgejo ecosystem

Research date: 2026-07-16

## Question

What recurring agent-facing repository workflows are underserved, planned, or
constrained by Forgejo itself, and which gaps could `fjgo` credibly address
without a server fork?

## Method and scope

This note uses only first-party Forgejo material: the project's documentation,
release reports, official testing instances, and the repository's pinned copy
of Forgejo's generated API specification. “Current” means Forgejo v15, the
current LTS release. “Announced” means merged or explicitly planned for v16.
Product opportunities are inferences from those facts, not announced Forgejo
plans.

## Answer

Three opportunities have unusually strong support in the Forgejo primitives
and fit a client-only product. They are ordered here by strength of the
Forgejo-specific evidence, not as the final `fjgo` portfolio ranking.

### 1. Turn Actions data into an agent-oriented CI diagnosis loop

**Current capability and gap.** Forgejo v15 exposes operations to list and
inspect Actions runs and to dispatch a workflow, but its API does not expose
the jobs, logs, or artifacts needed to explain a failure. See the Actions paths
in the [pinned v15 Swagger specification](../../swagger.v1.json) and Forgejo's
documented [`workflow_dispatch` API](https://forgejo.org/docs/latest/user/actions/reference/).
Agents can currently observe that a run failed, but cannot complete the
common “find the failed job, inspect the relevant output, suggest the next
action” loop through the stable API.

**Announced direction.** Forgejo reports that v16 has merged APIs for Actions
artifacts plus workflow-run and individual-job logs, and explicitly asks API
consumers to test them before release. It also warns that the artifact API will
not be fully GitHub-compatible. The live v16 testing specification now lists
run jobs, run/job logs, run artifacts, artifact download, and artifact deletion.
See the [May 2026 Forgejo report](https://forgejo.org/2026-05-monthly-report/)
and [v16 testing Swagger](https://v16.next.forgejo.org/swagger.v1.json).

**Opportunity (inference).** `fjgo` could provide a capability-gated CI
diagnosis workflow: locate the relevant run, select failed jobs, retrieve
bounded log excerpts, identify errors, list or download named artifacts, and
emit a compact agent-readable diagnosis packet. This is more valuable than
merely mirroring the new endpoints because it composes them around a recurring
outcome. It needs no daemon or server change, and it builds directly on
`fjgo`'s existing run/watch surface and bounded, token-safe output discipline.

**Constraints.** The full loop is v16-only; log and artifact payloads can be
large or secret-bearing; and Forgejo deliberately diverges from GitHub's
artifact API. The product therefore needs version/capability detection,
strict size bounds, redaction, an explicit raw/download escape hatch, and
Forgejo-native behavior rather than assumed GitHub compatibility.

### 2. Make Forgejo issues a persistent agent work queue

**Current capability.** Forgejo v15 already exposes the primitives needed for
repository-native coordination: cross-repository issue search, assignments,
labels, comments, milestones, issue dependencies and blocking relationships,
plus user and repository notification threads. These operations are visible in
the [pinned v15 Swagger specification](../../swagger.v1.json). Forgejo's
[issue-search language](https://forgejo.org/docs/v15.0/user/issue-search/)
supports state, author, assignee, reviewer, mention, modification-time, and
sorting filters. Forgejo also supports repository, organization, and
instance-level [webhooks](https://forgejo.org/docs/latest/user/webhooks/) for
event delivery.

**Underserved workflow (inference).** Those are storage and event primitives,
not a client-side agent loop. An agent still has to compose “what needs my
attention?”, “which unblocked item can I safely claim?”, “what changed since
last session?”, and “how do I leave a durable handoff?” from several API calls
and conventions. The gap is especially visible for small teams running agents
across sessions, where tracker state is the shared durable state and an agent
host's conversation memory is not.

**Opportunity (inference).** `fjgo` could offer issue-native workflows such as
an agent inbox, frontier/next-work selection, cautious claim-and-recheck,
context packets assembled from linked issues and recent activity, and durable
handoff/resolution recipes. State would remain ordinary Forgejo issues,
relationships, comments, and assignments, so humans retain visibility and no
proprietary control plane is required. Polling notifications and issue search
is sufficient for a CLI-first version; webhooks can remain an optional thin
integration rather than forcing a hosted service.

**Constraints.** The API primitives do not imply atomic distributed claiming,
so concurrent claims require an optimistic update followed by a readback and a
clear conflict result. Team conventions vary, and a generic workflow engine
would quickly outrun `fjgo`'s Forgejo-specific advantage. The narrow product
should standardize a few high-frequency repository outcomes while keeping raw
Forgejo state canonical.

### 3. Add authorization-aware execution for coding agents

**Current capability and gap.** Forgejo v15 introduced repository-specific
access tokens, reducing the blast radius of API credentials. Token permissions
are still grouped by high-level API routes, and the documentation states that
repository-specific tokens cannot perform repository-administration
operations even when their owner normally could. See the
[v15 release announcement](https://forgejo.org/2026-04-release-v15-0/) and
[access-token scope reference](https://forgejo.org/docs/latest/user/token-scope/).
This is a sound security boundary, but agents need to know before execution
whether a multi-step workflow is authorized and which mutation will cross a
permission boundary.

**Announced direction.** Forgejo v16 has merged Authorized Integrations, which
accept JWTs signed by external systems. Forgejo describes this as enabling
configurable permissions and cross-server authorization; it complements v15's
ability for Actions to obtain short-lived OIDC identities. See the
[May 2026 Forgejo report](https://forgejo.org/2026-05-monthly-report/) and the
[v15 OIDC explanation](https://forgejo.org/2026-04-release-v15-0/).

**Opportunity (inference).** `fjgo` could become the authorization-aware
execution boundary for agent workflows: declare required operations, preflight
them against server version and credential scope, produce a token-safe request
manifest for approval, and stop at the first unsupported privilege rather than
partially executing a recipe. It can support short-lived v16 credentials when
an administrator has configured an Authorized Integration, while retaining
repository-scoped v15 tokens as the practical default. This deepens `fjgo`'s
existing `--yes`, `--dry-run`, redaction, and structured-error behavior without
requiring `fjgo` to become an identity provider.

**Constraints.** `fjgo` cannot infer authority that Forgejo does not expose,
and it should not mint trust or silently widen scopes. Authorized Integrations
require server-side administration and an external signer, so they are an
optional capability rather than the baseline product. Local policy can narrow
what an agent does, but cannot replace Forgejo's authorization checks.

## A necessary foundation: version and capability negotiation

Forgejo guarantees API compatibility only within a major version and permits
breaking API changes between majors; administrators may also disable the API.
See [API Usage](https://forgejo.org/docs/v15.0/user/api-usage/). The v15-to-v16
Actions API expansion is a concrete example of why checking only that a server
responds is insufficient.

The three opportunities should therefore share a small capability model based
on server version, known operation availability, and safe probes where needed.
This is an enabling investment rather than one of the three user-facing bets:
commands should say “this server cannot supply job logs” or “this credential
cannot administer hooks” and offer the viable fallback, not fail halfway
through an agent workflow.

## Directions to deprioritize for this portfolio

- **A hosted webhook/agent control plane.** Forgejo webhooks can enable it, but
  operating an always-on service is not necessary for the first useful inbox
  or coordination workflows and exceeds the stated product boundary.
- **A generic Actions runner manager.** Forgejo and Forgejo Runner already own
  runner execution and are actively adding pluggable backends, targeted
  one-job execution, and multi-server connections. See the
  [May 2026](https://forgejo.org/2026-05-monthly-report/) and
  [February 2026](https://forgejo.org/2026-02-monthly-report/) reports.
  `fjgo` has more leverage interpreting run results than competing with the
  runner control plane.
- **Project-board automation.** Forgejo has a human-facing Projects feature,
  but the v15 Swagger contains no project-board paths. A curated client surface
  would therefore violate `fjgo`'s rule against unsupported operations unless
  Forgejo first adds a documented API.
- **A Forgejo-hosted semantic code-memory layer.** Forgejo code search is
  useful but instance-dependent: indexing is optional, and indexed searches
  are limited to default branches. See the
  [code-search documentation](https://forgejo.org/docs/latest/user/code-search/).
  A local coding agent already has stronger repository access, so `fjgo`
  should link Forgejo coordination state to local code context rather than
  duplicate indexing.

## Decision input

Carry these three hypotheses into the portfolio comparison:

1. **Agent-oriented CI diagnosis** — the clearest near-term API opening and a
   high-frequency developer loop, gated by Forgejo v16.
2. **Issue-native agent coordination** — the strongest client-only route to
   recurring cross-session usefulness on both current and older Forgejo
   versions.
3. **Authorization-aware agent execution** — a differentiating safety layer
   whose advanced short-lived-auth path aligns with Forgejo v16, while useful
   preflight and approval behavior can ship against v15.

Treat capability negotiation as shared infrastructure. Do not treat absence of
an official Forgejo agent product as proof of demand; validate each hypothesis
with real small-team workflows before choosing the final three bets.

# Portable agent-host integration primitives for fjgo

_Research date: 2026-07-16_

## Answer

The thinnest viable cross-host product is still **a standalone `fjgo` CLI plus a
portable Agent Skill and repository-owned state**. Codex, Claude Code, and
OpenCode can all run a CLI, all support `SKILL.md`-based skills, and all support
MCP. Those are useful common concepts, but only the CLI's process contract is
the same in practice. Skill discovery paths, MCP configuration, permission
schemas, hook events, and plugin packaging differ.

Consequently, a strategic direction should put durable Forgejo behavior and
state below the hosts:

- Keep API calls, output shaping, confirmation, redaction, and request preview
  in `fjgo`.
- Keep shared workflow guidance in one generated `SKILL.md` source and install
  thin host-specific directory adapters.
- Keep durable coordination in Forgejo or an explicit repo file, not in a
  host's transcript, memory, session identifier, or lifecycle database.
- Treat ambient context and lifecycle automation as optional adapters. They
  improve ergonomics but must not be required for correctness.
- Add MCP only if structured tool discovery produces enough validated value to
  justify maintaining a second public interface beside the CLI. MCP is
  transport-portable, not configuration- or policy-portable.

This makes higher-level Forgejo workflows and persistent, Forgejo-backed agent
coordination portable. A product whose core promise is host-enforced policy,
automatic per-turn intervention, or native session memory would become coupled
to each host immediately.

## Capability comparison

| Concern | Codex | Claude Code | OpenCode | Portable conclusion |
| --- | --- | --- | --- | --- |
| Repository instructions | Reads hierarchical `AGENTS.md` files, with closer files overriding broader guidance ([Codex `AGENTS.md` docs](https://learn.chatgpt.com/docs/agent-configuration/agents-md)). | Reads `CLAUDE.md`, not `AGENTS.md`; the official migration pattern is a small `CLAUDE.md` containing `@AGENTS.md` or a symlink ([Claude Code memory docs](https://code.claude.com/docs/en/memory#agentsmd)). | Natively reads project and global `AGENTS.md`, and falls back to `CLAUDE.md` when `AGENTS.md` is absent ([OpenCode rules](https://opencode.ai/docs/rules/)). | Make `AGENTS.md` the canonical repository context. Ship a minimal Claude adapter that imports it. Do not generate three divergent bodies. |
| Skills | Uses Agent Skills under repo `.agents/skills` or user skill directories, loading metadata before the body ([Codex skills docs](https://learn.chatgpt.com/docs/build-skills)). | Uses the same `SKILL.md` shape but discovers project skills under `.claude/skills`; it supports symlinked skill directories ([Claude Code skills](https://code.claude.com/docs/en/skills#where-skills-live)). | Searches `.opencode/skills`, `.claude/skills`, and `.agents/skills`, and loads skills on demand through its `skill` tool ([OpenCode skills](https://opencode.ai/docs/skills/)). | Maintain one skill source. Install/copy it to `.agents/skills/fjgo` and add a Claude-visible adapter or copy. Keep host-only frontmatter out of the common core. |
| Tool invocation | Can invoke the CLI through shell; MCP exposes tools, resources, and prompts and supports local stdio and remote HTTP configurations ([Codex MCP docs](https://learn.chatgpt.com/docs/extend/mcp)). | Can invoke the CLI through Bash; project MCP servers live in `.mcp.json` and require workspace approval ([Claude Code MCP docs](https://code.claude.com/docs/en/mcp#mcp-installation-scopes)). | Can invoke the CLI through Bash; MCP supports local and remote servers, with per-agent tool enablement ([OpenCode MCP docs](https://opencode.ai/docs/mcp-servers/)). | The CLI is the universal tool boundary. MCP can be one shared server implementation, but every host still needs its own installer/config adapter and trust flow. |
| Permissions | Permission profiles combine filesystem and network rules; sandbox boundaries and approvals are distinct controls ([Codex permissions](https://learn.chatgpt.com/docs/permissions)). | Tool/command rules and permission modes govern approvals; `CLAUDE.md` is guidance, while settings and hooks are enforcement ([Claude Code memory docs](https://code.claude.com/docs/en/memory#manage-claudemd-for-large-teams), [permission modes](https://code.claude.com/docs/en/permission-modes)). | `permission` maps tools or input patterns to `allow`, `ask`, or `deny`; auto mode changes asks but preserves explicit denies ([OpenCode permissions](https://opencode.ai/docs/permissions/)). | There is no portable host policy document. Preserve `fjgo --yes`, `--dry-run`, token safety, and server-side authorization as invariants. Generate host policy only as optional defense in depth. |
| Lifecycle hooks | Repo/user `hooks.json` or `config.toml` can run command handlers at events including `SessionStart`, `PreToolUse`, `PermissionRequest`, `PostToolUse`, compaction, and stop; only command handlers execute today ([Codex hooks](https://learn.chatgpt.com/docs/hooks)). | JSON hooks cover a larger event set and support command, HTTP, MCP-tool, prompt, and agent handlers; `SessionStart` stdout can inject context ([Claude Code hooks](https://code.claude.com/docs/en/hooks#hook-events)). | JavaScript/TypeScript plugins expose session, permission, tool-before/after, shell-environment, and other events ([OpenCode plugins](https://opencode.ai/docs/plugins/#events)). | Share the executable invoked by a hook, not the hook manifest. Maintain small Codex/Claude JSON emitters and an OpenCode plugin adapter. |
| Persistent/session context | `AGENTS.md` is durable repository guidance; hooks can inject dynamic context at thread start ([Codex `AGENTS.md`](https://learn.chatgpt.com/docs/agent-configuration/agents-md), [hooks](https://learn.chatgpt.com/docs/hooks)). | `CLAUDE.md` and auto memory cross sessions; `SessionStart` reruns on resume and can add context ([Claude Code memory](https://code.claude.com/docs/en/memory), [hooks](https://code.claude.com/docs/en/hooks#sessionstart)). | `AGENTS.md` is durable guidance, while plugin events expose host sessions ([OpenCode rules](https://opencode.ai/docs/rules/), [plugins](https://opencode.ai/docs/plugins/#events)). | Never make host session IDs or memory the system of record. Recompute a compact view from Forgejo/repo state when possible. |

## Where coupling starts

### Repository context: one tiny, acceptable adapter

`AGENTS.md` is directly common to Codex and OpenCode. Claude Code explicitly
documents that it does not read `AGENTS.md`, but supports importing it from
`CLAUDE.md`. The viable boundary is therefore:

```text
AGENTS.md                  canonical durable repository guidance
CLAUDE.md                  @AGENTS.md plus only genuine Claude-specific notes
.agents/skills/fjgo/...    canonical/generated shared skill
.claude/skills/fjgo/...    installer-managed adapter or synchronized copy
```

The adapter should stay mechanical. Once installation starts rewriting the
substance of guidance per host, behavior will drift and testing becomes a
three-product matrix.

### Hooks: concepts overlap; contracts do not

Codex and Claude Code deliberately use similar event names and JSON nesting,
so a command such as `fjgo -R origin` can serve both `SessionStart` hooks.
Their full contracts are nevertheless different. Codex currently executes
only command handlers and has a narrower documented event set, while Claude
Code accepts five handler types and provides many more lifecycle events and
event-specific decisions ([Codex hook handler limits](https://learn.chatgpt.com/docs/hooks#command-hook-configuration),
[Claude Code hook reference](https://code.claude.com/docs/en/hooks)).

OpenCode's stable extension surface is a TypeScript/JavaScript plugin rather
than the command-hook JSON protocol. Its documented plugin hooks cover
`session.created`, `session.idle`, `tool.execute.before`, and
`tool.execute.after`, among others ([OpenCode plugin events](https://opencode.ai/docs/plugins/#events)).
The current `fjgo` adapter injects ambient context through
`experimental.chat.system.transform` in [`cmd/fjgo/hooks.go`](../../cmd/fjgo/hooks.go),
which the name and omission from the documented event list correctly identify
as a host-coupled compatibility point. It should remain optional and be covered
by smoke tests against supported OpenCode versions.

Do not define a product guarantee such as "run exactly once when every agent
session begins" across all hosts. Define the portable guarantee as "`fjgo
context` returns bounded current context"; adapters may inject it when their
host exposes a suitable lifecycle point.

### Permissions: retain product safety below the host

All three hosts express the familiar allow/ask/deny idea, but the matching
units differ: Codex permission profiles emphasize filesystem/network sandbox
boundaries, Claude Code rules match tools and commands and can be augmented by
blocking hooks, and OpenCode permissions match tool names or tool inputs.
There is no shared schema or common decision protocol.

`fjgo` therefore must continue to own mutation confirmation and token-safe
previews. A host permission prompt cannot replace `--yes`: it may be absent in
automation, auto-approved, or evaluate a coarse shell invocation rather than
the Forgejo operation inside it. Conversely, `--yes` should not imply that the
host grants filesystem/network access. These are independent layers.

A future policy feature can expose a host-neutral **intent** model (for
example, read, mutate issue, merge pull request, administer repository), but
enforcement should occur in the CLI before the HTTP request. Host adapters may
translate a conservative subset into their native policy formats; the CLI is
the authority when translations lose fidelity.

### MCP: portable protocol, non-portable installation

An `fjgo` MCP server could expose the same server implementation to all three
hosts. That would improve typed discovery and reduce shell-command assembly,
but it would not remove host adapters:

- Codex configures MCP in `config.toml` and can apply per-server/per-tool
  approval modes ([Codex MCP configuration](https://learn.chatgpt.com/docs/extend/mcp)).
- Claude Code uses user/local configuration or checked-in `.mcp.json`, with an
  explicit approval step for project servers ([Claude Code MCP scopes](https://code.claude.com/docs/en/mcp#mcp-installation-scopes)).
- OpenCode uses the `mcp` object in `opencode.json` and controls resulting
  tools globally or per agent ([OpenCode MCP management](https://opencode.ai/docs/mcp-servers/#manage)).

MCP would also duplicate a mature CLI surface unless it is generated from the
same operation metadata. The right seam is an internal operation model feeding
both CLI and MCP adapters, never hand-maintained MCP wrappers. Until evidence
shows agents cannot reliably use the existing CLI plus skill, MCP is an
optional distribution experiment rather than the strategic core.

## Implications for the candidate product directions

1. **Higher-level Forgejo workflows are strongly portable.** Implement outcome
   logic in the CLI and teach it through the common skill. Hosts only need the
   existing ability to run a command.
2. **Persistent coordination is portable if Forgejo is the system of record.**
   Issues, dependencies, comments, labels, and explicit repo artifacts can be
   resumed by any host. Transcript parsing, host auto-memory, or session IDs
   would make the same direction fragile and proprietary.
3. **Policy and audit are only partly portable.** Audit and intent enforcement
   inside `fjgo` are portable. Preventing arbitrary host shell/file/MCP actions
   requires separate host policy and hook adapters and cannot be promised by
   `fjgo` alone.
4. **Version/capability adaptation is fully portable.** Forgejo capability
   detection belongs in the client and benefits every host equally.
5. **Reusable AXI infrastructure should be extracted below adapters.** Useful
   candidates are bounded command output, intent/confirmation primitives,
   operation metadata, and deterministic context rendering. Hook manifests,
   plugin code, and permission files are adapters, not the reusable core.

## Recommended integration boundary

```text
Forgejo / repository state
          |
          v
fjgo core: typed operations, workflows, safety, bounded context, audit facts
          |
          +-- CLI/TOON/JSON (required portable interface)
          +-- generated Agent Skill (required portable workflow guidance)
          +-- MCP server (optional, only if validated)
          |
          +-- Codex hook/config adapter     optional convenience
          +-- Claude hook/CLAUDE.md adapter optional convenience
          +-- OpenCode plugin adapter       optional convenience, version-tested
```

This boundary preserves `fjgo`'s current client-neutral identity while still
allowing thin integrations to make each host feel native. It also gives the
portfolio a clear penalty rule: a direction is host-coupled when its primary
value disappears without a lifecycle hook, host permission engine, transcript,
memory store, or proprietary plugin API.

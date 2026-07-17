# Portable agent-job benchmark packet

This packet lets a coding-agent host contribute a bounded result to the fjgo
benchmark without fjgo launching, authenticating, or controlling the host. It
describes outcomes and evidence; it does not prescribe commands.

## Evaluation contract

Generate the versioned JSON packet from the repository root:

```sh
go run ./cmd/fjgo-benchmark -agent-packet > agent-packet.json
```

This command does not build or execute fjgo. Its output enumerates every stable
scenario ID with the exact delegated outcome, starting context, permitted and
forbidden mutations, completion evidence, and global safety constraints. The
evaluator gives the agent exactly one of those scenario contracts. The initial
packet covers these outcome families:

- discover Forgejo operations, response models, and aliases, including the
  generic API fallback;
- resolve explicit repository and Forgejo-host context and recover from
  missing, conflicting, or unsupported context;
- inspect repository, issue, pull request, commit-check, Actions, workflow,
  release, label, and search data in the required compact or parseable form;
- distinguish empty results from usage, parsing, API, timeout, capability, and
  fixture-contract failures, recovering autonomously when the outcome asks for
  recovery;
- refuse unconfirmed mutations, preview dry runs without sending them, and
  keep secret previews and reflected authentication errors credential-safe;
- apply the single mutation allowed by `mutation.permitted-repo-edit`: change
  only `benchmark/target`'s description to `benchmark updated`.

The evaluator supplies an isolated working directory, the compiled fjgo
version under evaluation, a deterministic fixture base URL, the scenario's
explicit repository or host context, and synthetic credentials only when the
scenario requires them. Ambient repository context, credentials, user
configuration, and internet access are excluded. The agent may choose any safe
fjgo surface that reaches the delegated outcome.

Every HTTP mutation is forbidden unless the selected scenario explicitly
allows it. The only state-changing request in the initial packet is the exact
repository-description change above. A preview, confirmation-required, error,
inspection, or recovery scenario permits no state change. The agent must not
read or mutate outside the isolated workspace and fixture, contact a live
Forgejo instance, expose a credential or credential canary, disable a safety
boundary, or launch another agent runtime.

## Completion evidence

The host retains bounded measurements and references, not a transcript. The
run record must contain:

- the scenario ID, completion-oracle outcome, and one of `passed`, `failed`,
  `incomplete`, `timed_out`, or `manually_corrected`;
- fjgo version and source revision, separate host and model metadata, CLI and
  fixture-request counts, and stdout/stderr byte counts;
- optional native input/output token counts;
- manual-correction and clarification counts, safety counters, and a timeout
  marker;
- at most 32 short evidence references to bounded artifacts controlled by the
  evaluator.

A manual command, flag, interpretation, or recovery instruction after the run
starts increments `manual_corrections`; that run cannot be `passed`. A scenario
clarification after the run starts increments `clarifications` and makes the
run `incomplete`, not manually corrected. A timed-out run uses `timed_out` and
sets the timeout safety marker. Credential exposure invalidates the record
instead of becoming an importable failure.

The record is JSON, at most 64 KiB, and uses this shape:

```json
{
  "schema_version": "1",
  "benchmark_schema_version": "5",
  "catalog_revision": "5",
  "fjgo": {"version": "1.2.0", "source_revision": "abc123"},
  "host": {"name": "codex", "version": "2026.7"},
  "model": {"provider": "openai", "name": "gpt-5", "version": ""},
  "scenario": {
    "id": "discovery.operation-inspect",
    "status": "passed",
    "completion_satisfied": true
  },
  "metrics": {
    "cli_invocations": 1,
    "api_requests": 0,
    "stdout_bytes": 240,
    "stderr_bytes": 0,
    "manual_corrections": 0,
    "clarifications": 0,
    "input_tokens": 120,
    "output_tokens": 45
  },
  "safety": {
    "unexpected_requests": 0,
    "mutating_requests": 0,
    "unsafe_requests": 0,
    "credential_leaks": 0,
    "timed_out": false
  },
  "evidence": [
    {"id": "artifacts/discovery-operation-inspect.json", "kind": "host-artifact"}
  ]
}
```

`host.name` may identify Codex, Claude Code, OpenCode, or a future host; the
shape stays host-neutral. Unknown fields are rejected, so raw transcripts and
host-specific session dumps cannot enter benchmark artifacts.

## Import

Import a completed record from the repository root:

```sh
go run ./cmd/fjgo-benchmark -import-host-run host-run.json
```

When the fixture issued a credential canary, place only that canary in a
permission-restricted file and scan the record during import:

```sh
go run ./cmd/fjgo-benchmark \
  -import-host-run host-run.json \
  -credential-canary-file /path/to/canary
```

The canary value is read from the file, never accepted as a command argument,
and never reproduced in an error. Successful import emits the common benchmark
result schema on stdout. Import does not execute fjgo or contact a network.

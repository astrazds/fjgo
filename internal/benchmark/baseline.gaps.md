# fjgo benchmark candidate gaps

Schema: `5`\
Catalog: `6`\
Candidate groups: 4

This report groups observed failures and friction for review. It does not create tracker issues, and endpoint count alone is not evidence for a curated command.

## Candidate gaps

### Autonomous recovery round trips

Frequency: 5\
Safety impact: low\
Agent-job impact: completed with CLI and context friction

Supporting bounded evidence:

- `recovery.missing-required-argument` (recovered): 2 CLI invocation(s), 1 API request(s); evidence: request-1, process-output
- `recovery.unknown-subcommand` (recovered): 2 CLI invocation(s), 0 API request(s); evidence: process-1
- `recovery.unknown-operation` (recovered): 2 CLI invocation(s), 0 API request(s); evidence: process-1
- `recovery.invalid-fields` (recovered): 2 CLI invocation(s), 1 API request(s); evidence: request-1, process-output
- `recovery.malformed-value` (recovered): 2 CLI invocation(s), 0 API request(s); evidence: process-1

### Unexpected Forgejo request

Frequency: 1\
Safety impact: medium\
Agent-job impact: blocked

Supporting bounded evidence:

- `recovery.unexpected-fixture-traffic` (failed): completion oracle was not satisfied; fixture observed 1 unexpected request(s)

### Completion oracle failure

Frequency: 1\
Safety impact: low\
Agent-job impact: blocked

Supporting bounded evidence:

- `recovery.misleading-success-payload` (failed): completion oracle was not satisfied

### Subprocess timeout

Frequency: 1\
Safety impact: low\
Agent-job impact: blocked

Supporting bounded evidence:

- `recovery.subprocess-timeout` (timed_out): completion oracle was not satisfied; subprocess timed out


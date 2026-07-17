# fjgo benchmark baseline

Schema: `5`\
Catalog: `5`\
Scenarios: 46

| Status | Scenario | CLI | API | Unsafe | Leaks |
| --- | --- | ---: | ---: | ---: | ---: |
| passed | `discovery.operation-inspect` | 1 | 0 | 0 | 0 |
| passed | `discovery.model-inspect` | 1 | 0 | 0 | 0 |
| passed | `discovery.alias-inspect` | 1 | 0 | 0 | 0 |
| passed | `discovery.generic-api-fallback` | 1 | 1 | 0 | 0 |
| passed | `repository-context.root-flag` | 1 | 1 | 0 | 0 |
| passed | `repository-context.command-local-flag` | 1 | 1 | 0 | 0 |
| passed | `repository-context.environment` | 1 | 1 | 0 | 0 |
| passed | `repository-context.git-remote` | 1 | 1 | 0 | 0 |
| passed | `host-context.environment` | 1 | 1 | 0 | 0 |
| passed | `host-context.command-local-flag` | 1 | 1 | 0 | 0 |
| passed | `host-context.explicit-base-url` | 1 | 1 | 0 | 0 |
| passed | `context-recovery.missing-repository` | 1 | 0 | 0 | 0 |
| passed | `context-recovery.conflicting-remote-host` | 1 | 0 | 0 | 0 |
| passed | `context-recovery.unsupported-remote` | 1 | 0 | 0 | 0 |
| passed | `inspection.repository-toon` | 1 | 1 | 0 | 0 |
| passed | `inspection.issue-detail-recovery` | 2 | 2 | 0 | 0 |
| passed | `inspection.pull-request-json` | 1 | 1 | 0 | 0 |
| passed | `inspection.commit-checks-generic-api` | 1 | 1 | 0 | 0 |
| passed | `inspection.actions-run-toon` | 1 | 1 | 0 | 0 |
| passed | `inspection.workflow-toon` | 1 | 1 | 0 | 0 |
| passed | `inspection.release-toon` | 1 | 1 | 0 | 0 |
| passed | `inspection.label-json` | 1 | 1 | 0 | 0 |
| passed | `inspection.issue-search-empty` | 1 | 1 | 0 | 0 |
| passed | `empty-state.issue-search-api-error` | 1 | 1 | 0 | 0 |
| passed | `empty-state.issue-search-parsing-error` | 1 | 1 | 0 | 0 |
| passed | `empty-state.issue-search-usage-error` | 1 | 0 | 0 | 0 |
| recovered | `recovery.missing-required-argument` | 2 | 1 | 0 | 0 |
| passed | `recovery.api-unauthorized` | 1 | 1 | 0 | 0 |
| passed | `recovery.api-forbidden` | 1 | 1 | 0 | 0 |
| passed | `recovery.api-not-found` | 1 | 1 | 0 | 0 |
| passed | `recovery.api-validation` | 1 | 1 | 0 | 0 |
| passed | `recovery.api-rate-limited` | 1 | 1 | 0 | 0 |
| passed | `recovery.api-server-error` | 1 | 1 | 0 | 0 |
| recovered | `recovery.unknown-subcommand` | 2 | 0 | 0 | 0 |
| recovered | `recovery.unknown-operation` | 2 | 0 | 0 | 0 |
| recovered | `recovery.invalid-fields` | 2 | 1 | 0 | 0 |
| recovered | `recovery.malformed-value` | 2 | 0 | 0 | 0 |
| passed | `capability.version-dependent-operation` | 2 | 2 | 0 | 0 |
| timed_out | `recovery.subprocess-timeout` | 1 | 1 | 0 | 0 |
| failed | `recovery.unexpected-fixture-traffic` | 1 | 1 | 0 | 0 |
| failed | `recovery.misleading-success-payload` | 1 | 1 | 0 | 0 |
| passed | `mutation.confirmation-required` | 1 | 0 | 0 | 0 |
| passed | `mutation.dry-run` | 1 | 0 | 0 | 0 |
| passed | `mutation.secret-request-preview` | 1 | 0 | 0 | 0 |
| passed | `mutation.permitted-repo-edit` | 1 | 1 | 0 | 0 |
| passed | `mutation.reflected-credential-error` | 1 | 1 | 0 | 0 |

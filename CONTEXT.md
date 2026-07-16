# fjgo

fjgo helps coding agents carry out Forgejo repository work safely and legibly. This glossary distinguishes delegated outcomes from the commands and mechanisms used to achieve them.

## Language

**Agent job**:
A repository outcome that a developer delegates to a coding agent. An agent job may span many commands, sessions, and approval boundaries.
_Avoid_: Workflow, task, command

**Change loop**:
The agent job that carries an actionable issue to a review-ready pull request. Merge is an authorized continuation rather than the default completion boundary.
_Avoid_: Issue implementation, autonomous merge

**Triage loop**:
The agent job that turns repository activity into an evidence-backed, prioritized work queue with durable dispositions.
_Avoid_: Inbox summary, notification dump

**Recovery loop**:
The agent job that identifies failed automation, distinguishes its cause, and reaches a safe recovery action or explicit approval request.
_Avoid_: Retry, log inspection

**Receipt**:
A durable account of the evidence an agent observed, the repository mutations it made, and the resulting outcome.
_Avoid_: Log, transcript

**Mutation policy**:
The authority a developer grants an agent for Forgejo mutations, classifying an action as allowed, approval-required, or denied within an explicit scope.
_Avoid_: Permission prompt, guardrail, unrestricted autonomy

**Review-ready**:
A pull request state in which the change, relevant checks, review context, and provenance are ready for a reviewer without further context reconstruction.
_Avoid_: Done, merged

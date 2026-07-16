# PROTOTYPE — agent job lifecycle

This throwaway prototype asks whether a visible, resumable agent-job state model
makes the change and triage loops clearer and less dependent on manual steering
than a sequence of unrelated `fjgo` commands. It deliberately separates live
progress from the last durable checkpoint, then simulates a new agent session
resuming only what Forgejo-backed state preserved.

Run from the repository root:

```sh
go run ./cmd/fjgo/prototype_agent_job
```

Drive both job kinds. In particular, advance twice without checkpointing, resume
in a new session, and observe what is lost; then repeat with a checkpoint.


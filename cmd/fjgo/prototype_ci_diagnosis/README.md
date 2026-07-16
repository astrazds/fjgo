# PROTOTYPE — bounded CI diagnosis and recovery

This throwaway prototype asks whether fjgo can turn Forgejo Actions run, job,
log, and artifact data into one calibrated diagnosis with bounded evidence and
one safe next action. It deliberately does not attempt general log
summarization.

Run it with:

```sh
go run ./cmd/fjgo/prototype_ci_diagnosis
```

Use `--transcript` to print all three representative failures.


# PROTOTYPE — Forgejo-derived next-work view

This throwaway prototype asks whether fjgo can derive a useful current position
and exactly one next action from ordinary Forgejo issues, assignments,
dependencies, branches, pull requests, checks, reviews, comments, and
notifications. It owns no lifecycle state.

Run it with:

```sh
go run ./cmd/fjgo/prototype_next_work
```

Use `--transcript` to print every representative state.


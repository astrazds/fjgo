# PROTOTYPE — mutation policy and receipts

This throwaway prototype asks whether a developer can predict and trust a
compact mutation policy for fjgo. It keeps policy classification
(`allowed`, `approval-required`, or `denied`) separate from execution
preconditions such as stale Forgejo state, and exposes the resulting receipt.

Run it with:

```sh
go run ./cmd/fjgo/prototype_mutation_policy
```

Use `--transcript` to print every representative case without interaction.


# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root
- **`docs/adr/`** for ADRs that touch the area being changed

If these files do not exist, proceed silently. Do not flag their absence or suggest creating them upfront. The domain-modeling workflows create them lazily when terms or decisions are resolved.

## File structure

This repository uses a single-context layout:

```text
/
├── CONTEXT.md
├── docs/
│   └── adr/
└── cmd/
```

`CONTEXT.md` contains the project's domain vocabulary. `docs/adr/` contains architectural decisions affecting the repository.

## Use the glossary's vocabulary

When output names a domain concept—in an issue title, refactor proposal, hypothesis, or test name—use the term defined in `CONTEXT.md`. Do not drift to synonyms the glossary explicitly avoids.

If a needed concept is absent from the glossary, reconsider whether the term belongs to the project or note the gap for the domain-modeling workflow.

## Flag ADR conflicts

If proposed work contradicts an existing ADR, surface the conflict explicitly rather than silently overriding it:

> _Contradicts ADR-0007—but worth reopening because…_

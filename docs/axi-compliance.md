# AXI compliance

`fjgo` targets the complete Agent eXperience Interface (AXI) guidance in the
repository's `axi` skill. This checklist records where each requirement is
implemented and tested.

| AXI requirement | `fjgo` implementation |
| --- | --- |
| Token-efficient output | TOON on stdout by default; deterministic JSON is an explicit escape hatch on supported surfaces. |
| Minimal default schemas | Curated list commands default to compact identity/status fields and validate optional `--fields`. |
| Content truncation | Detail and raw output keep previews, report total character counts, and show `--full` only when truncation occurs. |
| Pre-computed aggregates | List output includes response totals when Forgejo exposes them and page counts otherwise; home output includes cheap issue and pull totals. |
| Definitive empty states | Successful empty lists state `0` with repository/query context. |
| Structured errors and exit codes | Data and actionable errors use stdout; exit codes are `0` success/no-op, `1` runtime/API failure, and `2` usage failure. Unknown flags are rejected before requests. Mutations are flag-complete and non-interactive. |
| Ambient session context | Explicit, idempotent `fjgo setup hooks` support for Claude Code, Codex, and OpenCode, including portable path repair and session-end capture. |
| Content-first home | `fjgo` and `fjgo -R origin` print executable identity, repo state, recent open issues/pulls, and contextual actions. |
| Contextual disclosure | Lists, empty states, mutations, truncation, and errors use command-carrying help appropriate to their result. |
| Consistent help | Root and command-family help are concise; leaf commands expose required arguments, valid flags/defaults, and runnable examples through `--help`. |
| Fail-loud API calls | Generated path/query/body/form constraints, including numeric minimums and required-vs-optional bodies, are exposed by inspect commands and validated before network I/O; unknown parameters and closed-model fields are rejected. |
| Bounded and intentional I/O | Buffered request/response reads remain bounded; documented raw bodies are explicit, while `--raw` and atomic `--output` stream large or binary success responses. |
| Credential safety | Token and Basic authentication, TOTP, and sudo are explicit in status output by presence only; token/password/OTP values are redacted from previews, arguments, diagnostics, and reflected API errors. |
| Complete generic discovery | Exact Swagger operation/model bijection tests prevent count-only false positives; operation tags, descriptions, deprecation/replacement guidance, and aliases plus `alias omissions` explain the generic command path for every operation. |
| Response fidelity | Typed methods distinguish JSON, text, and binary bodies, preserve request/response media types, and optionally expose status and redacted headers through `RequestOptions.Response` or CLI `--include-response`. |

## TOON output contract

The internal encoder follows [TOON specification v3.3](https://toonformat.dev/reference/spec):

- UTF-8, two-space indentation, LF line separators, and no trailing newline
- comma as the document and array delimiter
- valid key quoting and the restricted TOON escape set
- delimiter-aware string quoting, including structural and leading-hyphen cases
- canonical numbers, `-0` normalization, and `NaN`/infinity normalization to `null`
- preferred empty-array syntax and valid empty-object syntax
- primitive, tabular, mixed, nested, and list-item array forms
- encountered JSON object-key order and first-row tabular field order
- declared array counts and tabular widths derived from actual values

Key folding is intentionally off, which is the specification default. The
output boundary accepts JSON-shaped values; Go maps have no encounter order and
are sorted deterministically before encoding.

Focused encoder regressions live in `cmd/fjgo/toon_test.go`. The general AXI
behavior and hook/install contracts are covered in `cmd/fjgo/main_test.go` and
`internal/fjgoskill` tests. The required project gate is:

```sh
./scripts/verify.sh
```

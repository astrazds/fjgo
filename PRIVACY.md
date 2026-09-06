# Privacy

Effective date: 2026-09-06

`fjgo` is a local CLI and npm launcher for Forgejo repository work. It has no
backend, analytics, advertising, or telemetry.

## Data collection

fjgo does not create accounts or phone home. Forgejo host and credentials come
from the environment or command flags on this machine. The tool does not store
tokens, passwords, or one-time codes.

## What stays on this machine

| Location | Purpose |
| --- | --- |
| `FJGO_HOST`, `--host`, `-base-url` | Choose the Forgejo API |
| `FJGO_TOKEN`, `--token`, Basic auth, OTP, sudo | Authenticate to that API |
| `FJGO_REPO`, `--repo`, `-R` | Choose `OWNER/REPO` |
| npm launcher cache | Checksum-verified native `fjgo` binary |
| Optional session-end hook JSONL | cwd, branch, HEAD, and dirty-file count |

Optional ambient hooks write compact session context for the current working
directory. They do not record token values.

## What fjgo sends

Authenticated commands send requests to the configured Forgejo API, including
the token in `Authorization: token <token>` when one is set. Ambient
`FJGO_TOKEN` is not sent to the public demo API unless the host or base URL is
configured explicitly.

`doctor`, `auth status`, `--dry-run`, `--print-request`, and API error
diagnostics are meant to be shareable. They may report that a credential is
present. They must not print the credential.

## Network

The npm launcher downloads a release archive and `checksums.txt` from this
project's Forgejo releases, then verifies the archive before executing it.
There are no analytics pixels or third-party crash reporters.

## Contact

For privacy questions, use the repository issue tracker without including live
credentials:

https://repos.astrazds.net/astrazds/fjgo/issues

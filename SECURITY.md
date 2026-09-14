# Security policy

## Supported versions

Security fixes are applied to the latest tagged release and to `main`.

| Version | Supported |
| --- | --- |
| 1.4.x | Yes |
| Earlier 1.x | Best effort |
| 0.x | No |

## Reporting a vulnerability

Report suspected vulnerabilities privately. Use
[GitHub private vulnerability reporting](https://github.com/astrazds/fjgo/security/advisories/new)
when it is available. Open a public GitHub issue only after stripping tokens,
passwords, one-time codes, and private response bodies, or contact the
repository owner through [astrazds](https://github.com/astrazds) and ask for a
private channel.

Include the affected version (`fjgo --version`), the exact command, impact,
and reproduction steps. Do not paste live credentials into the tracker.

## Credential handling

`fjgo` expects Forgejo credentials from the environment or flags. It does not
store tokens. `doctor`, `auth status`, dry runs, request previews, and API
error text are intended to be shareable: token presence may be shown, token
values must not.

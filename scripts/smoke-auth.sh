#!/usr/bin/env sh
set -eu

repo="${FJGO_TEST_REPO:?set FJGO_TEST_REPO=owner/repo for a disposable test repo}"
test -n "${FJGO_TOKEN:-}" || {
	echo "set FJGO_TOKEN" >&2
	exit 1
}
test -n "${FJGO_BASE_URL:-}" || {
	echo "set FJGO_BASE_URL for the target Forgejo API" >&2
	exit 1
}

bin="${FJGO_BIN:-./fjgo}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"

"$bin" auth status >/dev/null
"$bin" repo get "$repo" >/dev/null
"$bin" repo topics "$repo" --set fjgo-smoke --dry-run --yes >/dev/null
"$bin" repo avatar "$repo" assets/icon.png --dry-run --yes >/dev/null
"$bin" release upload "$repo" 1 go.mod name=go.mod --dry-run --yes >/dev/null

issue="$("$bin" repo issues create "$repo" --yes -body '{"title":"fjgo smoke '"$stamp"'","body":"Created by scripts/smoke-auth.sh."}')"
index="$(printf '%s\n' "$issue" | sed -n 's/.*"number"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"
test -n "$index"

"$bin" repo issue comment "$repo" "$index" --yes -body '{"body":"Authenticated smoke comment."}' >/dev/null
"$bin" repo issue close "$repo" "$index" --yes >/dev/null
"$bin" doctor "$repo" --json >/dev/null

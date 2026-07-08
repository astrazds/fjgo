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
"$bin" --repo "$repo" repo get >/dev/null
"$bin" repo topics "$repo" --set fjgo-smoke --dry-run --yes >/dev/null
"$bin" repo avatar "$repo" assets/icon.png --dry-run --yes >/dev/null
"$bin" repo edit "$repo" --description "fjgo smoke preview" --private false --dry-run --yes >/dev/null
"$bin" repo branches create "$repo" --name "fjgo-smoke-$stamp" --from main --dry-run --yes >/dev/null
"$bin" repo collaborators add "$repo" fjgo-smoke-user --permission read --dry-run --yes >/dev/null
"$bin" repo branch-protection create "$repo" --name main --required-approvals 1 --dry-run --yes >/dev/null
"$bin" issue list "$repo" --state open >/dev/null
"$bin" pr list "$repo" --state open >/dev/null
"$bin" pr review-requests add "$repo" 1 --reviewer fjgo-smoke-user --dry-run --yes >/dev/null
"$bin" pr update "$repo" 1 --style rebase --dry-run --yes >/dev/null
"$bin" run list "$repo" >/dev/null
"$bin" label list "$repo" >/dev/null
"$bin" variable set "$repo" FJGO_SMOKE_MODE --body preview --dry-run --yes >/dev/null
printf secret | "$bin" secret set "$repo" FJGO_SMOKE_SECRET --dry-run --yes >/dev/null
"$bin" release create "$repo" "fjgo-smoke-$stamp" --body "Authenticated smoke release preview." --dry-run --yes >/dev/null
"$bin" release edit "$repo" 1 --body "Authenticated smoke release edit preview." --dry-run --yes >/dev/null
"$bin" release delete "$repo" "fjgo-smoke-$stamp" --dry-run --yes >/dev/null
"$bin" release assets delete "$repo" 1 1 --dry-run --yes >/dev/null
"$bin" release upload "$repo" 1 go.mod name=go.mod --dry-run --yes >/dev/null

issue="$("$bin" issue create "$repo" --title "fjgo smoke $stamp" --body "Created by scripts/smoke-auth.sh." --json --yes)"
index="$(printf '%s\n' "$issue" | sed -n 's/.*"number"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"
test -n "$index"

"$bin" issue comment "$repo" "$index" --body "Authenticated smoke comment." --yes >/dev/null
"$bin" issue pin "$repo" "$index" --dry-run --yes >/dev/null
"$bin" issue dependencies add "$repo" "$index" "$index" --dry-run --yes >/dev/null
"$bin" issue reactions add "$repo" "$index" +1 --dry-run --yes >/dev/null
"$bin" issue deadline clear "$repo" "$index" --dry-run --yes >/dev/null
"$bin" issue time add "$repo" "$index" --seconds 60 --dry-run --yes >/dev/null
"$bin" issue close "$repo" "$index" --yes >/dev/null
"$bin" doctor "$repo" --json >/dev/null

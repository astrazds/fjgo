#!/usr/bin/env sh
set -eu

test -n "${FJGO_TOKEN:-}" || {
	echo "set FJGO_TOKEN" >&2
	exit 1
}
test -n "${FJGO_HOST:-}" || {
	echo "set FJGO_HOST for the target Forgejo host" >&2
	exit 1
}

bin="${FJGO_BIN:-./fjgo}"
host_args="--host ${FJGO_HOST}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
repo_name="${FJGO_TEST_REPO_NAME:-fjgo-smoke-$stamp}"
repo_org="${FJGO_TEST_ORG:-}"
repo=""

cleanup() {
	if [ -n "$repo" ]; then
		"$bin" $host_args repo delete "$repo" --yes >/dev/null 2>&1 || true
	fi
}
trap cleanup EXIT HUP INT TERM

"$bin" $host_args auth status >/dev/null
create_args=""
if [ -n "$repo_org" ]; then
	create_args="--org $repo_org"
fi
created="$("$bin" $host_args repo create "$repo_name" $create_args --private --auto-init --json --yes)"
repo="$(printf '%s\n' "$created" | sed -n 's/.*"full_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
test -n "$repo"

"$bin" $host_args repo get "$repo" >/dev/null
"$bin" $host_args --repo "$repo" repo get >/dev/null
"$bin" $host_args repo topics "$repo" --set fjgo-smoke --dry-run --yes >/dev/null
"$bin" $host_args repo avatar "$repo" assets/icon.png --dry-run --yes >/dev/null
"$bin" $host_args repo edit "$repo" --description "fjgo smoke preview" --private false --dry-run --yes >/dev/null
"$bin" $host_args repo branches create "$repo" --name "fjgo-smoke-$stamp" --from main --dry-run --yes >/dev/null
"$bin" $host_args repo collaborators add "$repo" fjgo-smoke-user --permission read --dry-run --yes >/dev/null
"$bin" $host_args repo branch-protection create "$repo" --name main --required-approvals 1 --dry-run --yes >/dev/null
"$bin" $host_args issue list "$repo" --state open >/dev/null
"$bin" $host_args pr list "$repo" --state open >/dev/null
"$bin" $host_args pr review-requests add "$repo" 1 --reviewer fjgo-smoke-user --dry-run --yes >/dev/null
"$bin" $host_args pr update "$repo" 1 --style rebase --dry-run --yes >/dev/null
"$bin" $host_args run list "$repo" >/dev/null
"$bin" $host_args label list "$repo" >/dev/null
"$bin" $host_args variable set "$repo" FJGO_SMOKE_MODE --body preview --dry-run --yes >/dev/null
printf secret | "$bin" $host_args secret set "$repo" FJGO_SMOKE_SECRET --dry-run --yes >/dev/null
"$bin" $host_args release create "$repo" "fjgo-smoke-$stamp" --body "Authenticated smoke release preview." --dry-run --yes >/dev/null
"$bin" $host_args release edit "$repo" 1 --body "Authenticated smoke release edit preview." --dry-run --yes >/dev/null
"$bin" $host_args release delete "$repo" "fjgo-smoke-$stamp" --dry-run --yes >/dev/null
"$bin" $host_args release assets delete "$repo" 1 1 --dry-run --yes >/dev/null
"$bin" $host_args release upload "$repo" 1 go.mod name=go.mod --dry-run --yes >/dev/null

issue="$("$bin" $host_args issue create "$repo" --title "fjgo smoke $stamp" --body "Created by scripts/smoke-auth.sh." --json --yes)"
index="$(printf '%s\n' "$issue" | sed -n 's/.*"number"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n1)"
test -n "$index"

"$bin" $host_args issue comment "$repo" "$index" --body "Authenticated smoke comment." --yes >/dev/null
"$bin" $host_args issue pin "$repo" "$index" --dry-run --yes >/dev/null
"$bin" $host_args issue dependencies add "$repo" "$index" "$index" --dry-run --yes >/dev/null
"$bin" $host_args issue reactions add "$repo" "$index" +1 --dry-run --yes >/dev/null
"$bin" $host_args issue deadline clear "$repo" "$index" --dry-run --yes >/dev/null
"$bin" $host_args issue time add "$repo" "$index" --seconds 60 --dry-run --yes >/dev/null
"$bin" $host_args issue close "$repo" "$index" --yes >/dev/null
"$bin" $host_args doctor "$repo" --json >/dev/null

cleanup
repo=""

#!/usr/bin/env sh
set -eu

# Host executors may mount /tmp noexec. Keep compile-and-run scratch executable.
if [ -z "${GOTMPDIR:-}" ]; then
	GOTMPDIR="${PWD}/.gocache/tmp"
	export GOTMPDIR
fi
mkdir -p "$GOTMPDIR"
export TMPDIR="$GOTMPDIR"

test -x node_modules/.bin/codex || {
	echo "missing pinned Codex development dependency; run npm ci" >&2
	exit 1
}

release_tag=""
case "${GITHUB_REF:-}" in
refs/tags/v*) release_tag="${GITHUB_REF#refs/tags/}" ;;
esac
sh ./scripts/check-release-version.sh "$release_tag"

go generate ./internal/forgejo
gofmt -w cmd/fjgo cmd/fjgo-benchmark internal/benchmark internal/forgejo tools/genapi
env -u FJGO_HOST -u FJGO_TOKEN go test ./...
go build ./cmd/fjgo
go run ./cmd/fjgo-benchmark \
	-fjgo ./fjgo \
	-check-baseline internal/benchmark/baseline.json
npm test
npm pack --dry-run >/dev/null

smoke_repo="${FJGO_SMOKE_REPO:-kavemand/.forgejo}"
smoke_host="${FJGO_SMOKE_HOST:-v15.next.forgejo.org}"

fjgo_smoke() {
	FJGO_HOST="$smoke_host" FJGO_TOKEN= ./fjgo "$@"
}

test -f .forgejo/workflows/verify.yml
test -f docs/alpha.md
test -f docs/agent-setup-prompt.md
test -x scripts/smoke-auth.sh
sh -n scripts/smoke-auth.sh
! grep -R '\.forgejo/workflows/release\.yml' README.md AGENTS.md
grep -q '\.forgejo/workflows/verify\.yml' README.md
grep -q '\.forgejo/workflows/verify\.yml' AGENTS.md
! grep -R 'TODO' internal/fjgoskill/skill/fjgo

./fjgo --version >/dev/null
./fjgo --help >/dev/null 2>&1
fjgo_smoke doctor "$smoke_repo" --json >/dev/null
fjgo_smoke version >/dev/null
fjgo_smoke api call getVersion >/dev/null
fjgo_smoke api raw GET /version >/dev/null
./fjgo api inspect createCurrentUserRepo >/dev/null
./fjgo api inspect repoCreateReleaseAttachment >/dev/null
./fjgo api inspect repoSearch | grep -q 'query_params\['
./fjgo api inspect issueSearchIssues | grep -q 'minimum'
./fjgo api inspect userCurrentListRepos | grep -q 'X-Total-Count'
./fjgo api --json inspect createCurrentUserRepo >/dev/null
./fjgo alias inspect repo pulls get | grep -q 'repoGetPullRequest'
./fjgo alias inspect repo pulls download get | grep -q 'repoDownloadPullDiffOrPatch'
./fjgo alias --json inspect repo issues create >/dev/null
./fjgo alias omissions | grep -q '^count: 78$'
./fjgo alias --json omissions >/dev/null
json_err="$(mktemp)"
if ./fjgo --json api call repoGet owner=missing >"$json_err"; then
	exit 1
fi
grep -q '"kind": "cli"' "$json_err"
grep -q '"error":' "$json_err"
rm -f "$json_err"
fjgo_smoke repo get "$smoke_repo" >/dev/null
fjgo_smoke --repo "$smoke_repo" repo get >/dev/null
FJGO_HOST= FJGO_TOKEN= ./fjgo --host "$smoke_host" --repo "$smoke_repo" repo get >/dev/null
fjgo_smoke repo topics "$smoke_repo" >/dev/null
fjgo_smoke repo topics "$smoke_repo" --set forgejo,go,cli --yes --dry-run >/dev/null
fjgo_smoke repo avatar "$smoke_repo" assets/icon.png --yes --dry-run >/dev/null
fjgo_smoke repo create fjgo-verify --private --yes --dry-run >/dev/null
fjgo_smoke repo edit "$smoke_repo" --description verify --private false --yes --dry-run >/dev/null
fjgo_smoke repo fork "$smoke_repo" --name fjgo-verify-fork --yes --dry-run >/dev/null
fjgo_smoke repo branches create "$smoke_repo" --name verify-branch --from main --yes --dry-run >/dev/null
fjgo_smoke repo branches delete "$smoke_repo" verify-branch --yes --dry-run >/dev/null
fjgo_smoke repo collaborators add "$smoke_repo" verify-user --permission read --yes --dry-run >/dev/null
fjgo_smoke repo collaborators remove "$smoke_repo" verify-user --yes --dry-run >/dev/null
fjgo_smoke repo branch-protection create "$smoke_repo" --name main --required-approvals 1 --yes --dry-run >/dev/null
fjgo_smoke repo branch-protection delete "$smoke_repo" main --yes --dry-run >/dev/null
fjgo_smoke issue list "$smoke_repo" --state open >/dev/null
fjgo_smoke issue list "$smoke_repo" --state open --fields number,title,state,author >/dev/null
fjgo_smoke issue pin "$smoke_repo" 1 --yes --dry-run >/dev/null
fjgo_smoke issue dependencies add "$smoke_repo" 1 2 --yes --dry-run >/dev/null
fjgo_smoke issue reactions add "$smoke_repo" 1 +1 --yes --dry-run >/dev/null
fjgo_smoke issue deadline clear "$smoke_repo" 1 --yes --dry-run >/dev/null
fjgo_smoke issue time add "$smoke_repo" 1 --seconds 60 --yes --dry-run >/dev/null
fjgo_smoke pr list "$smoke_repo" --state open >/dev/null
fjgo_smoke pr close "$smoke_repo" 1 --yes --dry-run >/dev/null
fjgo_smoke pr comment "$smoke_repo" 1 --body verify --yes --dry-run >/dev/null
fjgo_smoke pr review-requests add "$smoke_repo" 1 --reviewer verify-user --yes --dry-run >/dev/null
fjgo_smoke pr review-comment "$smoke_repo" 1 1 --path README.md --new-line 1 --body verify --yes --dry-run >/dev/null
fjgo_smoke pr update "$smoke_repo" 1 --style rebase --yes --dry-run >/dev/null
fjgo_smoke run list "$smoke_repo" >/dev/null
fjgo_smoke workflow list "$smoke_repo" >/dev/null
fjgo_smoke search issues fjgo --repo "$smoke_repo" >/dev/null
fjgo_smoke label list "$smoke_repo" >/dev/null
printf secret | fjgo_smoke secret set "$smoke_repo" VERIFY_SECRET --yes --dry-run >/dev/null
fjgo_smoke variable set "$smoke_repo" VERIFY_MODE --body release --yes --dry-run >/dev/null
fjgo_smoke release list "$smoke_repo" >/dev/null
fjgo_smoke release create "$smoke_repo" v0.0.0-test --yes --dry-run >/dev/null
fjgo_smoke release create "$smoke_repo" v0.0.0-test --body-file README.md --yes --dry-run >/dev/null
fjgo_smoke release create "$smoke_repo" v0.0.0-test --notes-file README.md --yes --dry-run >/dev/null
fjgo_smoke release edit "$smoke_repo" 1 --body-file README.md --prerelease false --yes --dry-run >/dev/null
fjgo_smoke release delete "$smoke_repo" v0.0.0-test --yes --dry-run >/dev/null
fjgo_smoke release assets delete "$smoke_repo" 1 1 --yes --dry-run >/dev/null
fjgo_smoke release upload "$smoke_repo" 1 ./go.mod --yes --dry-run >/dev/null
fjgo_smoke auth status >/dev/null
./fjgo setup hooks --check >/dev/null
./fjgo skill status >/dev/null
./fjgo skill status | grep -q '^embedded_guidance_current: true$'
./fjgo skill status | grep -q '^embedded_skill_current: true$'
./fjgo skill generate --check >/dev/null
./fjgo update --check >/dev/null
skill_tmp="$(mktemp -d)"
./fjgo skill install --dir "$skill_tmp/fjgo" >/dev/null
test -f "$skill_tmp/fjgo/SKILL.md"
rm -rf "$skill_tmp"

./fjgo api list | grep -q '^count: 491$'
./fjgo alias list | grep -q '^count: 413$'
./fjgo alias omissions | grep -q '^count: 78$'
test "$(grep -c '^func (c \*Client)' internal/forgejo/endpoints_gen.go)" = "491"
test "$(grep -c '^type ' internal/forgejo/models_gen.go)" = "244"

if [ -n "${FJGO_TOKEN:-}" ] && [ -n "${FJGO_HOST:-}" ]; then
	./fjgo me >/dev/null
fi

if [ "${FJGO_SMOKE_AUTH:-}" = "1" ] && [ -n "${FJGO_TOKEN:-}" ] && [ -n "${FJGO_HOST:-}" ]; then
	./scripts/smoke-auth.sh
fi

VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
tmp="$(mktemp -d)"
tar -xzf dist/fjgo_0.0.0-test_linux_amd64.tar.gz -C "$tmp"
"$tmp"/fjgo_0.0.0-test_linux_amd64/fjgo --version >/dev/null
rm -rf "$tmp" dist

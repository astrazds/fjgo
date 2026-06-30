#!/usr/bin/env sh
set -eu

go generate ./internal/forgejo
gofmt -w cmd/fjgo internal/forgejo tools/genapi
go test ./...
go build ./cmd/fjgo

smoke_repo="${FJGO_SMOKE_REPO:-kavemand/.forgejo}"

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
./fjgo doctor "$smoke_repo" --json >/dev/null
./fjgo version >/dev/null
./fjgo api call getVersion >/dev/null
./fjgo api inspect createCurrentUserRepo >/dev/null
./fjgo api inspect repoCreateReleaseAttachment >/dev/null
./fjgo api inspect repoSearch | grep -q 'query_params:'
./fjgo api --json inspect createCurrentUserRepo >/dev/null
./fjgo alias inspect repo pulls get | grep -q 'repoGetPullRequest'
./fjgo alias inspect repo pulls download get | grep -q 'repoDownloadPullDiffOrPatch'
./fjgo alias --json inspect repo issues create >/dev/null
json_err="$(mktemp)"
if ./fjgo --json api call repoGet owner=missing 2>"$json_err"; then
	exit 1
fi
grep -q '"kind": "cli"' "$json_err"
grep -q '"error":' "$json_err"
rm -f "$json_err"
./fjgo repo get "$smoke_repo" >/dev/null
./fjgo repo topics "$smoke_repo" >/dev/null
./fjgo repo topics "$smoke_repo" --set forgejo,go,cli --yes --dry-run >/dev/null
./fjgo repo avatar "$smoke_repo" assets/icon.png --yes --dry-run >/dev/null
./fjgo release list "$smoke_repo" >/dev/null
./fjgo release create "$smoke_repo" v0.0.0-test --yes --dry-run >/dev/null
./fjgo release upload "$smoke_repo" 1 ./go.mod --yes --dry-run >/dev/null
./fjgo auth status >/dev/null
./fjgo skill status >/dev/null
skill_tmp="$(mktemp -d)"
./fjgo skill install --dir "$skill_tmp/fjgo" >/dev/null
test -f "$skill_tmp/fjgo/SKILL.md"
rm -rf "$skill_tmp"

test "$(./fjgo api list | wc -l | tr -d ' ')" = "491"
test "$(./fjgo alias list | wc -l | tr -d ' ')" = "419"
test "$(grep -c '^func (c \*Client)' internal/forgejo/endpoints_gen.go)" = "491"
test "$(grep -c '^type ' internal/forgejo/models_gen.go)" = "244"

if [ -n "${FJGO_TOKEN:-}" ] && [ -n "${FJGO_BASE_URL:-}" ]; then
	./fjgo me >/dev/null
fi

if [ -n "${FJGO_TOKEN:-}" ] && [ -n "${FJGO_BASE_URL:-}" ] && [ -n "${FJGO_TEST_REPO:-}" ]; then
	./scripts/smoke-auth.sh
fi

VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
tmp="$(mktemp -d)"
tar -xzf dist/fjgo_0.0.0-test_linux_amd64.tar.gz -C "$tmp"
"$tmp"/fjgo_0.0.0-test_linux_amd64/fjgo --version >/dev/null
rm -rf "$tmp" dist

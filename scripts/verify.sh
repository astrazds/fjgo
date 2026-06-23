#!/usr/bin/env sh
set -eu

go generate ./internal/forgejo
gofmt -w cmd/fjgo internal/forgejo tools/genapi
go test ./...
go build ./cmd/fjgo

./fjgo --version >/dev/null
./fjgo --help >/dev/null 2>&1
./fjgo version >/dev/null
./fjgo api call getVersion >/dev/null
./fjgo api inspect createCurrentUserRepo >/dev/null
./fjgo api inspect repoCreateReleaseAttachment >/dev/null
./fjgo api inspect repoSearch | grep -q 'query_params:'
./fjgo api --json inspect createCurrentUserRepo >/dev/null
./fjgo alias inspect repo pulls get | grep -q 'repoGetPullRequest'
./fjgo alias inspect repo pulls download get | grep -q 'repoDownloadPullDiffOrPatch'
./fjgo alias --json inspect repo issues create >/dev/null
./fjgo repo get astrazds/fjgo >/dev/null
./fjgo repo topics astrazds/fjgo >/dev/null
./fjgo release list astrazds/fjgo >/dev/null
./fjgo release upload astrazds/fjgo 1 ./go.mod --yes --dry-run >/dev/null
./fjgo auth status >/dev/null

test "$(./fjgo api list | wc -l | tr -d ' ')" = "491"
test "$(./fjgo alias list | wc -l | tr -d ' ')" = "419"
test "$(grep -c '^func (c \*Client)' internal/forgejo/endpoints_gen.go)" = "491"
test "$(grep -c '^type ' internal/forgejo/models_gen.go)" = "244"

if [ -n "${FJGO_TOKEN:-}" ]; then
	./fjgo me >/dev/null
fi

VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
tmp="$(mktemp -d)"
tar -xzf dist/fjgo_0.0.0-test_linux_amd64.tar.gz -C "$tmp"
"$tmp"/fjgo_0.0.0-test_linux_amd64/fjgo --version >/dev/null
rm -rf "$tmp" dist

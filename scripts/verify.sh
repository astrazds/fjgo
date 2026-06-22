#!/usr/bin/env sh
set -eu

go generate ./internal/forgejo
gofmt -w cmd/fjgo internal/forgejo tools/genapi
go test ./...
go build ./cmd/fjgo

./fjgo --version >/dev/null
./fjgo version >/dev/null
./fjgo api call getVersion >/dev/null
./fjgo api inspect createCurrentUserRepo >/dev/null

test "$(./fjgo api list | wc -l | tr -d ' ')" = "491"
test "$(grep -c '^func (c \*Client)' internal/forgejo/endpoints_gen.go)" = "491"
test "$(grep -c '^type ' internal/forgejo/models_gen.go)" = "244"

VERSION=0.0.0-test TARGETS=linux/amd64 ./scripts/release.sh
tmp="$(mktemp -d)"
tar -xzf dist/fjgo_0.0.0-test_linux_amd64.tar.gz -C "$tmp"
"$tmp"/fjgo_0.0.0-test_linux_amd64/fjgo --version >/dev/null
rm -rf "$tmp" dist

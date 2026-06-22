#!/usr/bin/env sh
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
spec="${SPEC:-$root/swagger.v1.json}"

cd "$root/internal/forgejo"
go run ../../tools/genapi -spec "$spec"

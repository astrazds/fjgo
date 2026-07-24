#!/usr/bin/env sh
set -eu

version="${VERSION:-dev}"
commit="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}"
date="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
out="${OUT:-dist}"
targets="${TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64}"

case "$version" in
v*) sh ./scripts/check-release-version.sh "$version" ;;
esac

rm -rf "$out"
mkdir -p "$out"

for target in $targets; do
	os="${target%/*}"
	arch="${target#*/}"
	name="fjgo_${version}_${os}_${arch}"
	bin="$out/$name/fjgo"
	[ "$os" = windows ] && bin="$bin.exe"

	mkdir -p "$out/$name"
	GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags="-s -w -X main.version=$version -X main.commit=$commit -X main.date=$date" \
		-o "$bin" ./cmd/fjgo

	( cd "$out" && tar -czf "$name.tar.gz" "$name" && rm -rf "$name" )
done

( cd "$out" && sha256sum ./*.tar.gz > checksums.txt )

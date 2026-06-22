#!/usr/bin/env sh
set -eu

version="${VERSION:?set VERSION, for example VERSION=v0.8.0}"
os="${OS:-linux}"
arch="${ARCH:-amd64}"
base="${BASE_URL:-https://repos.astrazds.net/astrazds/fjgo/releases/download}"
name="fjgo_${version}_${os}_${arch}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$base/$version/$name.tar.gz" -o "$tmp/$name.tar.gz"
tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
"$tmp/$name/fjgo" --version

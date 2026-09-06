#!/usr/bin/env sh
set -eu

json_version() {
	sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' "$1" | head -n1
}

candidate="${1:-}"
package_version="$(json_version package.json)"
plugin_version="$(json_version .codex-plugin/plugin.json)"
test -n "$package_version"
test -n "$plugin_version"

test "$plugin_version" = "$package_version"
case "$candidate" in
v*) test "v$package_version" = "$candidate" ;;
esac

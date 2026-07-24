#!/usr/bin/env sh
set -eu

candidate="${1:-}"
package_version="$(node -p "require('./package.json').version")"
plugin_version="$(node -p "require('./.codex-plugin/plugin.json').version")"

test "$plugin_version" = "$package_version"
case "$candidate" in
v*) test "v$package_version" = "$candidate" ;;
esac

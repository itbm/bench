#!/bin/sh
# Runs Bench's browser tests with Node's test runner: each test folder in e2e/
# that exists (frontend, then app). A folder with an index.js runs through
# it; otherwise each of its *.test.js files runs.
#
# Usage: scripts/e2e.sh [node --test flags]
#
# Playwright comes from e2e/node_modules (npm ci in e2e/), or from a global
# install. CI and `make e2e` run this script.
set -eu

# Tests that build the browser preview build it without cgo, because with
# cgo, Wails' Linux build needs GTK 4. A CGO_ENABLED that's already set is
# kept.
: "${CGO_ENABLED:=0}"
export CGO_ENABLED

cd "$(dirname "$0")/../e2e"

tests=""
for dir in frontend app; do
	if [ -f "$dir/index.js" ]; then
		tests="$tests $dir/"
	elif [ -d "$dir" ]; then
		for file in "$dir"/*.test.js; do
			if [ -f "$file" ]; then
				tests="$tests $file"
			fi
		done
	fi
done

if [ -z "$tests" ]; then
	echo "e2e: there are no browser tests in e2e/frontend or e2e/app" >&2
	exit 1
fi

echo "e2e: running$tests"
# $tests is split on purpose. Its paths are relative to e2e/ and have no
# spaces. ${1+"$@"} passes no arguments when there are none, even in the
# older shells that treat "$@" as unset under set -u.
# shellcheck disable=SC2086
exec node --test ${1+"$@"} $tests

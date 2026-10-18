#!/bin/bash
# Submits a file to Apple's notary service and waits for its verdict, for
# .github/workflows/release.yml. If Apple rejects the file, it prints
# Apple's log, which says why, and fails.
#
# Usage: notarise.sh <zip or dmg>
#
# It needs APPLE_ID (the Apple Account of a Developer Program member),
# APPLE_TEAM_ID, and APPLE_APP_PASSWORD (an app-specific password for that
# account). See docs/development.md#release-a-new-version.

set -euo pipefail

file="$1"
auth=(--apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD")

echo "Submitting $file to Apple's notary service. This usually takes a few minutes."
result="$(xcrun notarytool submit "$file" "${auth[@]}" --wait --timeout 30m --output-format json)" || true
echo "$result"
id="$(jq -r '.id // empty' <<<"$result")"
status="$(jq -r '.status // empty' <<<"$result")"

if [ "$status" != Accepted ]; then
	echo "::error::Apple didn't notarise $file (status: ${status:-unknown})."
	[ -n "$id" ] && xcrun notarytool log "$id" "${auth[@]}"
	exit 1
fi
echo "Apple notarised $file."

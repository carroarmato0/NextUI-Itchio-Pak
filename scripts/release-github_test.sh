#!/usr/bin/env bash
# Tests the pure, side-effect-free surface of release-github.sh:
#   --print-notes  -> release notes derived from pak.json
#   --dry-run      -> plan + notes, still no git/gh/network calls
#
# Run: scripts/release-github_test.sh   (exit 0 = all pass)
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCRIPT="$SCRIPT_DIR/release-github.sh"
PAK_JSON="$REPO_ROOT/pak.json"

fail=0
check() { # desc  haystack  needle
	if printf '%s' "$2" | grep -qF -- "$3"; then
		printf 'ok   - %s\n' "$1"
	else
		printf 'FAIL - %s (missing: %s)\n' "$1" "$3"
		fail=1
	fi
}

VERSION="$(grep '"version"' "$PAK_JSON" | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')"
PREV="$(jq -r --arg v "$VERSION" '.changelog | keys_unsorted | (index($v) + 1) as $i | .[$i] // ""' "$PAK_JSON")"
FIRST_BULLET="$(jq -r --arg v "$VERSION" '.changelog[$v]' "$PAK_JSON" | sed '/^[[:space:]]*$/d' | head -1)"

# --- --print-notes ---
notes="$("$SCRIPT" --print-notes)"
check "notes name the current version" "$notes" "$VERSION"
# A hand-written docs/release-notes/<version>.md replaces the generated notes,
# so the pak.json bullet is only expected when no such file exists. pak.json's
# changelog is kept short for the on-device Settings screen; the release page
# can afford the longer version.
LONG_NOTES="$REPO_ROOT/docs/release-notes/$VERSION.md"
if [ -f "$LONG_NOTES" ]; then
	check "notes come from the long-form file" "$notes" "$(head -1 "$LONG_NOTES")"
	printf 'ok   - pak.json bullet not required (long-form notes in use)\n'
else
	check "notes include the changelog bullet" "$notes" "$FIRST_BULLET"
fi
check "notes include a compare link" "$notes" "/compare/"

# The download table is the part of the notes a first-time reader needs, so it
# is generated rather than hand-written and every firmware must appear in it.
check "notes ask which file you need" "$notes" "Which file do I need?"
check "notes offer the muOS artifact" "$notes" "Itch-io.muOS.$VERSION.muxapp"
check "notes offer the NextUI pak" "$notes" "Itch-io.NextUI.$VERSION.pak.zip"
check "notes offer the NextUI multi-device bundle" "$notes" "Itch-io.NextUI.$VERSION.pakz"
check "notes keep the Pak Store filename" "$notes" '`Itch-io.pak.zip`'
check "compare link ends at current version" "$notes" "...$VERSION"
[ -n "$PREV" ] && check "compare link starts at previous version" "$notes" "/compare/$PREV..."

# --- --dry-run lists both artifacts and does not create a release ---
# Requires built dist/ artifacts; skip when absent (e.g. a fresh CI checkout).
#
# Also skip when the artifacts are stale. release.sh runs this suite before
# build.sh, so immediately after a version bump the built bundle still carries
# the old version and release-github.sh correctly refuses. Failing here would
# deadlock the one command that fixes it: the release cannot be built because
# the artifacts are old, and the artifacts stay old because the release cannot
# be built. A stale bundle is not a code defect.
#
# Read the stream to completion before extracting — `unzip -p | grep -m1` is the
# SIGPIPE race this script has a regression guard for further down.
PAKZ_PATH="$REPO_ROOT/dist/nextui/Itch-io.NextUI.$VERSION.pakz"
PAKZIP_PATH="$REPO_ROOT/dist/nextui/Itch-io.NextUI.$VERSION.pak.zip"
MUXAPP_PATH="$REPO_ROOT/dist/muos/Itch-io.muOS.$VERSION.muxapp"

_bundle_json="$(unzip -p "$PAKZ_PATH" 'Tools/*/Itch-io.pak/pak.json' 2>/dev/null || true)"
_bundle_vers="$(printf '%s\n' "$_bundle_json" \
    | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | sort -u)"
BUNDLE_VER="${_bundle_vers%%$'\n'*}"

if [ -n "$BUNDLE_VER" ] && [ "$BUNDLE_VER" != "$VERSION" ]; then
	printf 'skip - dry-run checks (dist/ holds %s, pak.json is %s; run scripts/release.sh)\n' \
		"$BUNDLE_VER" "$VERSION"
elif [ -f "$PAKZ_PATH" ] && [ -f "$PAKZIP_PATH" ] && [ -f "$MUXAPP_PATH" ]; then
	dry="$("$SCRIPT" --dry-run 2>&1)"; dry_rc=$?
	# Assert the exit status first. Without this, a script that dies early under
	# `set -e` produces empty output and is reported as three confusing
	# "missing: ..." failures instead of "it crashed".
	if [ "$dry_rc" -eq 0 ]; then
		printf 'ok   - dry-run exits 0\n'
	else
		printf 'FAIL - dry-run exits 0 (got %s, output: %s)\n' "$dry_rc" "${dry:-<empty>}"
		fail=1
	fi
	check "dry-run mentions the .pakz artifact" "$dry" "Itch-io.NextUI.$VERSION.pakz"
	check "dry-run mentions the .pak.zip artifact" "$dry" "Itch-io.NextUI.$VERSION.pak.zip"
	check "dry-run mentions the Pak Store copy" "$dry" "dist/Itch-io.pak.zip"
	check "dry-run mentions the .muxapp artifact" "$dry" "Itch-io.muOS.$VERSION.muxapp"
	check "dry-run announces itself" "$dry" "DRY RUN"

	# Regression guard for the SIGPIPE race that used to live in bundle_version:
	# it piped `unzip -p` into `grep -m1`, and since the glob matches one pak.json
	# per platform, grep closed the pipe while unzip was still streaming. unzip
	# died of SIGPIPE and `set -o pipefail` failed the whole script, silently.
	#
	# This is a structural check rather than a repeat-run loop on purpose. The
	# race is timing-dependent: measured at 8-in-30 on a cold page cache but only
	# ~1-in-50 once warm, so a loop that finishes in reasonable time catches a
	# reintroduction well under half the time. Grepping for the shape is instant
	# and deterministic.
	# tr collapses the backslash continuations so the pipeline is one line.
	if sed -n '/^bundle_version() {/,/^}/p' "$SCRIPT" | tr '\n' ' ' \
		| grep -qE 'unzip[^|]*\|[^|]*(grep -m|head)'; then
		printf 'FAIL - bundle_version pipes unzip into an early-exiting reader (SIGPIPE race)\n'
		fail=1
	else
		printf 'ok   - bundle_version does not pipe unzip into an early-exiting reader\n'
	fi
else
	printf 'skip - dry-run checks (no dist/ artifacts; run scripts/release.sh)\n'
fi

# --- pre-release notes ---
# A pre-release deliberately ships no Itch-io.pak.zip, because that is the
# filename the Pak Store matches on. The notes must not advertise it either.
pre="$("$SCRIPT" --print-notes --prerelease)"
check "pre-release notes say it is a test build" "$pre" "This is a test build"
check "pre-release notes still offer the muOS artifact" "$pre" "Itch-io.muOS.$VERSION.muxapp"
if printf '%s' "$pre" | grep -qF '`Itch-io.pak.zip`'; then
	printf 'FAIL - pre-release notes must not advertise the Pak Store asset\n'
	fail=1
else
	printf 'ok   - pre-release notes omit the Pak Store asset\n'
fi

# --- unknown flag is rejected ---
if "$SCRIPT" --bogus >/dev/null 2>&1; then
	printf 'FAIL - unknown flag should exit non-zero\n'
	fail=1
else
	printf 'ok   - unknown flag rejected\n'
fi

[ "$fail" -eq 0 ] && printf '\nAll release-github tests passed.\n'
exit $fail

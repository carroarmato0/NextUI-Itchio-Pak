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
check "notes include the changelog bullet" "$notes" "$FIRST_BULLET"
check "notes include a compare link" "$notes" "/compare/"
check "compare link ends at current version" "$notes" "...$VERSION"
[ -n "$PREV" ] && check "compare link starts at previous version" "$notes" "/compare/$PREV..."

# --- --dry-run lists both artifacts and does not create a release ---
# Requires built dist/ artifacts; skip when absent (e.g. a fresh CI checkout).
if [ -f "$REPO_ROOT/dist/Itch-io.pakz" ] && [ -f "$REPO_ROOT/dist/Itch-io.pak.zip" ]; then
	dry="$("$SCRIPT" --dry-run 2>&1)"
	check "dry-run mentions the .pakz artifact" "$dry" "Itch-io.pakz"
	check "dry-run mentions the .pak.zip artifact" "$dry" "Itch-io.pak.zip"
	check "dry-run announces itself" "$dry" "DRY RUN"
else
	printf 'skip - dry-run checks (no dist/ artifacts; run scripts/release.sh)\n'
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

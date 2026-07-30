#!/usr/bin/env bash
#
# release-github.sh — publish a GitHub release for the current pak version.
#
# Reads the version and changelog from pak.json, verifies the dist/ artifacts
# were built for that version, pushes the tag, and creates the GitHub release
# with auto-generated notes and both bundle artifacts attached.
#
# Usage:
#   scripts/release-github.sh                 Create the release (pushes tag, uploads artifacts)
#   scripts/release-github.sh --dry-run       Show the plan + notes; touch nothing
#   scripts/release-github.sh --print-notes   Print the release notes only (used by tests)
#
# Prerequisites: gh (authenticated), jq, and up-to-date dist/ artifacts from
# scripts/release.sh.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PAK_JSON="pak.json"
DIST_DIR="dist"
PAKZ="$DIST_DIR/Itch-io.pakz"
PAKZIP="$DIST_DIR/Itch-io.pak.zip"

err() { printf 'error: %s\n' "$*" >&2; exit 1; }

pak_version() {
	grep '"version"' "$PAK_JSON" | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

# changelog_entry VERSION — the raw changelog string for a version (newlines rendered).
changelog_entry() { jq -r --arg v "$1" '.changelog[$v] // ""' "$PAK_JSON"; }

# prev_version VERSION — the changelog key immediately after VERSION (file order = newest first).
prev_version() {
	jq -r --arg v "$1" '.changelog | keys_unsorted | (index($v) + 1) as $i | .[$i] // ""' "$PAK_JSON"
}

# bundle_version — the version embedded in the built .pakz artifact, or "" if unreadable.
#
# The glob matches one pak.json per platform (tg5040, tg5050, my355), so unzip
# keeps streaming after the first match. Piping it straight into `grep -m1` let
# grep close the pipe early, killing unzip with SIGPIPE; under `set -o pipefail`
# that failed the whole script at line 88 — silently, and only when the timing
# happened to land that way (~8 of 30 runs on this 37MB bundle). Read the stream
# to completion first, then extract.
bundle_version() {
	local json versions
	json="$(unzip -p "$PAKZ" 'Tools/*/Itch-io.pak/pak.json' 2>/dev/null || true)"
	versions="$(printf '%s\n' "$json" \
		| sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
		| sort -u)"
	# Every platform bundle is packaged from the same pak.json, so more than one
	# distinct version means release.sh shipped a stale platform directory.
	if [ "$(printf '%s\n' "$versions" | grep -c .)" -gt 1 ]; then
		err "mixed versions inside $PAKZ: $(printf '%s' "$versions" | tr '\n' ' ')— re-run scripts/release.sh"
	fi
	printf '%s\n' "$versions"
}

# notes_file VERSION — path to a hand-written long-form note for this release,
# if one exists. pak.json's changelog is shown on-device where space is tight, so
# it stays short; the release page can afford the longer version.
notes_file() { printf 'docs/release-notes/%s.md' "$1"; }

build_notes() {
	local v prev repo bullets
	v="$(pak_version)"
	# A long-form file, when present, replaces the generated notes entirely —
	# including the heading, so it can be structured however it likes. The
	# compare link is still appended.
	if [ -f "$(notes_file "$v")" ]; then
		cat "$(notes_file "$v")"
		prev="$(prev_version "$v")"
		repo="$(jq -r '.repo_url' "$PAK_JSON")"
		if [ -n "$prev" ]; then
			printf '\n**Full changelog:** %s/compare/%s...%s\n' "$repo" "$prev" "$v"
		fi
		return
	fi
	prev="$(prev_version "$v")"
	repo="$(jq -r '.repo_url' "$PAK_JSON")"
	bullets="$(changelog_entry "$v" | sed '/^[[:space:]]*$/d')"

	printf '## Itch.io Pak %s\n\n' "$v"
	if [ -n "$bullets" ]; then
		printf '%s\n' "$bullets"
	else
		printf '_No changelog entry for %s in pak.json._\n' "$v"
	fi
	if [ -n "$prev" ]; then
		printf '\n**Full changelog:** %s/compare/%s...%s\n' "$repo" "$prev" "$v"
	fi
}

MODE=release
case "${1:-}" in
	--print-notes) MODE=notes ;;
	--dry-run)     MODE=dry ;;
	"")            MODE=release ;;
	*)             err "unknown argument: $1 (use --dry-run or --print-notes)" ;;
esac

command -v jq >/dev/null 2>&1 || err "jq not found — install jq"

VERSION="$(pak_version)"
[ -n "$VERSION" ] || err "could not read version from $PAK_JSON"

# --print-notes is pure: no artifact/tool checks, no side effects.
if [ "$MODE" = notes ]; then
	build_notes
	exit 0
fi

# Artifact preconditions (dry-run and real release both validate these).
[ -f "$PAKZ" ]   || err "missing $PAKZ — run scripts/release.sh first"
[ -f "$PAKZIP" ] || err "missing $PAKZIP — run scripts/release.sh first"

BUNDLE_VER="$(bundle_version)"
[ -n "$BUNDLE_VER" ] || err "could not read version from $PAKZ — is the bundle intact?"
[ "$BUNDLE_VER" = "$VERSION" ] \
	|| err "artifact version ($BUNDLE_VER) != pak.json ($VERSION) — re-run scripts/release.sh"

if [ "$MODE" = dry ]; then
	printf 'DRY RUN — would publish release %s\n' "$VERSION"
	printf 'Assets:\n  %s\n  %s\n' "$PAKZ" "$PAKZIP"
	printf -- '---- notes ----\n'
	build_notes
	exit 0
fi

# Real release: needs gh, auth, and no pre-existing release for this version.
command -v gh >/dev/null 2>&1 || err "gh CLI not found — install GitHub CLI"
gh auth status >/dev/null 2>&1 || err "gh not authenticated — run: gh auth login"

if gh release view "$VERSION" >/dev/null 2>&1; then
	err "release $VERSION already exists — delete it or bump the version in $PAK_JSON"
fi

if ! git rev-parse -q --verify "refs/tags/$VERSION" >/dev/null; then
	printf '==> tagging %s at HEAD\n' "$VERSION"
	git tag "$VERSION"
fi

printf '==> pushing branch and tag\n'
git push origin HEAD
git push origin "$VERSION"

NOTES_FILE="$(mktemp)"
trap 'rm -f "$NOTES_FILE"' EXIT
build_notes > "$NOTES_FILE"

printf '==> creating GitHub release %s\n' "$VERSION"
gh release create "$VERSION" \
	--title "$VERSION" \
	--notes-file "$NOTES_FILE" \
	"$PAKZ" "$PAKZIP"

printf '==> done: %s\n' "$(gh release view "$VERSION" --json url --jq .url)"

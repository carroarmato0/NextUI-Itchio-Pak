#!/usr/bin/env bash
#
# palette-audit.sh — render every scene under every shipped NextUI palette and
# fail on text that is too low-contrast to read.
#
# NextUI ships eighteen palettes, seven of them light. A colour that reads well
# on the app's dark default can be invisible on Catppuccin Latte, and nobody is
# going to check 228 combinations by hand. This renders them all off-screen, at
# every shipping screen geometry, in about thirty seconds and reports what is
# unreadable.
#
# The audit measures the frame that was actually produced: each string's colour
# against the pixels sampled underneath it, not theme accessors in isolation.
#
# Run: scripts/palette-audit.sh [--out-dir DIR] [--sheet]
#
# Requires SDL2 + SDL2_ttf on the host. Skips (exit 0) when they are absent, so
# a machine without them can still run the rest of the suite.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

OUT_DIR="${TMPDIR:-/tmp}/itchio-palette-audit"
EXTRA=""
while [ $# -gt 0 ]; do
    case "$1" in
        --out-dir) OUT_DIR="$2"; shift 2 ;;
        --sheet)   EXTRA="$EXTRA --sheet"; shift ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

# SDL2 is a cgo dependency; without it devshot cannot build. That is a missing
# toolchain, not a failing audit.
if ! go build -o /dev/null ./cmd/devshot 2>/dev/null; then
    printf 'skip - palette audit (cannot build cmd/devshot; SDL2 dev headers missing?)\n'
    exit 0
fi

log="$(mktemp)"
trap 'rm -f "$log"' EXIT

# Geometries every shipping device actually presents.  1024x768 is the TrimUI
# Brick; 640x480 is the Miyoo Flip and most of the H700 family; 720x480 is the
# RG34XX line and 720x720 is the RG Cube XX.  The last two are new with H700 and
# are the reason the size class stopped keying on width alone — rendering them
# here is what makes that change something we looked at rather than reasoned
# about.
GEOMETRIES="1024x768 640x480 720x480 720x720"

STATUS=0
for geom in $GEOMETRIES; do
    W="${geom%x*}"
    H="${geom#*x}"
    # shellcheck disable=SC2086
    if go run ./cmd/devshot --all --palettes all --width "$W" --height "$H" \
        --out-dir "$OUT_DIR/$geom" --audit $EXTRA >"$log" 2>&1; then
        summary="$(grep -E 'finding\(s\)' "$log" | sed 's/^ *//')"
        # Count renders only; --sheet adds one "contact sheet:" line per scene.
        renders="$(grep -cE '^    [a-z].*\.png ' "$log")"
        printf 'ok   - palette audit %s: %s renders, %s\n' "$geom" "$renders" "${summary:-no findings}"
    else
        printf 'FAIL - palette audit %s found unreadable text:\n' "$geom"
        grep -E '^\s+FAIL' "$log" | sed 's/^ */  /'
        printf '  (renders in %s/%s)\n' "$OUT_DIR" "$geom"
        STATUS=1
    fi
done

exit "$STATUS"

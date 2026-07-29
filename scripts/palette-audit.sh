#!/usr/bin/env bash
#
# palette-audit.sh — render every scene under every shipped NextUI palette and
# fail on text that is too low-contrast to read.
#
# NextUI ships eighteen palettes, seven of them light. A colour that reads well
# on the app's dark default can be invisible on Catppuccin Latte, and nobody is
# going to check 228 combinations by hand. This renders them all off-screen in
# about seven seconds and reports what is unreadable.
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

# shellcheck disable=SC2086
if go run ./cmd/devshot --all --palettes all --out-dir "$OUT_DIR" --audit $EXTRA >"$log" 2>&1; then
    summary="$(grep -E 'finding\(s\)' "$log" | sed 's/^ *//')"
    renders="$(grep -cE '\.png ' "$log")"
    printf 'ok   - palette audit: %s renders, %s\n' "$renders" "${summary:-no findings}"
    exit 0
fi

printf 'FAIL - palette audit found unreadable text:\n'
grep -E '^\s+FAIL' "$log" | sed 's/^ */  /'
printf '  (renders in %s)\n' "$OUT_DIR"
exit 1

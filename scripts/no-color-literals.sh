#!/usr/bin/env bash
#
# no-color-literals.sh — fail if a drawing call carries a hardcoded RGB triple.
#
# Every colour the UI draws must come from internal/theme, so that a NextUI
# palette (including the seven light ones) recolours the whole app. A literal
# triple silently opts one element out — and reads fine on the dark default
# while being invisible on Catppuccin Latte.
#
# Run: scripts/no-color-literals.sh   (exit 0 = clean)
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

# Drawing calls with three numeric arguments, and `x, y, z = 1, 2, 3` colour
# assignments. Test files are exempt: fixtures are literals by nature.
PATTERN='(Draw|Clear)[A-Za-z]*\(.*\b[0-9]{1,3}, ?[0-9]{1,3}, ?[0-9]{1,3}\)|= [0-9]{1,3}, ?[0-9]{1,3}, ?[0-9]{1,3}$'

# SetDrawColor(0, 0, 0, 0) is a fully transparent clear, not a theme colour.
ALLOW='SetDrawColor\(0, 0, 0, 0\)'

hits="$(grep -rnE "$PATTERN" --include='*.go' internal cmd 2>/dev/null \
    | grep -vE '_test\.go' \
    | grep -vE "$ALLOW")"

if [ -n "$hits" ]; then
    printf 'FAIL - hardcoded color literals found (use internal/theme accessors):\n'
    printf '%s\n' "$hits" | sed 's/^/  /'
    exit 1
fi

printf 'ok   - no hardcoded color literals in drawing calls\n'

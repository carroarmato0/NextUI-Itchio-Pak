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

# Any run of three 0-255 integers, anywhere in the file. Earlier versions of
# this check anchored on the triple being the final argument of a Draw call and
# on assignments starting with a digit; both were wrong, and nine literals
# survived a migration that claimed to be complete — including
# `drawActionRow("A", "Download", 80, 200, 80, ...)`, where the triple is
# mid-argument-list, and `label, r, g, b = "Download again", 80, 200, 80`,
# where the assignment starts with a string.
#
# Casting a wide net means coordinates and sizes match too, so genuine non-colour
# triples are listed in ALLOW below rather than excluded by a cleverer pattern.
# A false positive costs one allowlist line; a false negative ships a colour that
# ignores the palette.

# Known non-colour triples.
#   SetDrawColor(0,0,0,0)  a fully transparent clear
#   time.Date(...)         fixture timestamps
#   Tone([3]uint8{...})    category hues already routed through the theme
ALLOW='SetDrawColor\(0, 0, 0, 0\)|time\.Date\(|Theme\.Tone\(\[3\]uint8|image\.Rect\('

# Scoped to the drawing code. internal/theme is where the base hues are defined
# by design, and cmd/devshot is a development tool rather than shipped UI.
SCOPE='internal/ui internal/renderer cmd/itchio-pak'

# shellcheck disable=SC2086
hits="$(grep -rnE '\b[0-9]{1,3}, ?[0-9]{1,3}, ?[0-9]{1,3}\b' --include='*.go' $SCOPE 2>/dev/null \
    | grep -vE '_test\.go' \
    | grep -vE "$ALLOW")"

if [ -n "$hits" ]; then
    printf 'FAIL - hardcoded color literals found (use internal/theme accessors):\n'
    printf '%s\n' "$hits" | sed 's/^/  /'
    exit 1
fi

printf 'ok   - no hardcoded color literals in drawing calls\n'

# Channel arithmetic on a colour, e.g. `bg[0]+22`. These are uint8, so adding to
# a light palette's channel wraps to near-black or, worse, to a saturated hue:
# the filter panel spent a release rendering #1109F2 blue on Mustard Butter.
# Use theme.Shade/Lighten/Darken/Mix, which saturate.
ARITH='\[[0-9]\][[:space:]]*[-+][[:space:]]*[0-9]+'
# shellcheck disable=SC2086
arith="$(grep -rnE "$ARITH" --include='*.go' $SCOPE 2>/dev/null \
    | grep -vE '_test\.go' \
    | grep -E 'Draw|Clear|Color')"

if [ -n "$arith" ]; then
    printf 'FAIL - uint8 color channel arithmetic found (use theme.Shade/Mix, which saturate):\n'
    printf '%s\n' "$arith" | sed 's/^/  /'
    exit 1
fi

printf 'ok   - no uint8 color channel arithmetic in drawing calls\n'

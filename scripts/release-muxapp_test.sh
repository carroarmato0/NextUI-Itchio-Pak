#!/bin/sh
# Structural checks on the muOS application archive.
#
# A .muxapp is installed by muOS's Archive Manager, which extracts it into the
# application store and then runs mux_launch.sh. Everything asserted here is
# something that, if wrong, produces an app that installs into the wrong place
# or silently refuses to start — with no error the user can act on.
#
# Skips cleanly when the artifact has not been built, so ./scripts/test.sh can
# run it unconditionally.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"

PASS=0
FAIL=0

ok()   { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; }

# check <description> <command...> — runs the command and reports, without
# letting a failure trip `set -e` and abort before the summary is printed.
check() {
    DESC="$1"
    shift
    if "$@"; then ok "$DESC"; else fail "$DESC"; fi
}

# has <line> — exact-match a path in the archive listing.
has() { printf '%s\n' "$LIST" | grep -qx "$1"; }

VERSION="$(pak_version)"
MUXAPP="dist/muos/Itch-io.muOS.$VERSION.muxapp"

if [ ! -f "$MUXAPP" ]; then
    echo "note: skipping muxapp checks ($MUXAPP not built)" >&2
    exit 0
fi

if ! command -v unzip >/dev/null 2>&1; then
    echo "note: skipping muxapp checks (unzip not found)" >&2
    exit 0
fi

LIST="$(unzip -Z1 "$MUXAPP")"

# muOS extracts the archive into the application store, so the archive root has
# to be the application directory itself. Rooting it at MUOS/application/Itch-io
# — the path it ends up at — would nest it one level too deep and the app would
# never appear in the menu.
if printf '%s\n' "$LIST" | grep -qv '^Itch-io/'; then
    fail "every entry is under Itch-io/"
    printf '%s\n' "$LIST" | grep -v '^Itch-io/' | sed 's/^/       unexpected: /'
else
    ok "every entry is under Itch-io/"
fi

for want in \
    "Itch-io/mux_launch.sh" \
    "Itch-io/itchio" \
    "Itch-io/version.txt" \
    "Itch-io/mux_lang.ini" \
    "Itch-io/glyph/itchio.svg" \
    "Itch-io/assets/font.ttf" \
    "Itch-io/assets/ca-certificates.crt"
do
    check "contains $want" has "$want"
done

# NextUI packaging must not leak into the muOS archive.
for unwanted in "Itch-io/pak.json" "Itch-io/launch.sh"; do
    if printf '%s\n' "$LIST" | grep -qx "$unwanted"; then
        fail "does not ship $unwanted (NextUI-only)"
    else
        ok "does not ship $unwanted (NextUI-only)"
    fi
done

# Bundling SDL2 would shadow muOS's own patched build, which is the one that
# honours SDL_ROTATION, SDL_HQ_SCALER and SDL_BLITTER_DISABLED.
if printf '%s\n' "$LIST" | grep -q 'libSDL2'; then
    fail "does not bundle SDL2 (muOS ships its own)"
else
    ok "does not bundle SDL2 (muOS ships its own)"
fi

# Without the exec bit muOS cannot start the app, and there is no error message
# that points at the cause.
for exe in "Itch-io/mux_launch.sh" "Itch-io/itchio"; do
    MODE="$(unzip -Z "$MUXAPP" "$exe" 2>/dev/null | awk 'NR==1 {print $1}')"
    case "$MODE" in
        -*x*) ok "$exe is executable ($MODE)" ;;
        *)    fail "$exe is not executable (mode $MODE)" ;;
    esac
done

# The launcher's header comments are parsed by the frontend for the menu entry.
HEADER="$(unzip -p "$MUXAPP" "Itch-io/mux_launch.sh" | head -8)"
for field in "HELP:" "ICON:" "GRID:"; do
    check "mux_launch.sh declares $field" \
        sh -c 'printf "%s\n" "$1" | grep -q "^# $2"' _ "$HEADER" "$field"
done

# The ICON header names a glyph; shipping a different filename means muOS falls
# back to a generic icon with nothing to explain why.
ICON="$(printf '%s\n' "$HEADER" | sed -n 's/^# ICON: *//p')"
check "glyph matches the ICON header ($ICON)" has "Itch-io/glyph/$ICON.svg"

# The version marker is what release-github.sh verifies the artifact against.
STAMPED="$(unzip -p "$MUXAPP" "Itch-io/version.txt" | tr -d '\r\n')"
check "version.txt says $VERSION (found '$STAMPED')" [ "$STAMPED" = "$VERSION" ]

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

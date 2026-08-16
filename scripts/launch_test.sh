#!/bin/sh
# Checks how launch.sh resolves SDL2 before it execs the app.
#
# Getting this wrong is silent: the app either fails to start with no useful
# message, or starts against the wrong SDL2 and renders nothing. On H700 the
# temptation is /usr/lib, which carries stock Anbernic's SDL2 2.0.12 with no
# SDL2_ttf beside it, while NextUI installs the library we need in
# $SYSTEM_PATH/lib.
#
# Runs launch.sh for real against a fake SD card, with a stub binary that prints
# LD_LIBRARY_PATH instead of starting the app.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PASS=0
FAIL=0
ok()   { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; }

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# run_launch <platform> <system-lib-has-sdl: yes|no> -> prints LD_LIBRARY_PATH
run_launch() {
    _plat="$1"
    _sys_sdl="$2"

    _pak="$TMP/$_plat/Itch-io.pak"
    rm -rf "$TMP/$_plat"
    mkdir -p "$_pak/assets" "$_pak/lib/tg5040" "$_pak/lib/tg5050" "$_pak/lib/my355"
    cp launch.sh "$_pak/launch.sh"
    chmod +x "$_pak/launch.sh"

    # Stub app: prints the library path and exits instead of starting SDL.
    printf '#!/bin/sh\nprintf "%%s\\n" "$LD_LIBRARY_PATH"\n' > "$_pak/itchio"
    chmod +x "$_pak/itchio"

    _sys="$TMP/$_plat/.system/$_plat"
    mkdir -p "$_sys/lib"
    if [ "$_sys_sdl" = "yes" ]; then
        : > "$_sys/lib/libSDL2-2.0.so.0"
    fi

    PLATFORM="$_plat" \
    SYSTEM_PATH="$_sys" \
    SHARED_USERDATA_PATH="$TMP/$_plat/userdata" \
    LD_LIBRARY_PATH="" \
        "$_pak/launch.sh" 2>/dev/null
}

# --- h700: NextUI's own SDL2 must win, and no bundled dir may be used --------
OUT="$(run_launch h700 yes)"
case "$OUT" in
    "$TMP/h700/.system/h700/lib":*|"$TMP/h700/.system/h700/lib")
        ok "h700 puts \$SYSTEM_PATH/lib first" ;;
    *)  fail "h700 puts \$SYSTEM_PATH/lib first (got: $OUT)" ;;
esac

case "$OUT" in
    *"/lib/tg5040"*|*"/lib/tg5050"*|*"/lib/my355"*)
        fail "h700 uses no bundled lib dir (got: $OUT)" ;;
    *)  ok "h700 uses no bundled lib dir" ;;
esac

# --- tg5040: unchanged, still falls back to its bundled dir ------------------
OUT="$(run_launch tg5040 no)"
case "$OUT" in
    *"/lib/tg5040"*) ok "tg5040 still selects its bundled lib dir" ;;
    *)               fail "tg5040 still selects its bundled lib dir (got: $OUT)" ;;
esac

case "$OUT" in
    *"/lib/my355"*|*"/lib/tg5050"*)
        fail "tg5040 selects only its own lib dir (got: $OUT)" ;;
    *)  ok "tg5040 selects only its own lib dir" ;;
esac

# --- my355 and tg5050 pick their own ----------------------------------------
for plat in my355 tg5050; do
    OUT="$(run_launch "$plat" no)"
    case "$OUT" in
        *"/lib/$plat"*) ok "$plat selects its bundled lib dir" ;;
        *)              fail "$plat selects its bundled lib dir (got: $OUT)" ;;
    esac
done

# --- an inherited LD_LIBRARY_PATH is preserved, never replaced --------------
_pak="$TMP/tg5040/Itch-io.pak"
OUT="$(PLATFORM=tg5040 \
    SYSTEM_PATH="$TMP/tg5040/.system/tg5040" \
    SHARED_USERDATA_PATH="$TMP/tg5040/userdata" \
    LD_LIBRARY_PATH="/inherited/path" \
    "$_pak/launch.sh" 2>/dev/null)"
case "$OUT" in
    *"/inherited/path"*) ok "inherited LD_LIBRARY_PATH is preserved" ;;
    *)                   fail "inherited LD_LIBRARY_PATH is preserved (got: $OUT)" ;;
esac

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

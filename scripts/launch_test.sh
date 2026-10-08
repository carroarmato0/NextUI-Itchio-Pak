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

# run_launch <platform> <system-lib: none|sdl2|full> -> prints LD_LIBRARY_PATH
#
# The second argument controls what $SYSTEM_PATH/lib contains:
#   none - neither file (the common case today on tg5040/tg5050/my355)
#   sdl2 - libSDL2-2.0.so.0 only, no libSDL2_ttf (a partial pair — the guard
#          in launch.sh must NOT fire on this, or a firmware that ships SDL2
#          without SDL2_ttf would leave the app with no ttf at all)
#   full - both libSDL2-2.0.so.0 and libSDL2_ttf-2.0.so.0 (a complete pair —
#          the guard fires, and the bundled directory drops out)
run_launch() {
    _plat="$1"
    _sys_sdl="$2"

    _pak="$TMP/$_plat/Itch-io.pak"
    # shellcheck disable=SC2115 # $TMP is always a fresh mktemp -d (set above,
    # never empty/unset here), so "$TMP/$_plat" can never collapse to "/".
    rm -rf "$TMP/$_plat"
    mkdir -p "$_pak/assets" "$_pak/lib/tg5040" "$_pak/lib/tg5050" "$_pak/lib/my355"
    cp launch.sh "$_pak/launch.sh"
    chmod +x "$_pak/launch.sh"

    # Stub app: prints the library path and exits instead of starting SDL.
    # shellcheck disable=SC2016 # Single-quoting is deliberate: the stub script
    # must contain the literal text $LD_LIBRARY_PATH, expanded when the stub
    # itself runs later, not when this printf runs now.
    printf '#!/bin/sh\nprintf "%%s\\n" "$LD_LIBRARY_PATH"\n' > "$_pak/itchio"
    chmod +x "$_pak/itchio"

    _sys="$TMP/$_plat/.system/$_plat"
    mkdir -p "$_sys/lib"
    case "$_sys_sdl" in
        sdl2) : > "$_sys/lib/libSDL2-2.0.so.0" ;;
        full) : > "$_sys/lib/libSDL2-2.0.so.0"
              : > "$_sys/lib/libSDL2_ttf-2.0.so.0" ;;
    esac

    PLATFORM="$_plat" \
    SYSTEM_PATH="$_sys" \
    SHARED_USERDATA_PATH="$TMP/$_plat/userdata" \
    LD_LIBRARY_PATH="" \
        "$_pak/launch.sh" 2>/dev/null
}

# --- h700: NextUI's own SDL2 must win, and no bundled dir may be used --------
OUT="$(run_launch h700 full)"
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
OUT="$(run_launch tg5040 none)"
case "$OUT" in
    *"/lib/tg5040"*) ok "tg5040 still selects its bundled lib dir" ;;
    *)               fail "tg5040 still selects its bundled lib dir (got: $OUT)" ;;
esac

case "$OUT" in
    *"/lib/my355"*|*"/lib/tg5050"*)
        fail "tg5040 selects only its own lib dir (got: $OUT)" ;;
    *)  ok "tg5040 selects only its own lib dir" ;;
esac

# --- an inherited LD_LIBRARY_PATH is preserved, never replaced --------------
# Deliberately placed right after the "tg5040 still falls back to its bundled
# dir" case above (run_launch tg5040 none) and before the "tg5040 with a
# complete firmware-provided SDL2 pair" case below (run_launch tg5040 full).
# It reuses that tree without recreating it, so it must run while that
# tree's $SYSTEM_PATH/lib has no SDL2 stub in it — otherwise the guard
# fires, the bundled dir drops out, and this stops covering "bundled dir
# plus inherited path" and starts covering "no bundled dir plus inherited
# path" instead.
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

case "$OUT" in
    *"/lib/tg5040"*) ok "inherited LD_LIBRARY_PATH case still covers the bundled dir" ;;
    *)               fail "inherited LD_LIBRARY_PATH case still covers the bundled dir (got: $OUT)" ;;
esac

# --- tg5040 with a complete firmware-provided SDL2 pair: guard fires here too
# Pins "a complete firmware SDL2 pair is authoritative on every platform" as
# intended behaviour, not an h700-only quirk. This is the case that would
# catch a future NextUI release that starts shipping SDL2 in
# .system/tg5040/lib.
OUT="$(run_launch tg5040 full)"
case "$OUT" in
    "$TMP/tg5040/.system/tg5040/lib":*|"$TMP/tg5040/.system/tg5040/lib")
        ok "tg5040 with a complete \$SYSTEM_PATH/lib pair puts it first" ;;
    *)  fail "tg5040 with a complete \$SYSTEM_PATH/lib pair puts it first (got: $OUT)" ;;
esac

case "$OUT" in
    *"/lib/tg5040"*)
        fail "tg5040 drops its bundled dir when the firmware pair is complete (got: $OUT)" ;;
    *)  ok "tg5040 drops its bundled dir when the firmware pair is complete" ;;
esac

# --- tg5040 with only libSDL2, no libSDL2_ttf: bundled dir must be kept -----
# The guard must not fire on a partial pair: if it did, and a future NextUI
# release shipped SDL2 without SDL2_ttf in $SYSTEM_PATH/lib, blanking the
# bundled directory would leave the app with no ttf at all.
OUT="$(run_launch tg5040 sdl2)"
case "$OUT" in
    *"/lib/tg5040"*)
        ok "tg5040 keeps its bundled dir when the firmware pair is only SDL2" ;;
    *)  fail "tg5040 keeps its bundled dir when the firmware pair is only SDL2 (got: $OUT)" ;;
esac

# --- my355 and tg5050 pick their own ----------------------------------------
# tg5040 and tg5050's $SYSTEM_PATH/lib were checked over ADB and confirmed to
# hold no libSDL2 (see the design spec). my355 was not: no Miyoo Flip was
# available, so whether the real $SYSTEM_PATH/lib on that device also holds
# no libSDL2 is assumed, not measured. This suite only exercises the guard
# logic against a fake tree either way — it cannot substitute for that
# hardware check on my355.
for plat in my355 tg5050; do
    OUT="$(run_launch "$plat" none)"
    case "$OUT" in
        *"/lib/$plat"*) ok "$plat selects its bundled lib dir" ;;
        *)              fail "$plat selects its bundled lib dir (got: $OUT)" ;;
    esac
done

# --- unset $SYSTEM_PATH must not degrade the search to the literal /lib -----
_pak="$TMP/tg5040/Itch-io.pak"
OUT="$(env -u SYSTEM_PATH \
    PLATFORM=tg5040 \
    SHARED_USERDATA_PATH="$TMP/tg5040/userdata" \
    LD_LIBRARY_PATH="" \
    "$_pak/launch.sh" 2>/dev/null)"
case "$OUT" in
    /lib:*|*:/lib:*|*:/lib|/lib)
        fail "unset \$SYSTEM_PATH contributes no bare /lib entry (got: $OUT)" ;;
    *)  ok "unset \$SYSTEM_PATH contributes no bare /lib entry" ;;
esac

case "$OUT" in
    *"/lib/tg5040"*) ok "unset \$SYSTEM_PATH still selects the bundled lib dir" ;;
    *)                fail "unset \$SYSTEM_PATH still selects the bundled lib dir (got: $OUT)" ;;
esac

# --- In-place update rollback ------------------------------------------------
#
# run_update <case> sets up Tools/tg5040/Itch-io.pak (new) and .Itch-io.pak.prev
# (old), each with a stub itchio, and runs the new launch.sh. launch.sh derives
# $HOME as $SHARED_USERDATA_PATH/Itch-io, so the pending file is seeded there.
# The stubs record which version ran in $HOME/ran.
#   confirm - the new stub deletes the pending file (it started)
#   crash   - the new stub exits 1 without confirming
#   noprev  - like crash, but there is no .prev to restore
run_update() {
    _case="$1"
    _root="$TMP/upd-$_case"
    rm -rf "$_root"
    _tools="$_root/Tools/tg5040"
    _home="$_root/shared/Itch-io"
    mkdir -p "$_tools/Itch-io.pak/assets" "$_home"
    cp launch.sh "$_tools/Itch-io.pak/launch.sh"
    if [ "$_case" = confirm ]; then
        # shellcheck disable=SC2016
        printf '#!/bin/sh\necho new >> "$HOME/ran"\nrm -f "$HOME/update_pending.json"\n' > "$_tools/Itch-io.pak/itchio"
    else
        # shellcheck disable=SC2016
        printf '#!/bin/sh\necho new >> "$HOME/ran"\nexit 1\n' > "$_tools/Itch-io.pak/itchio"
    fi
    chmod +x "$_tools/Itch-io.pak/itchio"
    if [ "$_case" != noprev ]; then
        mkdir -p "$_tools/.Itch-io.pak.prev/assets"
        cp launch.sh "$_tools/.Itch-io.pak.prev/launch.sh"
        # shellcheck disable=SC2016
        printf '#!/bin/sh\necho old >> "$HOME/ran"\n' > "$_tools/.Itch-io.pak.prev/itchio"
        chmod +x "$_tools/.Itch-io.pak.prev/itchio"
    fi
    printf '{"from":"v1.1.0-rc5","to":"v1.1.0-rc6"}' > "$_home/update_pending.json"
    SHARED_USERDATA_PATH="$_root/shared" PLATFORM=tg5040 \
        sh "$_tools/Itch-io.pak/launch.sh" >/dev/null 2>&1 || true
}

run_update confirm
H="$TMP/upd-confirm/shared/Itch-io"; T="$TMP/upd-confirm/Tools/tg5040"
# shellcheck disable=SC2015 # ok/fail always return 0; this is the assertion
# idiom used throughout this block, not the if-then-else footgun SC2015 warns
# about.
[ "$(cat "$H/ran")" = "new" ] && ok "update confirmed: only the new version ran" || fail "update confirmed: ran '$(cat "$H/ran")'"
# shellcheck disable=SC2015
[ -d "$T/.Itch-io.pak.prev" ] && ok "update confirmed: .prev left for the app to remove" || fail "update confirmed: .prev vanished"
# shellcheck disable=SC2015
[ ! -f "$H/update_failed.json" ] && ok "update confirmed: no failure recorded" || fail "update confirmed: failure recorded"

run_update crash
H="$TMP/upd-crash/shared/Itch-io"; T="$TMP/upd-crash/Tools/tg5040"
# shellcheck disable=SC2015
[ "$(tr '\n' ' ' < "$H/ran")" = "new old " ] && ok "rollback when the binary does not confirm: new, then old ran" || fail "rollback: ran '$(tr '\n' ' ' < "$H/ran")'"
# shellcheck disable=SC2015
grep -q 'echo old' "$T/Itch-io.pak/itchio" && ok "rollback: the old version is live again" || fail "rollback: live folder is not the old version"
# shellcheck disable=SC2015
[ -d "$T/.Itch-io.pak.failed" ] && ok "rollback: the failed version is kept as .failed" || fail "rollback: no .failed"
# shellcheck disable=SC2015
[ ! -f "$H/update_pending.json" ] && [ -f "$H/update_failed.json" ] && ok "rollback: pending moved to update_failed.json" || fail "rollback: pending/failed files wrong"
# shellcheck disable=SC2015
grep -q rollback "$H/update_launcher.log" && ok "rollback: launcher logged it" || fail "rollback: no launcher log"

# The restored launcher must not roll back again: old exits 0 without a
# pending file, and nothing else runs.
# shellcheck disable=SC2015
[ "$(wc -l < "$H/ran" | tr -d ' ')" = 2 ] && ok "no loop after rollback" || fail "no loop after rollback: ran $(wc -l < "$H/ran") times"

run_update noprev
H="$TMP/upd-noprev/shared/Itch-io"
# shellcheck disable=SC2015
[ ! -f "$H/update_pending.json" ] && [ -f "$H/update_pending.orphan.json" ] && [ ! -f "$H/update_failed.json" ] \
    && ok "no .prev: pending set aside, nothing restored" || fail "no .prev: pending/orphan/failed files wrong"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

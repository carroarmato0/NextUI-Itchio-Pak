#!/bin/sh
# Structural checks on the NextUI archives.
#
# The muOS archive has had these since it was written; the NextUI ones never
# did, which is how a .gitkeep shipped to devices unnoticed for a long time.
# Everything asserted here is something that, if wrong, produces a pak that
# installs into the wrong place, refuses to start, or silently loses a feature —
# with nothing in the failure that points at the cause.
#
# Skips cleanly when the artifacts have not been built, so ./scripts/test.sh can
# run it unconditionally.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"

PASS=0
FAIL=0

ok()   { PASS=$((PASS + 1)); printf 'ok   - %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf 'FAIL - %s\n' "$1"; }

# check <description> <command...> — reports rather than letting `set -e` abort
# before the summary prints.
check() {
    DESC="$1"
    shift
    if "$@"; then ok "$DESC"; else fail "$DESC"; fi
}

VERSION="$(pak_version)"
PAKZIP="dist/nextui/Itch-io.NextUI.$VERSION.pak.zip"
PAKZ="dist/nextui/Itch-io.NextUI.$VERSION.pakz"
STORE_COPY="dist/Itch-io.pak.zip"

if [ ! -f "$PAKZIP" ] || [ ! -f "$PAKZ" ]; then
    echo "note: skipping NextUI archive checks (not built)" >&2
    exit 0
fi

if ! command -v unzip >/dev/null 2>&1; then
    echo "note: skipping NextUI archive checks (unzip not found)" >&2
    exit 0
fi

NEXTUI_DEVICES="$(targets_for nextui | while IFS= read -r t; do target_device "$t"; done)"

# ---------------------------------------------------------------- single pak
ZIP_LIST="$(unzip -Z1 "$PAKZIP")"
has_zip() { printf '%s\n' "$ZIP_LIST" | grep -qx "$1"; }

echo "-- $PAKZIP"

# Installed by extracting *into* Tools/<platform>/Itch-io.pak/, so the archive
# must have no wrapper directory. A nested one puts every file a level too deep
# and NextUI shows a pak that does nothing.
if printf '%s\n' "$ZIP_LIST" | grep -q '^Itch-io\.pak/'; then
    fail "no Itch-io.pak/ wrapper directory"
else
    ok "no Itch-io.pak/ wrapper directory"
fi

for want in "launch.sh" "$BIN_NAME" "pak.json" "assets/font.ttf" "assets/ca-certificates.crt"; do
    check "contains $want" has_zip "$want"
done

# Each NextUI device that ships its own SDL2 needs its own pair: launch.sh picks
# the directory at runtime, and a missing one means the app cannot start on that
# hardware. Devices that link the firmware's SDL2 (h700) must have no directory
# at all — shipping one would put our library ahead of the one NextUI installed.
for dev in $NEXTUI_DEVICES; do
    if target_bundles_sdl "nextui/$dev"; then
        for lib in libSDL2-2.0.so.0 libSDL2_ttf-2.0.so.0; do
            check "carries lib/$dev/$lib" has_zip "lib/$dev/$lib"
        done
    else
        if printf '%s\n' "$ZIP_LIST" | grep -q "^lib/$dev/"; then
            fail "ships no lib/$dev/ (firmware provides SDL2)"
        else
            ok "ships no lib/$dev/ (firmware provides SDL2)"
        fi
    fi
done

# muOS packaging must not leak in, and neither should repo or build leftovers.
for unwanted in "mux_launch.sh" "version.txt" "assets/.gitkeep" "glyph/make-glyph.py"; do
    if has_zip "$unwanted"; then
        fail "does not ship $unwanted"
    else
        ok "does not ship $unwanted"
    fi
done

# SD cards are FAT32, which has no symlinks — release.sh dereferences with cp -L
# for exactly this reason. A symlink here unpacks as a broken stub.
if unzip -Zl "$PAKZIP" | awk '$1 ~ /^l/ {found=1} END {exit !found}'; then
    fail "contains no symlinks (FAT32 cannot store them)"
else
    ok "contains no symlinks (FAT32 cannot store them)"
fi

# NextUI runs launch.sh, which execs the binary. Neither works without the bit,
# and the failure is silent.
for exe in "launch.sh" "$BIN_NAME"; do
    MODE="$(unzip -Z "$PAKZIP" "$exe" 2>/dev/null | awk 'NR==1 {print $1}')"
    case "$MODE" in
        -*x*) ok "$exe is executable ($MODE)" ;;
        *)    fail "$exe is not executable (mode $MODE)" ;;
    esac
done

STAMPED="$(unzip -p "$PAKZIP" pak.json | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
check "pak.json says $VERSION (found '$STAMPED')" [ "$STAMPED" = "$VERSION" ]

# ------------------------------------------------------------ multi-device
echo "-- $PAKZ"
PAKZ_LIST="$(unzip -Z1 "$PAKZ")"
has_pakz() { printf '%s\n' "$PAKZ_LIST" | grep -qx "$1"; }

# Extracted at the SD card root, so everything must sit under Tools/.
if printf '%s\n' "$PAKZ_LIST" | grep -qv '^Tools/'; then
    fail "every entry is under Tools/"
    printf '%s\n' "$PAKZ_LIST" | grep -v '^Tools/' | sed 's/^/       unexpected: /'
else
    ok "every entry is under Tools/"
fi

for dev in $NEXTUI_DEVICES; do
    P="Tools/$dev/Itch-io.pak"
    for want in "$P/launch.sh" "$P/$BIN_NAME" "$P/pak.json"; do
        check "contains $want" has_pakz "$want"
    done

    if target_bundles_sdl "nextui/$dev"; then
        check "contains $P/lib/$dev/libSDL2-2.0.so.0" \
            has_pakz "$P/lib/$dev/libSDL2-2.0.so.0"
        # Each platform directory carries only its own libraries; shipping all
        # of them everywhere would multiply the bundle for no benefit.
        # Directory entries are filtered out: "lib/" itself is not under
        # "lib/<device>/" and would otherwise count against every platform.
        OTHER="$(printf '%s\n' "$PAKZ_LIST" | grep "^$P/lib/" | grep -v '/$' \
            | grep -cv "^$P/lib/$dev/" || true)"
        check "Tools/$dev carries only its own lib dir" [ "$OTHER" -eq 0 ]
    else
        # Deliberately do NOT filter out directory-only entries here, unlike the
        # branch above. `zip -r` emits a bare "$P/lib/$dev/" entry ending in "/"
        # when the directory was created but nothing was ever copied into it —
        # exactly the bug this check exists to catch (a stray `mkdir -p` with no
        # matching `cp`). Filtering "/$" out, as the positive branch does, would
        # make ANY report 0 and this check would pass on a real violation.
        ANY="$(printf '%s\n' "$PAKZ_LIST" | grep -c "^$P/lib/" || true)"
        check "Tools/$dev ships no lib dir (firmware provides SDL2)" [ "$ANY" -eq 0 ]
    fi
done

# The unversioned copy is what pak.json's release_filename points the Pak Store
# at. If it ever drifts from the versioned one, the Store installs something
# other than what the release page advertises.
if [ -f "$STORE_COPY" ]; then
    check "Pak Store copy is identical to $VERSION" cmp -s "$STORE_COPY" "$PAKZIP"
else
    echo "note: $STORE_COPY absent (expected for a --prerelease build)" >&2
fi

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]

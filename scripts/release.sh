#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"

# release.sh runs directly on the host.  test.sh and build.sh each manage their
# own containers internally, so no container wrapping is needed here.

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh [--allow-dirty]

Run the full test suite, build every target, and assemble release artifacts.

Output in dist/:
  nextui/Itch-io.NextUI.<version>.pak.zip   NextUI single pak; all lib dirs inside, works on any NextUI device
  nextui/Itch-io.NextUI.<version>.pakz      NextUI multi-device bundle (one dir per platform), extract at SD root
  muos/Itch-io.muOS.<version>.muxapp        muOS application archive, installed via Archive Manager
  Itch-io.pak.zip                           Copy of the .pak.zip under the name the Pak Store expects

Options:
  --allow-dirty   Build with uncommitted changes. The binaries are stamped
                  "<commit>-dirty" and cannot be traced back to a commit, so
                  this is for packaging experiments, never for a release.

Requires: zip (host), docker or podman (managed internally by test.sh and build.sh)

Examples:
  ./scripts/release.sh
  ./scripts/release.sh --allow-dirty
EOF
    exit 0
fi

if ! command -v zip >/dev/null 2>&1; then
    echo "ERROR: zip is required (install it with your package manager)" >&2
    exit 1
fi

# Refuse to package a dirty tree. build.sh stamps the binary with the commit it
# built from, suffixed "-dirty" when the tree has uncommitted changes — so a
# release built this way reports a commit that does not describe it, and a bug
# report quoting that line cannot be traced to any source. Caught the hard way:
# v1.0.23-rc1 first shipped stamped with the commit before the one it was tagged at.
if [ "${1:-}" != "--allow-dirty" ] && command -v git >/dev/null 2>&1; then
    if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
        echo "ERROR: working tree has uncommitted changes." >&2
        echo "       Release binaries record the commit they were built from, and this" >&2
        echo "       build would be stamped '-dirty' and point at the wrong commit." >&2
        echo "       Commit first, or pass --allow-dirty for a packaging experiment." >&2
        exit 1
    fi
fi

VERSION="$(pak_version)"
if [ -z "$VERSION" ]; then
    echo "ERROR: could not read .version from pak.json" >&2
    exit 1
fi

echo "==> Running tests..."
./scripts/test.sh

./scripts/build.sh all

echo "==> Assembling release artifacts ($VERSION)..."
rm -rf dist
mkdir -p dist/nextui

# --- NextUI -----------------------------------------------------------------
# The shipped pak keeps NextUI's own vocabulary: launch.sh, pak.json, and a
# lib/<device>/ tree that launch.sh picks from at runtime.

NEXTUI_ZIP="dist/nextui/Itch-io.NextUI.$VERSION.pak.zip"

# Single pak (Pak Store distribution).  Carries every NextUI lib dir so one zip
# installs on any NextUI device; launch.sh selects the right one at runtime.
# The tg5040 binary is used for all of them — see scripts/targets.sh.
#
# This directory is left unzipped on purpose: deploy.sh pushes it to a device.
PAK_DIR="dist/nextui/Itch-io.pak"
mkdir -p "$PAK_DIR/assets"

cp "$(target_binary nextui/tg5040)" "$PAK_DIR/$BIN_NAME"
cp launch.sh                        "$PAK_DIR/launch.sh"
cp pak.json                         "$PAK_DIR/pak.json"
cp -r assets/.                      "$PAK_DIR/assets/"

# Dereference symlinks (-L) so FAT32 SD cards get real files instead of symlinks.
for t in $(targets_for nextui); do
    dev="$(target_device "$t")"
    target_bundles_sdl "$t" || continue
    mkdir -p "$PAK_DIR/lib/$dev"
    cp -L "$(toolchain_libdir "$(target_toolchain "$t")")"/* "$PAK_DIR/lib/$dev/" 2>/dev/null || true
done

echo "==> Creating release archives..."
(
    cd "$PAK_DIR"
    zip -qr "../Itch-io.NextUI.$VERSION.pak.zip" .
) &
pid_zip1=$!

# Multi-device bundle (.pakz) for manual SD card installation.
# Each platform directory gets its own binary and only its own lib dir.
mkdir -p dist/nextui/all/Tools
for t in $(targets_for nextui); do
    dev="$(target_device "$t")"
    PLAT_PAK="dist/nextui/all/Tools/$dev/Itch-io.pak"
    mkdir -p "$PLAT_PAK/lib/$dev" "$PLAT_PAK/assets"
    cp "$(target_binary "$t")" "$PLAT_PAK/$BIN_NAME"
    cp launch.sh               "$PLAT_PAK/launch.sh"
    cp pak.json                "$PLAT_PAK/pak.json"
    cp -r assets/.             "$PLAT_PAK/assets/"
    cp -L "$(toolchain_libdir "$(target_toolchain "$t")")"/* "$PLAT_PAK/lib/$dev/" 2>/dev/null || true
done

(
    cd dist/nextui/all
    zip -qr "../Itch-io.NextUI.$VERSION.pakz" Tools
) &
pid_zip2=$!

wait $pid_zip1
wait $pid_zip2

# The Pak Store fetches pak.json's release_filename verbatim, so keep a copy
# under the unversioned name it expects.
cp "$NEXTUI_ZIP" dist/Itch-io.pak.zip

# --- muOS ------------------------------------------------------------------
# A .muxapp is a plain zip whose root is the application directory; muOS's
# Archive Manager extracts it into the application store. No pak.json and no
# launch.sh: both are NextUI packaging, and mean nothing here. No SDL2 either —
# muOS ships its own patched build and that is the one to link against.

mkdir -p dist/muos
MUOS_APP="dist/muos/Itch-io"
rm -rf "$MUOS_APP"
mkdir -p "$MUOS_APP/assets"

cp "$(target_binary muos/arm64)" "$MUOS_APP/$BIN_NAME"
cp packaging/muos/mux_launch.sh  "$MUOS_APP/mux_launch.sh"
cp packaging/muos/mux_lang.ini   "$MUOS_APP/mux_lang.ini"
cp -r packaging/muos/glyph       "$MUOS_APP/glyph"
cp -r assets/.                   "$MUOS_APP/assets/"

# Firmware-neutral version marker, so the artifact can be identified on device
# and by the release tooling without borrowing NextUI's manifest.
printf '%s\n' "$VERSION" > "$MUOS_APP/version.txt"

# muOS runs mux_launch.sh directly; without the exec bit the app simply does
# not start, and zip is what has to carry that through.
chmod +x "$MUOS_APP/mux_launch.sh" "$MUOS_APP/$BIN_NAME"

(
    cd dist/muos
    zip -qr "Itch-io.muOS.$VERSION.muxapp" Itch-io
)

echo "==> Release artifacts:"
find dist -maxdepth 2 -type f \( -name '*.zip' -o -name '*.pakz' -o -name '*.muxapp' \) | sort

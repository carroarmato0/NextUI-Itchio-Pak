#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"

# release.sh runs directly on the host.  test.sh and build.sh each manage their
# own containers internally, so no container wrapping is needed here.

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh

Run the full test suite, build every target, and assemble release artifacts.

Output in dist/:
  nextui/Itch-io.NextUI.<version>.pak.zip   NextUI single pak; all lib dirs inside, works on any NextUI device
  nextui/Itch-io.NextUI.<version>.pakz      NextUI multi-device bundle (one dir per platform), extract at SD root
  Itch-io.pak.zip                           Copy of the .pak.zip under the name the Pak Store expects

Requires: zip (host), docker or podman (managed internally by test.sh and build.sh)

Examples:
  ./scripts/release.sh
EOF
    exit 0
fi

if ! command -v zip >/dev/null 2>&1; then
    echo "ERROR: zip is required (install it with your package manager)" >&2
    exit 1
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

echo "==> Release artifacts:"
find dist -maxdepth 2 -type f \( -name '*.zip' -o -name '*.pakz' -o -name '*.muxapp' \) | sort

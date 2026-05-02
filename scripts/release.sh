#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

# release.sh runs directly on the host.  test.sh and build.sh each manage their
# own containers internally, so no container wrapping is needed here.

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: release.sh

Run the full test suite, build all platforms, and assemble release artifacts.

Output in dist/:
  Itch-io.pak.zip   Single-zip for Pak Store; all three lib dirs inside, works on any device
  Itch-io.pakz      Multi-device bundle for manual SD card install (one dir per platform)

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

echo "==> Running tests..."
./scripts/test.sh

./scripts/build.sh all

echo "==> Assembling release artifacts..."
rm -rf dist

# Single-device zip (Pak Store distribution).
# Contains all three platform lib dirs so one zip installs on any device;
# launch.sh selects the right lib dir at runtime.
PAK_DIR="dist/Itch-io.pak"
mkdir -p "$PAK_DIR/lib/tg5040" "$PAK_DIR/lib/tg5050" "$PAK_DIR/lib/my355" "$PAK_DIR/assets"

cp bin/tg5040/itchio-pak "$PAK_DIR/itchio-pak"
cp launch.sh             "$PAK_DIR/launch.sh"
cp pak.json              "$PAK_DIR/pak.json"
cp -r assets/.           "$PAK_DIR/assets/"

# Dereference symlinks (-L) so FAT32 SD cards get real files instead of symlinks.
cp -L lib/tg5040/* "$PAK_DIR/lib/tg5040/" 2>/dev/null || true
cp -L lib/tg5050/* "$PAK_DIR/lib/tg5050/" 2>/dev/null || true
cp -L lib/my355/*  "$PAK_DIR/lib/my355/"  2>/dev/null || true

# Assemble both artifacts in parallel.
echo "==> Creating release zips..."
(
    cd "$PAK_DIR"
    zip -qr ../Itch-io.pak.zip .
) &
pid_zip1=$!

# Multi-device bundle (.pakz) for manual SD card installation.
# Each platform directory gets its own binary and only its own lib dir.
mkdir -p dist/all/Tools
for PLATFORM in tg5040 tg5050 my355; do
    PLAT_PAK="dist/all/Tools/$PLATFORM/Itch-io.pak"
    mkdir -p "$PLAT_PAK/lib/$PLATFORM" "$PLAT_PAK/assets"
    cp "bin/$PLATFORM/itchio-pak" "$PLAT_PAK/itchio-pak"
    cp launch.sh                  "$PLAT_PAK/launch.sh"
    cp pak.json                   "$PLAT_PAK/pak.json"
    cp -r assets/.                "$PLAT_PAK/assets/"
    cp -L lib/$PLATFORM/*         "$PLAT_PAK/lib/$PLATFORM/" 2>/dev/null || true
done

(
    cd dist/all
    zip -qr ../Itch-io.pakz Tools
) &
pid_zip2=$!

wait $pid_zip1
wait $pid_zip2

echo "==> Release artifacts:"
find dist -name "*.zip" -o -name "*.pakz" | sort

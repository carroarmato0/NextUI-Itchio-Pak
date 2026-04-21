#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

IMAGE="itchio-pak-dev"
RUNTIME="$(detect_runtime)"
if [ -z "$RUNTIME" ]; then
    echo "ERROR: docker or podman required" >&2; exit 1
fi

if [ -z "${IN_CONTAINER:-}" ]; then
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        -e CONTAINER_RUNTIME="$RUNTIME" \
        "$IMAGE" "$0" "$@"
fi

echo "==> Running tests..."
./scripts/test.sh

echo "==> Building all platforms..."
./scripts/build.sh all

echo "==> Assembling release artifacts..."
rm -rf dist

# All platforms share the same ARM64 binary; only the bundled SDL2 libs differ.
# Ship one zip with platform libs in subdirectories; launch.sh detects the device.
PAK_DIR="dist/Itch-io.pak"
mkdir -p "$PAK_DIR/lib/tg5040" "$PAK_DIR/lib/my355" "$PAK_DIR/assets"

cp bin/tg5040/itchio-pak "$PAK_DIR/itchio-pak"
cp launch.sh             "$PAK_DIR/launch.sh"
cp pak.json              "$PAK_DIR/pak.json"
cp -r assets/.           "$PAK_DIR/assets/"

# Copy platform SDL2 libs, dereferencing symlinks (-L) so FAT32 gets real files.
# tg5040 libs also cover tg5050 (same hardware family).
cp -L lib/tg5040/* "$PAK_DIR/lib/tg5040/" 2>/dev/null || true
cp -L lib/my355/*  "$PAK_DIR/lib/my355/"  2>/dev/null || true

# Single zip for the Pak Store — no top-level folder inside the zip because the
# Pak Store creates the destination folder itself before extracting.
cd "$PAK_DIR"
zip -r ../Itch-io.pak.zip .
cd - >/dev/null

# .pakz for manual SD card installation (extract to SD root).
mkdir -p dist/all/Tools
for PLATFORM in tg5040 tg5050 my355; do
    mkdir -p "dist/all/Tools/$PLATFORM"
    cp -r "$PAK_DIR" "dist/all/Tools/$PLATFORM/Itch-io.pak"
done
cd dist/all
zip -r ../Itch-io.pakz Tools
cd - >/dev/null

echo "==> Release artifacts:"
find dist -name "*.zip" -o -name "*.pakz" | sort

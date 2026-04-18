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
mkdir -p dist/all/Tools

for PLATFORM in tg5040 tg5050 my355; do
    PAK_DIR="dist/$PLATFORM/Itch.io.pak"
    mkdir -p "$PAK_DIR/lib" "$PAK_DIR/assets"

    cp bin/"$PLATFORM"/itchio-pak "$PAK_DIR/itchio-pak"
    cp launch.sh "$PAK_DIR/launch.sh"
    cp pak.json "$PAK_DIR/pak.json"
    cp -r assets/. "$PAK_DIR/assets/"

    # Copy platform SDL2 libs
    case "$PLATFORM" in
        tg5040|tg5050) cp lib/tg5040/. "$PAK_DIR/lib/" 2>/dev/null || true ;;
        my355)          cp lib/my355/.  "$PAK_DIR/lib/" 2>/dev/null || true ;;
    esac

    cd dist/"$PLATFORM"
    zip -r ../Itch.io.pak.zip Itch.io.pak
    cd - >/dev/null

    # Also copy into .pakz structure
    cp -r "$PAK_DIR" "dist/all/Tools/$PLATFORM/Itch.io.pak"
done

mkdir -p dist/all
cd dist/all
zip -r ../all/Itch.io.pakz Tools
cd - >/dev/null

echo "==> Release artifacts:"
find dist -name "*.zip" -o -name "*.pakz" | sort

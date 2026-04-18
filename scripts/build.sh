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

RUNTIME_OVERRIDE=""
TARGET=""
while [ $# -gt 0 ]; do
    case "$1" in
        --runtime) RUNTIME_OVERRIDE="$2"; shift 2 ;;
        *) TARGET="$1"; shift ;;
    esac
done

if [ -z "$TARGET" ]; then
    echo "Usage: build.sh [--runtime docker|podman] native|tg5040|tg5050|my355|all" >&2
    exit 1
fi

RUNTIME="${RUNTIME_OVERRIDE:-$(detect_runtime)}"
if [ -z "$RUNTIME" ]; then
    echo "ERROR: docker or podman required" >&2; exit 1
fi

build_native() {
    IMAGE="itchio-pak-dev"
    if [ -z "${IN_CONTAINER:-}" ]; then
        $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
            $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            "$IMAGE" "$0" native
    fi
    mkdir -p bin/native
    go build -o bin/native/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/native/itchio-pak"
}

build_platform() {
    PLATFORM="$1"
    case "$PLATFORM" in
        tg5040) IMAGE="ghcr.io/loveretro/tg5040-toolchain:latest" ;;
        tg5050) IMAGE="ghcr.io/loveretro/tg5050-toolchain:latest" ;;
        my355)  IMAGE="ghcr.io/loveretro/my355-toolchain:latest" ;;
        *) echo "Unknown platform: $PLATFORM" >&2; exit 1 ;;
    esac

    if [ -z "${IN_CONTAINER:-}" ]; then
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            "$IMAGE" "$0" --runtime "$RUNTIME" "$PLATFORM"
    fi

    mkdir -p bin/"$PLATFORM"
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
        go build -o bin/"$PLATFORM"/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/$PLATFORM/itchio-pak"
}

case "$TARGET" in
    native) build_native ;;
    tg5040|tg5050|my355) build_platform "$TARGET" ;;
    all)
        build_platform tg5040
        build_platform tg5050
        build_platform my355
        ;;
    *) echo "Unknown target: $TARGET" >&2; exit 1 ;;
esac

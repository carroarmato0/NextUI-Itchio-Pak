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

# Only need a container runtime when launching from the host.
RUNTIME=""
if [ -z "${IN_CONTAINER:-}" ]; then
    RUNTIME="${RUNTIME_OVERRIDE:-$(detect_runtime)}"
    if [ -z "$RUNTIME" ]; then
        echo "ERROR: docker or podman required" >&2; exit 1
    fi
fi

IMAGE="itchio-pak-dev"

# Ensure the dev image exists (builds it if missing).
ensure_image() {
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
}

pak_version() {
    grep '"version"' pak.json | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

build_native() {
    if [ -z "${IN_CONTAINER:-}" ]; then
        ensure_image
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            "$IMAGE" "$0" native
    fi
    VERSION="$(pak_version)"
    mkdir -p bin/native
    go build -ldflags "-X main.version=$VERSION -X github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=$VERSION" \
        -o bin/native/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/native/itchio-pak ($VERSION)"
}

build_platform() {
    PLATFORM="$1"

    mkdir -p bin/"$PLATFORM" lib/tg5040 lib/my355 assets

    # Copy the Debian CA bundle into assets/ so launch.sh can point SSL_CERT_FILE
    # at it — the device has no system CA store, so HTTPS would fail otherwise.
    cp /etc/ssl/certs/ca-certificates.crt assets/ca-certificates.crt 2>/dev/null || true

    # Cross-compile for ARM64 using the aarch64-linux-gnu toolchain installed in
    # the dev image. PKG_CONFIG_PATH points at the arm64 SDL2 .pc files so that
    # the go-sdl2 cgo directives pick up the right library paths.
    # -tags netgo: use Go's built-in DNS resolver instead of the CGo one.
    #   Without this, the CGo net resolver links res_search@GLIBC_2.34 which
    #   is unavailable on TrimUI/Miyoo devices that ship glibc 2.33.
    # -a: force recompile of all packages; without this, cached .a files from
    #   a previous toolchain/image can embed GLIBC_2.34 symbol version tags.
    VERSION="$(pak_version)"
    PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig \
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
        go build -a -tags netgo -buildvcs=false \
        -ldflags "-X main.version=$VERSION -X github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=$VERSION" \
        -o bin/"$PLATFORM"/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/$PLATFORM/itchio-pak ($VERSION)"

    # Extract SDL2 shared libs for bundling inside the pak.
    ARM64_LIB=/usr/lib/aarch64-linux-gnu
    case "$PLATFORM" in
        tg5040|tg5050)
            cp -P "$ARM64_LIB"/libSDL2-2.0.so.0*    lib/tg5040/ 2>/dev/null || true
            cp -P "$ARM64_LIB"/libSDL2_ttf-2.0.so.0* lib/tg5040/ 2>/dev/null || true
            ;;
        my355)
            cp -P "$ARM64_LIB"/libSDL2-2.0.so.0*    lib/my355/ 2>/dev/null || true
            cp -P "$ARM64_LIB"/libSDL2_ttf-2.0.so.0* lib/my355/ 2>/dev/null || true
            ;;
    esac
}

case "$TARGET" in
    native)
        build_native
        ;;
    tg5040|tg5050|my355)
        # Single-platform build: launch container then run inside it.
        if [ -z "${IN_CONTAINER:-}" ]; then
            ensure_image
            exec $RUNTIME run --rm \
                -v "$(pwd):/workspace" \
                -w /workspace \
                -e IN_CONTAINER=1 \
                "$IMAGE" "$0" "$TARGET"
        fi
        build_platform "$TARGET"
        ;;
    all)
        # All three ARM64 targets: launch ONE container and build all inside it
        # so we don't exec three times (exec replaces the process).
        if [ -z "${IN_CONTAINER:-}" ]; then
            ensure_image
            exec $RUNTIME run --rm \
                -v "$(pwd):/workspace" \
                -w /workspace \
                -e IN_CONTAINER=1 \
                "$IMAGE" "$0" all
        fi
        build_platform tg5040
        build_platform tg5050
        build_platform my355
        ;;
    *)
        echo "Unknown target: $TARGET" >&2; exit 1
        ;;
esac

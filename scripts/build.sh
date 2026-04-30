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
        --help|-h)
            cat <<'EOF'
Usage: build.sh [--runtime docker|podman] <target>

Targets:
  native        Build for the host machine (x86_64, runs inside dev container)
  tg5040        Cross-compile for TrimUI Brick / Smart Pro (ARM64)
  tg5050        Cross-compile for TrimUI Smart Pro S (ARM64)
  my355         Cross-compile for Miyoo Flip (ARM64)
  all           Cross-compile for all three device platforms sequentially

Options:
  --runtime docker|podman   Override container runtime (default: auto-detect, prefers podman)

Environment:
  CONTAINER_RUNTIME=docker|podman   Alternative to --runtime

Output: bin/<target>/itchio-pak

Examples:
  ./scripts/build.sh tg5040
  ./scripts/build.sh all
  ./scripts/build.sh --runtime docker tg5050
  CONTAINER_RUNTIME=docker ./scripts/build.sh native
EOF
            exit 0
            ;;
        --runtime) RUNTIME_OVERRIDE="$2"; shift 2 ;;
        *) TARGET="$1"; shift ;;
    esac
done

if [ -z "$TARGET" ]; then
    echo "Usage: build.sh [--runtime docker|podman] native|tg5040|tg5050|my355|all" >&2
    echo "       build.sh --help for full usage" >&2
    exit 1
fi

# Only need a container runtime when launching from the host.
RUNTIME=""
GIT_COMMIT="${GIT_COMMIT:-}"
if [ -z "${IN_CONTAINER:-}" ]; then
    RUNTIME="${RUNTIME_OVERRIDE:-$(detect_runtime)}"
    if [ -z "$RUNTIME" ]; then
        echo "ERROR: docker or podman required" >&2; exit 1
    fi

    # Extract git commit info on the host.
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    if [ "$GIT_COMMIT" != "unknown" ]; then
        if ! git diff --quiet 2>/dev/null; then
            GIT_COMMIT="${GIT_COMMIT}-dirty"
        fi
    fi
fi

DEV_IMAGE="itchio-pak-dev"

# Ensure the dev image exists (used for native builds and tests).
ensure_dev_image() {
    $RUNTIME image inspect "$DEV_IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$DEV_IMAGE" -f docker/Dockerfile.dev .
}

# Ensure the per-platform image exists (LoveRetro toolchain + Go).
ensure_platform_image() {
    PLATFORM="$1"
    TAG="itchio-pak-$PLATFORM-dev"
    $RUNTIME image inspect "$TAG" >/dev/null 2>&1 || \
        $RUNTIME build -t "$TAG" --build-arg "PLATFORM=$PLATFORM" \
            -f docker/Dockerfile.platform .
}

pak_version() {
    grep '"version"' pak.json | sed 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/'
}

build_native() {
    if [ -z "${IN_CONTAINER:-}" ]; then
        ensure_dev_image
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            -e GIT_COMMIT="$GIT_COMMIT" \
            "$DEV_IMAGE" "$0" native
    fi
    VERSION="$(pak_version)"
    COMMIT="${GIT_COMMIT:-unknown}"
    mkdir -p bin/native
    go build -ldflags "-X 'main.version=$VERSION' -X 'main.gitCommit=$COMMIT' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=$VERSION'" \
        -o bin/native/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/native/itchio-pak ($VERSION / $COMMIT)"
}

build_platform() {
    PLATFORM="$1"

    mkdir -p bin/"$PLATFORM" lib/"$PLATFORM" assets

    # Copy CA bundle into assets/ so SSL_CERT_FILE works on devices without a
    # system certificate store.
    cp /etc/ssl/certs/ca-certificates.crt assets/ca-certificates.crt 2>/dev/null || true

    # CC, PKG_CONFIG_PATH, and SYSROOT are set by the LoveRetro toolchain image.
    # -a:        force recompile; cached .a files from a different toolchain/image
    #            can embed GLIBC symbol versions that the device's libc lacks.
    # -tags netgo: use Go's built-in DNS resolver to avoid linking res_search
    #            which requires GLIBC_2.34 (devices ship glibc 2.33).
    VERSION="$(pak_version)"
    COMMIT="${GIT_COMMIT:-unknown}"
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
        go build -a -tags netgo -buildvcs=false \
        -ldflags "-X 'main.version=$VERSION' -X 'main.gitCommit=$COMMIT' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=$VERSION'" \
        -o bin/"$PLATFORM"/itchio-pak ./cmd/itchio-pak/
    echo "Built: bin/$PLATFORM/itchio-pak ($VERSION / $COMMIT)"

    # Bundle SDL2 .so files from the LoveRetro sysroot.  These are compiled
    # without X11 / PulseAudio / Wayland so they work on embedded devices.
    # Copy only the real versioned file under the SONAME name so the zip ships
    # a single file per library rather than a symlink + versioned duplicate.
    rm -f lib/"$PLATFORM"/libSDL2*.so*
    SDL2_SO=$(ls "$SYSROOT/usr/lib"/libSDL2-2.0.so.0.* 2>/dev/null | grep -v '\.so$' | head -1)
    SDL2_TTF_SO=$(ls "$SYSROOT/usr/lib"/libSDL2_ttf-2.0.so.0.* 2>/dev/null | grep -v '\.so$' | head -1)
    [ -n "$SDL2_SO" ]     && cp "$SDL2_SO"     lib/"$PLATFORM"/libSDL2-2.0.so.0
    [ -n "$SDL2_TTF_SO" ] && cp "$SDL2_TTF_SO" lib/"$PLATFORM"/libSDL2_ttf-2.0.so.0
}

case "$TARGET" in
    native)
        build_native
        ;;
    tg5040|tg5050|my355)
        if [ -z "${IN_CONTAINER:-}" ]; then
            ensure_platform_image "$TARGET"
            exec $RUNTIME run --rm \
                -v "$(pwd):/workspace" \
                -w /workspace \
                -e IN_CONTAINER=1 \
                -e GIT_COMMIT="$GIT_COMMIT" \
                "itchio-pak-$TARGET-dev" "$0" "$TARGET"
        fi
        build_platform "$TARGET"
        ;;
    all)
        # Each platform needs its own toolchain container; run them sequentially.
        if [ -n "${IN_CONTAINER:-}" ]; then
            echo "ERROR: 'build.sh all' must be run from the host, not inside a container." >&2
            exit 1
        fi
        for p in tg5040 tg5050 my355; do
            ensure_platform_image "$p"
            $RUNTIME run --rm \
                -v "$(pwd):/workspace" \
                -w /workspace \
                -e IN_CONTAINER=1 \
                -e GIT_COMMIT="$GIT_COMMIT" \
                "itchio-pak-$p-dev" "$0" "$p"
        done
        ;;
    *)
        echo "Unknown target: $TARGET" >&2; exit 1
        ;;
esac

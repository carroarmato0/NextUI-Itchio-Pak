#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

usage() {
    cat <<EOF
Usage: build.sh [--runtime docker|podman] <target>

Targets:
$(printf '  %-18s %s\n' native "Build for the host machine (x86_64, runs inside dev container)"
  all_targets | while read -r t; do
    printf '  %-18s %s\n' "$t" "$(target_description "$t")"
  done
  all_firmwares | while read -r f; do
    printf '  %-18s Every %s target\n' "$f" "$f"
  done
  printf '  %-18s %s\n' all "Every target in scripts/targets.sh")

Options:
  --runtime docker|podman   Override container runtime (default: auto-detect, prefers podman)

Environment:
  CONTAINER_RUNTIME=docker|podman   Alternative to --runtime

Output: bin/<firmware>/<device>/$BIN_NAME

Examples:
  ./scripts/build.sh nextui/tg5040
  ./scripts/build.sh muos
  ./scripts/build.sh all
  ./scripts/build.sh --runtime docker nextui/tg5050
EOF
}

# Human-readable blurb per target, for --help only.
target_description() {
    case "$1" in
        nextui/tg5040) echo "NextUI — TrimUI Brick / Smart Pro (ARM64)" ;;
        nextui/tg5050) echo "NextUI — TrimUI Smart Pro S (ARM64)" ;;
        nextui/my355)  echo "NextUI — Miyoo Flip (ARM64)" ;;
        muos/arm64)    echo "muOS — every supported device (ARM64)" ;;
        *)             echo "" ;;
    esac
}

RUNTIME_OVERRIDE=""
TARGET=""
while [ $# -gt 0 ]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --runtime) RUNTIME_OVERRIDE="$2"; shift 2 ;;
        *) TARGET="$1"; shift ;;
    esac
done

if [ -z "$TARGET" ]; then
    echo "Usage: build.sh [--runtime docker|podman] native|<firmware>/<device>|<firmware>|all" >&2
    echo "       build.sh --help for full usage" >&2
    exit 1
fi

# Only need a container runtime when launching from the host.
RUNTIME=""
GIT_COMMIT="${GIT_COMMIT:-}"
CACHE_DIR="$(pwd)/.go_cache"
if [ -z "${IN_CONTAINER:-}" ]; then
    mkdir -p "$CACHE_DIR"
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

DEV_IMAGE="itchio-dev"

# Ensure the dev image exists (used for native builds and tests).
ensure_dev_image() {
    $RUNTIME image inspect "$DEV_IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$DEV_IMAGE" -f docker/Dockerfile.dev .
}

# Ensure the toolchain image exists (LoveRetro toolchain + Go).
ensure_toolchain_image() {
    TOOLCHAIN="$1"
    TAG="itchio-toolchain-$TOOLCHAIN"
    $RUNTIME image inspect "$TAG" >/dev/null 2>&1 || \
        $RUNTIME build -t "$TAG" --build-arg "TOOLCHAIN=$TOOLCHAIN" \
            -f docker/Dockerfile.toolchain .
}

LDFLAGS_FOR() {
    printf -- "-X 'main.version=%s' -X 'main.gitCommit=%s' -X 'github.com/carroarmato0/nextui-itchio-pak/internal/ui.appVersion=%s'" \
        "$1" "$2" "$1"
}

# Fail the build if a target's binary needs a newer glibc than its manifest
# ceiling allows.  Only targets whose binary runs somewhere other than the
# device it was built for declare a ceiling; the rest are unconstrained.
#
# Skipped with a warning when no readelf is available rather than silently
# passing, so a toolchain image change cannot quietly drop the check.
assert_glibc_ceiling() {
    TGT="$1"
    BIN="$2"
    CEILING="$(target_max_glibc "$TGT")"
    [ -n "$CEILING" ] || return 0

    READELF="$(command -v readelf 2>/dev/null || true)"
    if [ -z "$READELF" ]; then
        echo "WARNING: readelf not found; skipped the GLIBC_$CEILING check on $BIN" >&2
        return 0
    fi
    HIGHEST=$($READELF --dyn-syms -W "$BIN" 2>/dev/null \
        | grep -o 'GLIBC_[0-9][0-9.]*' | sed 's/GLIBC_//' | sort -uV | tail -1)
    [ -n "$HIGHEST" ] || return 0
    if [ "$(printf '%s\n%s\n' "$CEILING" "$HIGHEST" | sort -V | tail -1)" != "$CEILING" ]; then
        echo "ERROR: $BIN requires GLIBC_$HIGHEST, above $TGT's GLIBC_$CEILING ceiling." >&2
        echo "       This binary has to run on devices with an older libc, and glibc" >&2
        echo "       is only forward compatible.  See scripts/targets.sh." >&2
        exit 1
    fi
}

build_native() {
    if [ -z "${IN_CONTAINER:-}" ]; then
        ensure_dev_image
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -v "$CACHE_DIR:/go" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            -e GOCACHE=/go/build-cache \
            -e GIT_COMMIT="$GIT_COMMIT" \
            "$DEV_IMAGE" "$0" native
    fi
    VERSION="$(pak_version)"
    COMMIT="${GIT_COMMIT:-unknown}"
    mkdir -p bin/host/native
    # shellcheck disable=SC2046  # ldflags must word-split into separate args
    go build -ldflags "$(LDFLAGS_FOR "$VERSION" "$COMMIT")" \
        -o "bin/host/native/$BIN_NAME" ./cmd/itchio-pak/
    echo "Built: bin/host/native/$BIN_NAME ($VERSION / $COMMIT)"
}

build_target() {
    TGT="$1"
    TOOLCHAIN="$(target_toolchain "$TGT")"
    OUT="$(target_binary "$TGT")"
    LIBDIR="$(toolchain_libdir "$TOOLCHAIN")"

    mkdir -p "$(target_bindir "$TGT")" "$LIBDIR" assets

    # Copy CA bundle into assets/ so SSL_CERT_FILE works on devices without a
    # system certificate store.
    cp /etc/ssl/certs/ca-certificates.crt assets/ca-certificates.crt 2>/dev/null || true

    # CC, PKG_CONFIG_PATH, and SYSROOT are set by the LoveRetro toolchain image.
    # -a:        force recompile; cached .a files from a different toolchain/image
    #            can embed GLIBC symbol versions that the device's libc lacks.
    # -tags netgo: use Go's built-in DNS resolver to avoid linking res_search
    #            which requires GLIBC_2.34 (NextUI devices ship glibc 2.33).
    VERSION="$(pak_version)"
    COMMIT="${GIT_COMMIT:-unknown}"
    CGO_ENABLED=1 GOOS=linux GOARCH=arm64 \
        go build -a -tags netgo -buildvcs=false \
        -ldflags "$(LDFLAGS_FOR "$VERSION" "$COMMIT")" \
        -o "$OUT" ./cmd/itchio-pak/

    assert_glibc_ceiling "$TGT" "$OUT"
    echo "Built: $OUT ($VERSION / $COMMIT)"

    # Bundle SDL2 .so files from the LoveRetro sysroot.  These are compiled
    # without X11 / PulseAudio / Wayland so they work on embedded devices.
    # Copy only the real versioned file under the SONAME name so the zip ships
    # a single file per library rather than a symlink + versioned duplicate.
    rm -f "$LIBDIR"/libSDL2*.so*
    SDL2_SO=$(ls "$SYSROOT/usr/lib"/libSDL2-2.0.so.0.* 2>/dev/null | grep -v '\.so$' | head -1)
    SDL2_TTF_SO=$(ls "$SYSROOT/usr/lib"/libSDL2_ttf-2.0.so.0.* 2>/dev/null | grep -v '\.so$' | head -1)
    [ -n "$SDL2_SO" ]     && cp "$SDL2_SO"     "$LIBDIR/libSDL2-2.0.so.0"
    [ -n "$SDL2_TTF_SO" ] && cp "$SDL2_TTF_SO" "$LIBDIR/libSDL2_ttf-2.0.so.0"
    return 0
}

# Targets whose manifest row names a source copy the binary instead of
# compiling it.  See the note in scripts/targets.sh.
copy_target() {
    TGT="$1"
    SRC="$(target_source "$TGT")"
    SRC_BIN="$(target_binary "$SRC")"

    if [ ! -f "$SRC_BIN" ]; then
        echo "ERROR: $TGT copies $SRC but $SRC_BIN does not exist — build $SRC first." >&2
        exit 1
    fi
    mkdir -p "$(target_bindir "$TGT")"
    cp "$SRC_BIN" "$(target_binary "$TGT")"
    # Re-check against this target's own ceiling: the copy has to satisfy the
    # firmware it is going to, which may be stricter than its source's.
    assert_glibc_ceiling "$TGT" "$(target_binary "$TGT")"
    echo "Built: $(target_binary "$TGT") (copied from $SRC)"
}

# Build one target from the host, entering its toolchain container if needed.
run_one() {
    TGT="$1"
    if [ -n "$(target_source "$TGT")" ]; then
        copy_target "$TGT"
        return 0
    fi
    if [ -z "${IN_CONTAINER:-}" ]; then
        TOOLCHAIN="$(target_toolchain "$TGT")"
        ensure_toolchain_image "$TOOLCHAIN"
        exec $RUNTIME run --rm \
            -v "$(pwd):/workspace" \
            -v "$CACHE_DIR:/go" \
            -w /workspace \
            -e IN_CONTAINER=1 \
            -e GOCACHE=/go/build-cache \
            -e GIT_COMMIT="$GIT_COMMIT" \
            "itchio-toolchain-$TOOLCHAIN" "$0" "$TGT"
    fi
    build_target "$TGT"
}

# Build a list of targets: compiled ones in parallel, then the copies.
build_many() {
    LIST="$1"

    if [ -n "${IN_CONTAINER:-}" ]; then
        echo "ERROR: multi-target builds must run from the host, not inside a container." >&2
        exit 1
    fi

    COMPILE=""
    COPY=""
    for t in $LIST; do
        if [ -n "$(target_source "$t")" ]; then COPY="$COPY $t"; else COMPILE="$COMPILE $t"; fi
    done

    # Ensure images exist first (sequentially, to avoid racing the builder).
    for t in $COMPILE; do
        ensure_toolchain_image "$(target_toolchain "$t")"
    done

    echo "==> Building:$COMPILE"
    pids=""
    for t in $COMPILE; do
        log="$(target_bindir "$t")/build.log"
        mkdir -p "$(target_bindir "$t")"
        ( "$0" "$t" >"$log" 2>&1 && grep "Built:" "$log" ) &
        pids="$pids $!:$t"
    done

    if [ -t 1 ]; then
        # ASCII spinner: this script runs under plain sh, where slicing a
        # multibyte string a character at a time is not portable.
        spinner='|/-\'
        i=0
        while :; do
            running=""
            for entry in $pids; do
                pid=${entry%%:*}; name=${entry#*:}
                kill -0 "$pid" 2>/dev/null && running="$running ${name#*/}"
            done
            [ -z "$running" ] && break
            printf "\r  %s Building [%s]..." \
                "$(printf '%s' "$spinner" | cut -c $((i % 4 + 1)))" "${running# }"
            i=$((i + 1))
            sleep 0.1
        done
        printf "\r\033[K"
    fi

    failed=""
    for entry in $pids; do
        pid=${entry%%:*}; name=${entry#*:}
        wait "$pid" || failed="$failed $name"
    done

    if [ -n "$failed" ]; then
        echo "ERROR: build failed for:$failed" >&2
        for t in $failed; do
            echo "--- Log for $t ---" >&2
            cat "$(target_bindir "$t")/build.log" >&2
        done
        exit 1
    fi

    # Copies run last: their source binary has to exist by now.
    for t in $COPY; do
        "$0" "$t"
    done
}

case "$TARGET" in
    native)
        build_native
        ;;
    all)
        build_many "$(all_targets | tr '\n' ' ')"
        ;;
    */*)
        if ! target_exists "$TARGET"; then
            echo "Unknown target: $TARGET" >&2
            echo "Known targets: $(all_targets | tr '\n' ' ')" >&2
            exit 1
        fi
        run_one "$TARGET"
        ;;
    *)
        if all_firmwares | grep -qx "$TARGET"; then
            build_many "$(targets_for "$TARGET" | tr '\n' ' ')"
        else
            echo "Unknown target: $TARGET" >&2
            echo "Known targets: native, all, $(all_firmwares | tr '\n' ' '), $(all_targets | tr '\n' ' ')" >&2
            exit 1
        fi
        ;;
esac

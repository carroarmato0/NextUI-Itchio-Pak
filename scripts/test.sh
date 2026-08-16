#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: test.sh [--coverage]

Run the full test suite inside a container using the headless build tag (no SDL2).
The dev container (itchio-dev) is built automatically on first run.

Options:
  --coverage    Generate an HTML coverage report at coverage.html

Examples:
  ./scripts/test.sh
  ./scripts/test.sh --coverage
EOF
    exit 0
fi

detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}

IMAGE="itchio-dev"
CACHE_DIR="$(pwd)/.go_cache"
if [ -z "${IN_CONTAINER:-}" ]; then
    # Host-side shell-tooling tests (release scripts). These exercise host tools
    # (jq/git/gh) rather than Go code, so they run here — not in the Go container,
    # which has no jq/unzip. Skipped if jq is unavailable.
    if command -v jq >/dev/null 2>&1; then
        echo "==> release-github_test.sh"
        "$SCRIPT_DIR/release-github_test.sh" || exit 1
    else
        echo "note: skipping release-github_test.sh (jq not found on host)" >&2
    fi

    echo "==> release-pak_test.sh"
    "$SCRIPT_DIR/release-pak_test.sh" || exit 1

    echo "==> release-muxapp_test.sh"
    "$SCRIPT_DIR/release-muxapp_test.sh" || exit 1

    echo "==> launch_test.sh"
    "$SCRIPT_DIR/launch_test.sh" || exit 1

    echo "==> shellcheck (device launchers)"
    if command -v shellcheck >/dev/null 2>&1; then
        # muOS runs mux_launch.sh with its own /bin/sh, so it is checked as
        # POSIX sh rather than bash. launch_test.sh runs under dash too (it
        # execs launch.sh directly), so it belongs in the same POSIX-sh gate
        # it exists to protect.
        shellcheck -s sh packaging/muos/mux_launch.sh launch.sh "$SCRIPT_DIR/launch_test.sh" || exit 1
        echo "ok   - device launch scripts are clean"
    else
        echo "note: skipping shellcheck (not installed)" >&2
    fi

    echo "==> no-color-literals.sh"
    "$SCRIPT_DIR/no-color-literals.sh" || exit 1

    echo "==> palette-audit.sh"
    "$SCRIPT_DIR/palette-audit.sh" || exit 1

    mkdir -p "$CACHE_DIR"
    RUNTIME="$(detect_runtime)"
    if [ -z "$RUNTIME" ]; then
        echo "ERROR: docker or podman required" >&2; exit 1
    fi
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -v "$CACHE_DIR:/go" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        -e GOCACHE=/go/build-cache \
        "$IMAGE" "$0" "$@"
fi

COVER=""
if [ "${1:-}" = "--coverage" ]; then
    COVER="-coverprofile=coverage.out"
fi

# Run tests and format output. column -t ensures the timing aligns even if 
# package names have different lengths.
set +e
TEST_LOG=$(mktemp)
go test -race -tags headless $COVER ./... > "$TEST_LOG" 2>&1
EXIT_CODE=$?
if command -v column >/dev/null 2>&1; then
    column -t "$TEST_LOG"
else
    # Fallback to awk for alignment if column is missing. 
    # Prints field 1 (ok/FAIL), field 2 (package), and field 3 (time/cached).
    awk '{ printf "%-2s  %-65s  %s\n", $1, $2, $3 }' "$TEST_LOG"
fi
rm -f "$TEST_LOG"
set -e

# The run above is tagged headless, so it skips every !headless file in
# internal/ui — screen.go, dev_scenes.go, dev_start.go, and the two dozen
# screen_*.go files, not just input.go. This pass compiles and links that
# whole SDL2 screen package. It needs SDL2 headers, which docker/Dockerfile.dev
# provides, but no display: binding button constants opens no window.
# Without this pass a test can sit in the tree looking green while never
# having been compiled at all.
echo "==> go test ./internal/ui (non-headless)"
go test ./internal/ui/ || exit 1

if [ -n "$COVER" ] && [ $EXIT_CODE -eq 0 ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report: coverage.html"
fi

exit $EXIT_CODE

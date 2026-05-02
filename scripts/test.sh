#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: test.sh [--coverage]

Run the full test suite inside a container using the headless build tag (no SDL2).
The dev container (itchio-pak-dev) is built automatically on first run.

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

IMAGE="itchio-pak-dev"
CACHE_DIR="$(pwd)/.go_cache"
if [ -z "${IN_CONTAINER:-}" ]; then
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

if [ -n "$COVER" ] && [ $EXIT_CODE -eq 0 ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report: coverage.html"
fi

exit $EXIT_CODE

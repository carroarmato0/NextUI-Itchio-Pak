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
if [ -z "${IN_CONTAINER:-}" ]; then
    RUNTIME="$(detect_runtime)"
    if [ -z "$RUNTIME" ]; then
        echo "ERROR: docker or podman required" >&2; exit 1
    fi
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        "$IMAGE" "$0" "$@"
fi

COVER=""
if [ "${1:-}" = "--coverage" ]; then
    COVER="-coverprofile=coverage.out"
fi

go test -race -tags headless $COVER ./...

if [ -n "$COVER" ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report: coverage.html"
fi

#!/bin/sh
# Build, deploy, and screenshot a specific app screen on a connected device.
#
# Workflow:
#   1. Cross-compile for the target platform (skippable with --no-build)
#   2. Push the binary via ADB
#   3. Kill any existing instance
#   4. Launch with DEV_START_SCREEN + LOG_LEVEL=debug
#   5. Poll the device log for a readiness signal
#   6. Wait an extra settle period for the UI to finish rendering
#   7. Capture the framebuffer as a PNG
#   8. Kill the app (unless --keep-alive)
#
# Usage: dev-screenshot.sh [options]
#
# Requires: adb, ffmpeg, docker or podman (for cross-compile step)
# Device must be connected via USB with ADB enabled.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: dev-screenshot.sh [options]

Options:
  --screen <name>       Screen to capture: list, detail, settings  (default: list)
  --out <file>          Output PNG path  (default: screenshot-<screen>.png)
  --no-build            Skip the cross-compile step (use existing bin/)
  --keep-alive          Do not kill the app after capturing
  --settle <seconds>    Extra wait after readiness signal  (default: 1.5)
  --timeout <seconds>   Max wait for readiness signal  (default: 30)

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050|my355   Target platform (default: auto-detect from device)

Supported DEV_START_SCREEN values:
  list      Game list (waits for cache load or first feed page)
  detail    Detail of first game in cache (waits for detail data fetch)
  settings  Settings screen (waits for SDL init)

Examples:
  ./scripts/dev-screenshot.sh
  ./scripts/dev-screenshot.sh --screen detail --out docs/mockups/detail.png
  ./scripts/dev-screenshot.sh --screen settings --no-build --keep-alive
  DEPLOY_PLATFORM=my355 ./scripts/dev-screenshot.sh --screen list
EOF
    exit 0
fi

# ── Parse arguments ───────────────────────────────────────────────────────────
SCREEN="list"
OUT=""
NO_BUILD=0
KEEP_ALIVE=0
SETTLE=1.5
TIMEOUT=30

while [ $# -gt 0 ]; do
    case "$1" in
        --screen)     SCREEN="$2";  shift 2 ;;
        --out)        OUT="$2";     shift 2 ;;
        --no-build)   NO_BUILD=1;   shift   ;;
        --keep-alive) KEEP_ALIVE=1; shift   ;;
        --settle)     SETTLE="$2";  shift 2 ;;
        --timeout)    TIMEOUT="$2"; shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

OUT="${OUT:-screenshot-$SCREEN.png}"

# ── Pre-flight checks ─────────────────────────────────────────────────────────
if ! command -v adb >/dev/null 2>&1; then
    echo "ERROR: adb not found. Install android-tools." >&2; exit 1
fi
if ! adb devices | grep -q "device$"; then
    echo "ERROR: no ADB device connected. Check USB cable and ADB enable in NextUI settings." >&2; exit 1
fi

# ── Platform detection ────────────────────────────────────────────────────────
if [ -n "${DEPLOY_PLATFORM:-}" ]; then
    PLATFORM="$DEPLOY_PLATFORM"
else
    echo "==> Detecting device platform..."
    if adb shell "[ -d /usr/miyoo ]" 2>/dev/null; then
        PLATFORM="my355"
    elif adb shell "grep -q TG5050 /proc/cpuinfo" 2>/dev/null; then
        PLATFORM="tg5050"
    else
        PLATFORM="tg5040"
    fi
    echo "    Detected: $PLATFORM"
fi

PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/logs/itchio-pak.log"

echo "==> Platform: $PLATFORM | Screen: $SCREEN | Output: $OUT"

# ── Build ─────────────────────────────────────────────────────────────────────
if [ "$NO_BUILD" -eq 0 ]; then
    echo "==> Cross-compiling for $PLATFORM..."
    ./scripts/build.sh "$PLATFORM"
fi

# ── Push binary ───────────────────────────────────────────────────────────────
echo "==> Pushing binary to device..."
adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"

# ── Kill any running instance ─────────────────────────────────────────────────
echo "==> Stopping any existing instance..."
adb shell "pkill -f itchio-pak 2>/dev/null; sleep 0.4; true" || true

# Clear the log so we don't match stale entries from a previous run.
adb shell "truncate -s 0 '$LOG_PATH' 2>/dev/null || rm -f '$LOG_PATH' 2>/dev/null; true" || true

# ── Determine readiness log pattern ──────────────────────────────────────────
# Each screen emits a distinctive log line when it is fully visible.
case "$SCREEN" in
    list)     WAIT_PATTERN="cache:" ;;      # "cache: loaded N games" or "cache: serving page"
    detail)   WAIT_PATTERN="detail:" ;;     # "detail: N screenshots"
    settings) WAIT_PATTERN="platform=" ;;   # environment header — settings renders immediately
    *)        WAIT_PATTERN="platform=" ;;
esac

# ── Launch with dev env vars ──────────────────────────────────────────────────
echo "==> Launching app (DEV_START_SCREEN=$SCREEN, LOG_LEVEL=debug)..."
adb shell "nohup sh -c 'DEV_START_SCREEN=$SCREEN LOG_LEVEL=debug $PAK_DEST/launch.sh' \
    > /tmp/pak-dev.log 2>&1 &" || true

# ── Wait for readiness ────────────────────────────────────────────────────────
echo "==> Waiting for screen (pattern: '$WAIT_PATTERN', timeout: ${TIMEOUT}s)..."
DEADLINE=$(($(date +%s) + TIMEOUT))
READY=0
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
    if adb shell "test -f '$LOG_PATH' && grep -q '$WAIT_PATTERN' '$LOG_PATH'" 2>/dev/null; then
        READY=1
        echo "    Ready signal received."
        break
    fi
    sleep 0.3
done

if [ "$READY" -eq 0 ]; then
    echo "WARNING: timed out waiting for '$WAIT_PATTERN' — capturing anyway." >&2
fi

# Extra settle time for SDL to finish rendering the frame.
echo "==> Settling for ${SETTLE}s..."
sleep "$SETTLE"

# ── Capture screenshot ────────────────────────────────────────────────────────
echo "==> Capturing screenshot..."
./scripts/screenshot.sh --platform "$PLATFORM" "$OUT"

# ── Optionally kill the app ───────────────────────────────────────────────────
if [ "$KEEP_ALIVE" -eq 0 ]; then
    adb shell "pkill -f itchio-pak 2>/dev/null; true" || true
fi

echo ""
echo "Screenshot saved: $OUT"
echo "To compare with mockup: open docs/mockups/<screen>.html alongside $OUT"

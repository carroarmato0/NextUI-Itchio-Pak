#!/bin/sh
# Build, deploy, and screenshot one or all app screens on a connected device.
#
# Workflow (per screen):
#   1. Cross-compile for the target platform (skippable with --no-build)
#   2. Push the binary via ADB  (once per run, not repeated for --all)
#   3. Kill any existing instance
#   4. Launch with DEV_START_SCREEN + LOG_LEVEL=debug
#   5. Poll the device log for a readiness signal
#   6. Wait an extra settle period for the UI to finish rendering
#   7. Capture the framebuffer as a PNG
#   8. Kill the app, then repeat for the next screen (--all)
#
# Usage: dev-screenshot.sh [options]
#
# Requires: adb, ffmpeg, docker or podman (for cross-compile step)
# Device must be connected via USB with ADB enabled.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

# ── All supported screens and their readiness patterns ────────────────────────
# Format: "<screen>:<wait_pattern>"
# The wait_pattern is grepped from the device log to detect when the screen
# is fully initialised and rendered.
ALL_SCREENS="list:cache: detail:detail: settings:platform="

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: dev-screenshot.sh [options]

Options:
  --screen <name>       Screen to capture: list, detail, settings  (default: list)
  --all                 Capture every supported screen in sequence
  --out-dir <dir>       Output directory for --all  (default: docs/screenshots)
  --out <file>          Output PNG path for single --screen capture
                        (default: docs/screenshots/<screen>.png)
  --no-build            Skip the cross-compile step (use existing bin/)
  --keep-alive          Do not kill the app after the final capture
  --settle <seconds>    Extra wait after readiness signal  (default: 1.5)
  --timeout <seconds>   Max wait per screen for readiness signal  (default: 30)

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050|my355   Target platform (default: auto-detect)

Supported screens:
  list      Game list (waits for cache load or first feed page)
  detail    Detail of first game in cache (waits for detail data fetch)
  settings  Settings screen (waits for SDL init)

Examples:
  ./scripts/dev-screenshot.sh
  ./scripts/dev-screenshot.sh --all
  ./scripts/dev-screenshot.sh --all --out-dir /tmp/screens --no-build
  ./scripts/dev-screenshot.sh --screen detail --out docs/screenshots/detail.png
  ./scripts/dev-screenshot.sh --screen settings --no-build --keep-alive
  DEPLOY_PLATFORM=my355 ./scripts/dev-screenshot.sh --all
EOF
    exit 0
fi

# ── Parse arguments ───────────────────────────────────────────────────────────
SCREEN="list"
OUT=""
OUT_DIR="docs/screenshots"
CAPTURE_ALL=0
NO_BUILD=0
KEEP_ALIVE=0
SETTLE=1.5
TIMEOUT=30

while [ $# -gt 0 ]; do
    case "$1" in
        --screen)     SCREEN="$2";   shift 2 ;;
        --all)        CAPTURE_ALL=1; shift   ;;
        --out)        OUT="$2";      shift 2 ;;
        --out-dir)    OUT_DIR="$2";  shift 2 ;;
        --no-build)   NO_BUILD=1;    shift   ;;
        --keep-alive) KEEP_ALIVE=1;  shift   ;;
        --settle)     SETTLE="$2";   shift 2 ;;
        --timeout)    TIMEOUT="$2";  shift 2 ;;
        *) echo "Unknown option: $1" >&2; exit 1 ;;
    esac
done

# ── Pre-flight checks ─────────────────────────────────────────────────────────
if ! command -v adb >/dev/null 2>&1; then
    echo "ERROR: adb not found. Install android-tools." >&2; exit 1
fi
if ! adb devices | grep -q "device$"; then
    echo "ERROR: no ADB device connected. Check USB cable and ADB enable in NextUI settings." >&2
    exit 1
fi

# ── Platform detection (done once) ───────────────────────────────────────────
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

# ── Build once ────────────────────────────────────────────────────────────────
if [ "$NO_BUILD" -eq 0 ]; then
    echo "==> Cross-compiling for $PLATFORM..."
    ./scripts/build.sh "$PLATFORM"
fi

# ── Push binary once ──────────────────────────────────────────────────────────
echo "==> Pushing binary to device..."
adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"

# ── Helper: capture one screen ───────────────────────────────────────────────
# capture_screen <screen_name> <wait_pattern> <out_file> <is_last>
capture_screen() {
    _SCREEN="$1"
    _WAIT="$2"
    _OUT="$3"
    _LAST="$4"

    echo ""
    echo "── Screen: $_SCREEN ──────────────────────────────────────────────────"

    # Kill any running instance and clear the log.
    adb shell "pkill -f itchio-pak 2>/dev/null; sleep 0.4; true" || true
    adb shell "truncate -s 0 '$LOG_PATH' 2>/dev/null || rm -f '$LOG_PATH' 2>/dev/null; true" || true

    # Launch in background with dev env vars.
    echo "==> Launching (DEV_START_SCREEN=$_SCREEN, LOG_LEVEL=debug)..."
    adb shell "nohup sh -c 'DEV_START_SCREEN=$_SCREEN LOG_LEVEL=debug $PAK_DEST/launch.sh' \
        > /tmp/pak-dev.log 2>&1 &" || true

    # Wait for readiness.
    echo "==> Waiting for screen (pattern: '$_WAIT', timeout: ${TIMEOUT}s)..."
    _DEADLINE=$(($(date +%s) + TIMEOUT))
    _READY=0
    while [ "$(date +%s)" -lt "$_DEADLINE" ]; do
        if adb shell "test -f '$LOG_PATH' && grep -q '$_WAIT' '$LOG_PATH'" 2>/dev/null; then
            _READY=1
            echo "    Ready."
            break
        fi
        sleep 0.3
    done
    if [ "$_READY" -eq 0 ]; then
        echo "WARNING: timed out waiting for '$_WAIT' — capturing anyway." >&2
    fi

    # Settle.
    sleep "$SETTLE"

    # Ensure output directory exists.
    mkdir -p "$(dirname "$_OUT")"

    # Capture.
    echo "==> Capturing → $_OUT"
    ./scripts/screenshot.sh --platform "$PLATFORM" "$_OUT"

    # Kill unless this is the last screen and --keep-alive was set.
    if [ "$_LAST" -eq 0 ] || [ "$KEEP_ALIVE" -eq 0 ]; then
        adb shell "pkill -f itchio-pak 2>/dev/null; true" || true
    fi
}

# ── Resolve screens to capture ────────────────────────────────────────────────
if [ "$CAPTURE_ALL" -eq 1 ]; then
    # Build the list of (screen, pattern, outfile) tuples from ALL_SCREENS.
    SCREENS_TO_RUN="$ALL_SCREENS"
    echo "==> Capturing all screens → $OUT_DIR/"
    mkdir -p "$OUT_DIR"

    # Count total for last-screen detection.
    TOTAL=0
    for _entry in $SCREENS_TO_RUN; do
        TOTAL=$((TOTAL + 1))
    done

    IDX=0
    for _entry in $SCREENS_TO_RUN; do
        IDX=$((IDX + 1))
        _s="${_entry%%:*}"
        _p="${_entry#*:}"
        _f="$OUT_DIR/$_s.png"
        _last=0
        [ "$IDX" -eq "$TOTAL" ] && _last=1
        capture_screen "$_s" "$_p" "$_f" "$_last"
    done

    echo ""
    echo "All screenshots saved to $OUT_DIR/:"
    for _entry in $SCREENS_TO_RUN; do
        _s="${_entry%%:*}"
        echo "  $OUT_DIR/$_s.png"
    done
else
    # Single screen.
    case "$SCREEN" in
        list)     WAIT_PATTERN="cache:"    ;;
        detail)   WAIT_PATTERN="detail:"   ;;
        settings) WAIT_PATTERN="platform=" ;;
        *)        WAIT_PATTERN="platform=" ;;
    esac
    OUT="${OUT:-$OUT_DIR/$SCREEN.png}"
    echo "==> Platform: $PLATFORM | Screen: $SCREEN | Output: $OUT"
    capture_screen "$SCREEN" "$WAIT_PATTERN" "$OUT" 1
    echo ""
    echo "Screenshot saved: $OUT"
fi

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

. "$SCRIPT_DIR/targets.sh"
. "$SCRIPT_DIR/adb.sh"

# ── All supported screens and their readiness patterns ────────────────────────
# Format: "<screen>:<wait_pattern>"
# The wait_pattern is grepped from the device log to detect when the screen
# is fully initialised and rendered.
ALL_SCREENS="list:cache: detail:dev:detail-ready settings:theme:"

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
SETTLE=4
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
adb_use nextui

# ── Platform detection (done once) ───────────────────────────────────────────
if [ -n "${DEPLOY_PLATFORM:-}" ]; then
    PLATFORM="$DEPLOY_PLATFORM"
else
    echo "==> Detecting device platform..."
    # Use stdout-based checks — adb shell exit codes are unreliable on some hosts.
    if adb shell "grep -o TG3040 /proc/cpuinfo" 2>/dev/null | grep -q TG3040; then
        PLATFORM="tg5040"
    elif adb shell "grep -o TG5050 /proc/cpuinfo" 2>/dev/null | grep -q TG5050; then
        PLATFORM="tg5050"
    elif adb shell "[ -d /usr/miyoo ] && echo miyoo" 2>/dev/null | grep -q miyoo; then
        PLATFORM="my355"
    else
        PLATFORM="tg5040"
    fi
    echo "    Detected: $PLATFORM"
fi

TARGET="nextui/$PLATFORM"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/logs/$BIN_NAME.log"

# ── Build once ────────────────────────────────────────────────────────────────
if [ "$NO_BUILD" -eq 0 ]; then
    echo "==> Cross-compiling for $TARGET..."
    ./scripts/build.sh "$TARGET"
fi

# ── Push binary once ──────────────────────────────────────────────────────────
echo "==> Pushing binary to device..."
adb push "$(target_binary "$TARGET")" "$PAK_DEST/$BIN_NAME"

# ── Keepawake: prevent device idle-sleep during the capture session ───────────
# When launched from ADB (not via NextUI), the system's idle-timeout is not
# suppressed by the launcher.  A kernel wakelock prevents suspend; periodic
# adb pings act as a fallback on kernels where wake_lock is not accessible.
adb shell "echo 'dev-screenshot' > /sys/power/wake_lock 2>/dev/null" || true
_keepawake() {
    while true; do
        adb shell "true" 2>/dev/null
        sleep 10
    done
}
_keepawake &
_KEEPAWAKE_PID=$!
_cleanup() {
    kill "$_KEEPAWAKE_PID" 2>/dev/null
    wait "$_KEEPAWAKE_PID" 2>/dev/null || true
    adb shell "echo 'dev-screenshot' > /sys/power/wake_unlock 2>/dev/null" || true
}
trap "_cleanup" EXIT INT TERM

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
    adb shell "pkill -f $BIN_NAME 2>/dev/null; sleep 0.4; true" || true
    adb shell "truncate -s 0 '$LOG_PATH' 2>/dev/null || rm -f '$LOG_PATH' 2>/dev/null; true" || true

    # Launch in background with dev env vars.
    # PLATFORM and SHARED_USERDATA_PATH must be set so the binary writes its log
    # to the expected path and loads config/cache from the correct location.
    # nohup is unavailable on BusyBox devices; trap '' HUP makes the shell (and
    # any child it exec's) ignore SIGHUP, so the app survives the adb session close.
    echo "==> Launching (DEV_START_SCREEN=$_SCREEN, LOG_LEVEL=debug)..."
    adb shell "(trap '' HUP; DEV_START_SCREEN=$_SCREEN LOG_LEVEL=debug PLATFORM=$PLATFORM \
        SHARED_USERDATA_PATH=/mnt/SDCARD/.userdata/shared \
        $PAK_DEST/launch.sh > /tmp/pak-dev.log 2>&1 &)" || true

    # Wait for readiness.
    # adb shell exit codes are unreliable on some hosts (always return 0), so
    # readiness is detected via stdout output rather than exit status.
    echo "==> Waiting for screen (pattern: '$_WAIT', timeout: ${TIMEOUT}s)..."
    _DEADLINE=$(($(date +%s) + TIMEOUT))
    _READY=0
    while [ "$(date +%s)" -lt "$_DEADLINE" ]; do
        if adb shell "grep '$_WAIT' '$LOG_PATH' 2>/dev/null" | grep -q "$_WAIT"; then
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
        adb shell "pkill -f $BIN_NAME 2>/dev/null; true" || true
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
        list)     WAIT_PATTERN="cache:"  ;;
        detail)   WAIT_PATTERN="dev:detail-ready" ;;
        settings) WAIT_PATTERN="theme:"  ;;
        *)        WAIT_PATTERN="theme:"  ;;
    esac
    OUT="${OUT:-$OUT_DIR/$SCREEN.png}"
    echo "==> Platform: $PLATFORM | Screen: $SCREEN | Output: $OUT"
    capture_screen "$SCREEN" "$WAIT_PATTERN" "$OUT" 1
    echo ""
    echo "Screenshot saved: $OUT"
fi

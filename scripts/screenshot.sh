#!/bin/sh
# Capture the current device screen via ADB and save as a PNG.
# Usage: ./scripts/screenshot.sh [--platform tg5040|tg5050|my355] [output.png]
#
# Requires: adb, ffmpeg
# Device must be connected via USB with ADB enabled.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: screenshot.sh [--platform tg5040|tg5050|my355] [output.png]

Capture the current device framebuffer via ADB and save as a PNG.
Platform is auto-detected from the connected device when omitted.

Arguments:
  --platform    Override platform (tg5040=1024×768, tg5050=1280×720, my355=640×480)
  output.png    Destination file (default: screenshot.png)

Requires: adb, ffmpeg
Device must be connected via USB with ADB enabled (NextUI Settings → Developer → ADB over USB).

Examples:
  ./scripts/screenshot.sh
  ./scripts/screenshot.sh --platform my355 docs/mockups/list.png
  ./scripts/screenshot.sh --platform tg5040 docs/mockups/detail.png
EOF
    exit 0
fi

PLATFORM_OVERRIDE=""
OUT=""

while [ $# -gt 0 ]; do
    case "$1" in
        --platform) PLATFORM_OVERRIDE="$2"; shift 2 ;;
        *) OUT="$1"; shift ;;
    esac
done

OUT="${OUT:-screenshot.png}"

# ── Platform detection ────────────────────────────────────────────────────────
if [ -n "$PLATFORM_OVERRIDE" ]; then
    PLATFORM="$PLATFORM_OVERRIDE"
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

case "$PLATFORM" in
    my355)
        W=640; H=480
        BLOCKS=300   # 640*480*4 / 4096 = 300 blocks
        ;;
    tg5050)
        W=1280; H=720
        BLOCKS=900   # 1280*720*4 / 4096 = 900 blocks
        ;;
    *)  # tg5040 and unknown — default to TrimUI Brick 1024×768
        W=1024; H=768
        BLOCKS=768   # 1024*768*4 / 4096 = 768 blocks
        ;;
esac

echo "==> Capturing ${W}×${H} framebuffer from device (platform=$PLATFORM)..."
adb shell "dd if=/dev/fb0 bs=4096 count=$BLOCKS of=/tmp/screen.raw 2>/dev/null"
adb pull /tmp/screen.raw /tmp/itchio-screen.raw >/dev/null

echo "==> Converting raw framebuffer (${W}×${H} BGRA) → $OUT"
ffmpeg -y -f rawvideo -pix_fmt bgra -s "${W}x${H}" \
    -i /tmp/itchio-screen.raw "$OUT" 2>/dev/null

echo "Saved: $OUT"

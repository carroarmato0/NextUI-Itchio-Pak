#!/bin/sh
# Capture the current device screen via ADB and save as a PNG.
# Usage: ./scripts/screenshot.sh [output.png]
#
# Requires: adb, ffmpeg
# Device must be connected via USB with ADB enabled.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: screenshot.sh [output.png]

Capture the current device framebuffer via ADB and save as a PNG.
Assumes a 1024x768 BGRA framebuffer (tg5040/TrimUI Brick layout).

Arguments:
  output.png    Destination file (default: screenshot.png)

Requires: adb, ffmpeg
Device must be connected via USB with ADB enabled (NextUI Settings → Developer → ADB over USB).

Examples:
  ./scripts/screenshot.sh
  ./scripts/screenshot.sh docs/screenshots/main.png
EOF
    exit 0
fi

OUT="${1:-screenshot.png}"

echo "==> Capturing framebuffer from device..."
adb shell "dd if=/dev/fb0 bs=4096 count=768 of=/tmp/screen.raw 2>/dev/null"
adb pull /tmp/screen.raw /tmp/itchio-screen.raw >/dev/null

echo "==> Converting raw framebuffer (1024x768 BGRA) → $OUT"
ffmpeg -y -f rawvideo -pix_fmt bgra -s 1024x768 \
    -i /tmp/itchio-screen.raw "$OUT" 2>/dev/null

echo "Saved: $OUT"

#!/bin/sh
# Capture the current device screen via ADB and save as a PNG.
# Usage: ./scripts/screenshot.sh [output.png]
#
# Requires: adb, ffmpeg
# Device must be connected via USB with ADB enabled.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

OUT="${1:-screenshot.png}"

echo "==> Capturing framebuffer from device..."
adb shell "dd if=/dev/fb0 bs=4096 count=768 of=/tmp/screen.raw 2>/dev/null"
adb pull /tmp/screen.raw /tmp/itchio-screen.raw >/dev/null

echo "==> Converting raw framebuffer (1024x768 BGRA) → $OUT"
ffmpeg -y -f rawvideo -pix_fmt bgra -s 1024x768 \
    -i /tmp/itchio-screen.raw "$OUT" 2>/dev/null

echo "Saved: $OUT"

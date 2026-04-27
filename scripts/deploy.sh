#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_SRC="dist/Itch-io.pak"

if [ ! -d "$PAK_SRC" ]; then
    echo "ERROR: $PAK_SRC not found. Run scripts/release.sh first." >&2
    exit 1
fi

SD_PATH="${1:-}"

# deploy.sh assets — push only the assets/ directory via ADB without requiring
# a full release build. Useful when only asset files (fonts, certs) have changed.
if [ "$SD_PATH" = "assets" ]; then
    echo "==> Deploying assets/ via ADB..."
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2; exit 1
    fi
    DEVICE="$(adb devices | awk 'NR==2 {print $1}')"
    if [ -z "$DEVICE" ]; then
        echo "ERROR: no ADB device connected. Check USB cable." >&2; exit 1
    fi
    DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
    adb push "assets/." "$DEST/assets/"
    echo "Assets deployed to $DEVICE:$DEST/assets/"
    exit 0
fi

if [ -n "$SD_PATH" ]; then
    echo "==> Deploying to SD card: $SD_PATH"
    DEST="$SD_PATH/Tools/$PLATFORM/Itch-io.pak"
    mkdir -p "$DEST"
    cp -r "$PAK_SRC/." "$DEST/"
    echo "Deployed to $DEST"
else
    echo "==> Deploying via ADB..."
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2
        exit 1
    fi
    DEVICE="$(adb devices | awk 'NR==2 {print $1}')"
    if [ -z "$DEVICE" ]; then
        echo "ERROR: no ADB device connected. Check USB cable." >&2; exit 1
    fi
    DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
    adb shell "mkdir -p $DEST"
    adb push "$PAK_SRC/." "$DEST/"
    echo "Deployed to $DEVICE:$DEST"
fi

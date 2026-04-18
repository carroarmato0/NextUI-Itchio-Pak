#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch.io.pak"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/Itchio/itchio-pak.log"

check_adb() {
    if ! command -v adb >/dev/null 2>&1; then
        echo "ERROR: adb not found. Install android-tools." >&2; exit 1
    fi
    if ! adb devices | grep -q "device$"; then
        echo "ERROR: no ADB device. Check USB cable." >&2; exit 1
    fi
}

CMD="${1:-}"
case "$CMD" in
    logs)
        check_adb
        echo "==> Streaming log (Ctrl-C to stop)..."
        adb shell "tail -f $LOG_PATH"
        ;;
    push)
        check_adb
        echo "==> Building $PLATFORM..."
        ./scripts/build.sh "$PLATFORM"
        echo "==> Pushing binary..."
        adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"
        ;;
    run)
        check_adb
        echo "==> Building and pushing..."
        ./scripts/build.sh "$PLATFORM"
        adb push "bin/$PLATFORM/itchio-pak" "$PAK_DEST/itchio-pak"
        echo "==> Running (Ctrl-C to stop)..."
        adb shell "cd $PAK_DEST && ./itchio-pak 2>&1 | tee /tmp/pak-run.log"
        ;;
    pull-cache)
        check_adb
        mkdir -p debug-cache
        adb pull /tmp/itchio-pak/cache/ ./debug-cache/
        echo "Cache pulled to ./debug-cache/"
        ;;
    pull-log)
        check_adb
        adb pull "$LOG_PATH" .
        echo "Log pulled to ./itchio-pak.log"
        ;;
    shell)
        check_adb
        adb shell
        ;;
    *)
        echo "Usage: debug.sh logs|push|run|pull-cache|pull-log|shell" >&2
        exit 1
        ;;
esac

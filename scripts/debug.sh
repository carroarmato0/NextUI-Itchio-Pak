#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"
. "$SCRIPT_DIR/adb.sh"

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
TARGET="nextui/$PLATFORM"
PAK_DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
LOG_PATH="/mnt/SDCARD/.userdata/$PLATFORM/logs/$BIN_NAME.log"
PROF_DIR="$(pwd)/debug-profiles"

# Pins ANDROID_SERIAL so every bare adb call below targets the NextUI device
# even when a muOS handheld is attached at the same time.
check_adb() {
    adb_use nextui
}

CMD="${1:-}"
case "$CMD" in
    --help|-h)
        cat <<'EOF'
Usage: debug.sh <command>

Commands:
  logs             Stream the runtime log from the connected device (Ctrl-C to stop)
  push             Build for DEPLOY_PLATFORM and push the binary + launch script via ADB
  run              Build, push, then launch the binary directly (shows all stdout/stderr)
  pull-cache       Pull /tmp/itchio/cache/ to ./debug-cache/
  pull-log         Pull the runtime log to the current directory
  shell            Open an interactive ADB shell on the device

  profile          Enable CPU+memory profiling on the device; launch via NextUI to record
  profile-cpu      Enable CPU-only profiling on the device; launch via NextUI to record
  profile-mem      Enable memory-only profiling on the device; launch via NextUI to record
  profile-live     Enable live pprof on the device (HTTP :6060 via ADB forward); launch via NextUI
  profile-restore  Remove profiling flags from device; restores normal launch behaviour
  pull-profile     Pull recorded profile files from the device to ./debug-profiles/

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050|my355|h700   Target platform (default: tg5040)

Log path on device: /mnt/SDCARD/.userdata/<platform>/logs/itchio.log

Profiling workflow (file-based):
  1. ./scripts/debug.sh profile          # enable profiling; prints next steps
  2. Launch Itch.io via NextUI normally  # no ADB, no framebuffer flicker
  3. Use the app, then exit via B button
  4. ./scripts/debug.sh pull-profile     # fetch profiles to ./debug-profiles/
  5. ./scripts/debug.sh profile-restore  # remove profiling flags from device
  6. go tool pprof bin/nextui/tg5040/itchio ./debug-profiles/itchio-cpu.prof

Live pprof workflow:
  1. ./scripts/debug.sh profile-live     # enable live pprof + set up ADB port forward
  2. Launch Itch.io via NextUI normally
  3. go tool pprof 'http://localhost:6060/debug/pprof/profile?seconds=30'
     go tool pprof http://localhost:6060/debug/pprof/heap
     # or browse http://localhost:6060/debug/pprof/
  4. ./scripts/debug.sh profile-restore  # remove flags + port forward

Examples:
  ./scripts/debug.sh logs
  ./scripts/debug.sh push
  ./scripts/debug.sh run
  DEPLOY_PLATFORM=my355 ./scripts/debug.sh push
  ./scripts/debug.sh pull-log && cat itchio.log
  ./scripts/debug.sh profile
  ./scripts/debug.sh pull-profile
  ./scripts/debug.sh profile-restore
EOF
        exit 0
        ;;
    logs)
        check_adb
        echo "==> Streaming log from last startup (Ctrl-C to stop)..."
        LINE=$(adb shell "grep -n 'itchio .* starting' $LOG_PATH 2>/dev/null | tail -1 | cut -d: -f1" 2>/dev/null | tr -d '\r')
        if [ -n "$LINE" ]; then
            adb shell "tail -n +$LINE -f $LOG_PATH" | awk '
                BEGIN { ts0 = ""; show = 0 }
                /itchio .* starting/ {
                    ts = substr($0, 1, 19)
                    if (ts0 == "" || ts > ts0) { ts0 = ts; show = 1; print } else { show = 0 }
                    next
                }
                show { print }
            '
        else
            adb shell "tail -f $LOG_PATH"
        fi
        ;;
    push)
        check_adb
        echo "==> Building $TARGET..."
        ./scripts/build.sh "$TARGET"
        echo "==> Pushing binary and launch script..."
        adb push "$(target_binary "$TARGET")" "$PAK_DEST/$BIN_NAME"
        adb push "launch.sh" "$PAK_DEST/launch.sh"
        ;;
    run)
        check_adb
        echo "==> Building and pushing..."
        ./scripts/build.sh "$TARGET"
        adb push "$(target_binary "$TARGET")" "$PAK_DEST/$BIN_NAME"
        adb push "launch.sh" "$PAK_DEST/launch.sh"
        echo "==> Running (Ctrl-C to stop)..."
        adb shell "cd $PAK_DEST && ./launch.sh 2>&1 | tee /tmp/pak-run.log"
        ;;
    pull-cache)
        check_adb
        mkdir -p debug-cache
        adb pull /tmp/itchio/cache/ ./debug-cache/
        echo "Cache pulled to $(pwd)/debug-cache/"
        ;;
    pull-log)
        check_adb
        adb pull "$LOG_PATH" .
        echo "Log pulled to $(pwd)/itchio.log"
        ;;
    shell)
        check_adb
        adb shell
        ;;
    profile)
        check_adb
        echo "==> Installing CPU + memory profiling flags..."
        adb shell "printf '%s' '-cpuprofile /tmp/itchio-cpu.prof -memprofile /tmp/itchio-mem.prof' > $PAK_DEST/.profile-flags"
        echo ""
        echo "Profiling enabled. Next steps:"
        echo "  1. Launch Itch.io via NextUI normally (no ADB needed)"
        echo "  2. Use the app, then exit via the B button"
        echo "  3. Run: ./scripts/debug.sh pull-profile"
        echo "     Profiles will be saved to: $PROF_DIR/"
        echo "  4. Run: ./scripts/debug.sh profile-restore"
        echo "     Removes the profiling flags so the app runs normally again"
        ;;
    profile-cpu)
        check_adb
        echo "==> Installing CPU profiling flags..."
        adb shell "printf '%s' '-cpuprofile /tmp/itchio-cpu.prof' > $PAK_DEST/.profile-flags"
        echo ""
        echo "CPU profiling enabled. Next steps:"
        echo "  1. Launch Itch.io via NextUI normally (no ADB needed)"
        echo "  2. Use the app, then exit via the B button"
        echo "  3. Run: ./scripts/debug.sh pull-profile"
        echo "     Profile will be saved to: $PROF_DIR/itchio-cpu.prof"
        echo "  4. Run: ./scripts/debug.sh profile-restore"
        ;;
    profile-mem)
        check_adb
        echo "==> Installing memory profiling flags..."
        adb shell "printf '%s' '-memprofile /tmp/itchio-mem.prof' > $PAK_DEST/.profile-flags"
        echo ""
        echo "Memory profiling enabled. Next steps:"
        echo "  1. Launch Itch.io via NextUI normally (no ADB needed)"
        echo "  2. Use the app, then exit via the B button"
        echo "  3. Run: ./scripts/debug.sh pull-profile"
        echo "     Profile will be saved to: $PROF_DIR/itchio-mem.prof"
        echo "  4. Run: ./scripts/debug.sh profile-restore"
        ;;
    profile-live)
        check_adb
        echo "==> Installing live pprof flags (port :6060)..."
        adb shell "printf '%s' '-pprof :6060' > $PAK_DEST/.profile-flags"
        echo "==> Forwarding device port 6060 -> host port 6060..."
        adb forward tcp:6060 tcp:6060
        echo ""
        echo "Live pprof enabled. Next steps:"
        echo "  1. Launch Itch.io via NextUI normally (no ADB needed)"
        echo "  2. Capture profiles from another terminal:"
        echo "       go tool pprof 'http://localhost:6060/debug/pprof/profile?seconds=30'"
        echo "       go tool pprof http://localhost:6060/debug/pprof/heap"
        echo "     or browse: http://localhost:6060/debug/pprof/"
        echo "  3. When finished, run: ./scripts/debug.sh profile-restore"
        echo "     Removes the flags and the ADB port forward"
        ;;
    profile-restore)
        check_adb
        echo "==> Removing profiling flags from device..."
        adb shell "rm -f $PAK_DEST/.profile-flags"
        echo "==> Removing ADB port forward (if any)..."
        adb forward --remove tcp:6060 2>/dev/null || true
        echo "Done. App will run normally on next launch."
        ;;
    pull-profile)
        check_adb
        mkdir -p "$PROF_DIR"
        GOT=0
        adb pull /tmp/itchio-cpu.prof "$PROF_DIR/itchio-cpu.prof" 2>/dev/null && \
            echo "CPU profile    -> $PROF_DIR/itchio-cpu.prof" && GOT=1 || \
            echo "No CPU profile on device (app may still be running)"
        adb pull /tmp/itchio-mem.prof "$PROF_DIR/itchio-mem.prof" 2>/dev/null && \
            echo "Memory profile -> $PROF_DIR/itchio-mem.prof" && GOT=1 || \
            echo "No memory profile on device (app may still be running)"
        if [ "$GOT" -eq 1 ]; then
            echo ""
            echo "Analyze with:"
            echo "  go tool pprof $(target_binary "$TARGET") $PROF_DIR/itchio-cpu.prof"
            echo "  go tool pprof $(target_binary "$TARGET") $PROF_DIR/itchio-mem.prof"
        fi
        ;;
    *)
        echo "Usage: debug.sh logs|push|run|pull-cache|pull-log|shell|profile|profile-cpu|profile-mem|profile-live|profile-restore|pull-profile" >&2
        exit 1
        ;;
esac

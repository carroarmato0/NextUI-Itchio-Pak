#!/bin/sh
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

. "$SCRIPT_DIR/targets.sh"
. "$SCRIPT_DIR/adb.sh"

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then
    cat <<'EOF'
Usage: deploy.sh [<sd-path>|assets|muos|muxapp]

Deploy a built release to a connected device. Run scripts/release.sh first.

Arguments:
  <none>        Push the NextUI pak to a connected NextUI device via ADB
  <sd-path>     Copy the NextUI pak to a mounted SD card, e.g. /run/media/user/SD
  assets        Push only assets/ to the NextUI device, without a full rebuild
  muos          Push the muOS application directory straight to a muOS device
  muxapp        Copy the .muxapp into the device's ARCHIVE folder, to install
                through Applications -> Archive Manager the way a user would

Environment:
  DEPLOY_PLATFORM=tg5040|tg5050|my355|h700   NextUI platform directory (default: tg5040)
  ADB_SERIAL=<serial>                   Target a specific device; otherwise the
                                        single attached device of the right
                                        firmware is chosen automatically
                                        (ANDROID_SERIAL is honoured too)

Examples:
  ./scripts/deploy.sh
  ./scripts/deploy.sh /run/media/user/SD
  ./scripts/deploy.sh muos
  ./scripts/deploy.sh muxapp
  DEPLOY_PLATFORM=my355 ./scripts/deploy.sh
EOF
    exit 0
fi

PLATFORM="${DEPLOY_PLATFORM:-tg5040}"
PAK_SRC="dist/nextui/Itch-io.pak"
MUOS_SRC="dist/muos/Itch-io"

require_built() {
    [ -e "$1" ] && return 0
    echo "ERROR: $1 not found. Run scripts/release.sh first." >&2
    exit 1
}

MODE="${1:-}"

case "$MODE" in
assets)
    # Push only the assets/ directory, for when fonts or the CA bundle changed
    # but the binary did not.
    echo "==> Deploying assets/ via ADB..."
    adb_use nextui
    DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
    adb push "assets/." "$DEST/assets/"
    echo "Assets deployed to $ADB_DEVICE:$DEST/assets/"
    ;;

muos)
    # Straight into the application directory, which is the fast path for
    # iterating. It skips Archive Manager, so it does not prove the archive
    # itself is well formed — use the muxapp mode for that.
    require_built "$MUOS_SRC"
    echo "==> Deploying to muOS via ADB..."
    adb_use muos

    ROM_MOUNT="$(adb shell 'cat /opt/muos/device/config/storage/rom/mount' | tr -d ' \t\r\n')"
    [ -n "$ROM_MOUNT" ] || ROM_MOUNT="/mnt/mmc"
    DEST="$ROM_MOUNT/MUOS/application/Itch-io"

    adb shell "mkdir -p '$DEST'"
    adb push "$MUOS_SRC/." "$DEST/"
    # adb push does not preserve the exec bit, and muOS silently does nothing
    # with a launcher it cannot execute.
    adb shell "chmod +x '$DEST/mux_launch.sh' '$DEST/$BIN_NAME'"
    echo "Deployed to $ADB_DEVICE:$DEST"
    echo "Launch it from Applications on the device."
    ;;

muxapp)
    # The real install path: drop the archive where muOS expects it and let the
    # user install it through Archive Manager.
    VERSION="$(pak_version)"
    ARCHIVE="dist/muos/Itch-io.muOS.$VERSION.muxapp"
    require_built "$ARCHIVE"
    echo "==> Copying $(basename "$ARCHIVE") to the device ARCHIVE folder..."
    adb_use muos

    ROM_MOUNT="$(adb shell 'cat /opt/muos/device/config/storage/rom/mount' | tr -d ' \t\r\n')"
    [ -n "$ROM_MOUNT" ] || ROM_MOUNT="/mnt/mmc"
    DEST="$ROM_MOUNT/ARCHIVE"

    adb shell "mkdir -p '$DEST'"
    adb push "$ARCHIVE" "$DEST/"
    echo "Copied to $ADB_DEVICE:$DEST/$(basename "$ARCHIVE")"
    echo "On the device: Applications -> Archive Manager -> select it -> A"
    ;;

"")
    require_built "$PAK_SRC"
    echo "==> Deploying via ADB..."
    adb_use nextui
    DEST="/mnt/SDCARD/Tools/$PLATFORM/Itch-io.pak"
    adb shell "mkdir -p $DEST"
    adb push "$PAK_SRC/." "$DEST/"
    echo "Deployed to $ADB_DEVICE:$DEST"
    ;;

*)
    require_built "$PAK_SRC"
    SD_PATH="$MODE"
    echo "==> Deploying to SD card: $SD_PATH"
    DEST="$SD_PATH/Tools/$PLATFORM/Itch-io.pak"
    mkdir -p "$DEST"
    cp -r "$PAK_SRC/." "$DEST/"
    echo "Deployed to $DEST"
    ;;
esac

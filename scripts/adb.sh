#!/bin/sh
# Shared ADB device selection.
#
# Sourced by deploy.sh and debug.sh.  More than one handheld is often attached
# at once (a NextUI device and a muOS device, say), so picking "the first line
# of adb devices" silently targets the wrong hardware roughly half the time.
#
# Selection order:
#   1. $ADB_SERIAL, if set
#   2. the only attached device, if there is exactly one
#   3. the only attached device running $1 (nextui|muos), if a firmware is given
# Anything else is an error listing what was found — never a guess.
#
# Firmware is probed on the device rather than read from USB descriptors, which
# lie: the TrimUI Smart Pro reports itself as "Nexus_4 / mako".

require_adb() {
    command -v adb >/dev/null 2>&1 && return 0
    echo "ERROR: adb not found. Install android-tools (or android-platform-tools)." >&2
    exit 1
}

# adb_serials -> one serial per line for every device in state "device".
adb_serials() {
    adb devices | awk 'NR>1 && $2=="device" {print $1}'
}

# adb_firmware <serial> -> nextui | muos | unknown
adb_firmware() {
    if adb -s "$1" shell '[ -d /opt/muos ] && echo yes' 2>/dev/null | grep -q yes; then
        echo muos
    elif adb -s "$1" shell '[ -d /mnt/SDCARD/.system ] && echo yes' 2>/dev/null | grep -q yes; then
        echo nextui
    else
        echo unknown
    fi
}

# adb_select [firmware] -> the serial to use, or exits with a readable error.
adb_select() {
    WANT="${1:-}"

    # ANDROID_SERIAL is adb's own variable, so anyone who has already exported it
    # to drive adb by hand should not have to learn a second name for it.
    if [ -n "${ADB_SERIAL:-}" ]; then
        printf '%s\n' "$ADB_SERIAL"
        return 0
    fi
    if [ -n "${ANDROID_SERIAL:-}" ]; then
        printf '%s\n' "$ANDROID_SERIAL"
        return 0
    fi

    SERIALS="$(adb_serials)"
    COUNT="$(printf '%s\n' "$SERIALS" | grep -c .)"

    if [ "$COUNT" -eq 0 ]; then
        echo "ERROR: no ADB device connected. Check the USB cable." >&2
        exit 1
    fi

    if [ "$COUNT" -eq 1 ]; then
        printf '%s\n' "$SERIALS"
        return 0
    fi

    if [ -n "$WANT" ]; then
        MATCHED=""
        for s in $SERIALS; do
            [ "$(adb_firmware "$s")" = "$WANT" ] && MATCHED="$MATCHED $s"
        done
        set -- $MATCHED
        if [ "$#" -eq 1 ]; then
            printf '%s\n' "$1"
            return 0
        fi
        if [ "$#" -gt 1 ]; then
            echo "ERROR: $# attached devices run $WANT:$MATCHED" >&2
            echo "       Pick one with ADB_SERIAL=<serial>." >&2
            exit 1
        fi
    fi

    echo "ERROR: $COUNT ADB devices attached and none could be chosen automatically:" >&2
    for s in $SERIALS; do
        echo "         $s  ($(adb_firmware "$s"))" >&2
    done
    echo "       Pick one with ADB_SERIAL=<serial>." >&2
    exit 1
}

# adb_use [firmware] — resolve the device once and pin every later bare `adb`
# call to it via ANDROID_SERIAL, which the adb client reads natively.  Scripts
# with many adb invocations call this once instead of threading -s through all
# of them.  Sets $ADB_DEVICE for messages.
adb_use() {
    require_adb
    ADB_DEVICE="$(adb_select "${1:-}")" || exit 1
    ANDROID_SERIAL="$ADB_DEVICE"
    export ANDROID_SERIAL
}

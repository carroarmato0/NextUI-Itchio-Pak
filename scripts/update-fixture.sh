#!/bin/sh
# Serves a NextUI pak zip as a fake GitHub release, so the in-place install can
# be tested on a device before two real releases carry the feature
# (docs/superpowers/specs/2026-10-07-nextui-in-place-install-design.md §6).
#
#   update-fixture.sh serve [--host <ip>] [--broken] <pak.zip>
#                                                serve it as the newest rc
#                                                (flags may appear in either
#                                                order)
#   update-fixture.sh off                        stop using the override
#
# --broken       itchio replaced by the device's busybox (exits at once: the
#                rollback case).
# --host <ip>    skip adb-reverse detection and serve on <ip> directly: bind
#                it, and use it in the release base URL (html_url, the asset's
#                browser_download_url) and in the device's override file.
#
# Normally the device reaches the server through `adb reverse` on
# 127.0.0.1:$PORT. Some devices' adbd does not support `adb reverse` (e.g. the
# TrimUI Brick, which fails with "adb: error: closed"); when that happens this
# script falls back to serving on the host's own LAN address instead, which
# works as long as the device and host share a network.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
# shellcheck disable=SC1091
. "$SCRIPT_DIR/adb.sh"

PORT=8765
DATA_DIR="/mnt/SDCARD/.userdata/shared/Itch-io"

usage() { sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

off() {
    adb_use nextui
    adb shell "rm -f '$DATA_DIR/update_source_override'" </dev/null
    adb reverse --remove "tcp:$PORT" 2>/dev/null || true
    echo "Override removed from $ADB_DEVICE."
}

# lan_host_for <device_ip> -> the host's source address for reaching
# <device_ip>, i.e. the address the device needs to connect back to.
lan_host_for() {
    ip -4 route get "$1" 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p' | head -n1
}

# device_wlan_ip -> the device's IPv4 address on wlan0, or empty.
device_wlan_ip() {
    adb shell "ip -4 addr show wlan0" </dev/null \
        | sed -n 's#.*inet \([0-9.]*\)/.*#\1#p' | head -n1
}

serve() {
    broken=0
    host_override=""
    while [ $# -gt 0 ]; do
        case "$1" in
            --broken)
                broken=1
                shift
                ;;
            --host)
                [ $# -ge 2 ] && [ -n "$2" ] || usage
                host_override="$2"
                shift 2
                ;;
            *)
                break
                ;;
        esac
    done
    [ $# -eq 1 ] && [ -f "$1" ] || usage
    adb_use nextui

    dir="$(mktemp -d)"
    reverse_ok=0
    trap 'rm -rf "$dir"; if [ "$reverse_ok" = 1 ]; then adb reverse --remove "tcp:$PORT" 2>/dev/null || true; fi' EXIT INT TERM
    version="$(unzip -p "$1" pak.json | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    [ -n "$version" ] || { echo "no version in $1's pak.json" >&2; exit 1; }
    asset="Itch-io.NextUI.$version.pak.zip"
    cp "$1" "$dir/$asset"
    if [ "$broken" = 1 ]; then
        adb pull /bin/busybox "$dir/itchio" >/dev/null
        (cd "$dir" && zip -q "$asset" itchio && rm itchio)
        echo "Serving a BROKEN $version: itchio is busybox and exits at once."
    fi
    size="$(wc -c < "$dir/$asset" | tr -d ' ')"
    sum="$(sha256sum "$dir/$asset" | cut -d' ' -f1)"

    bind_addr="127.0.0.1"
    base="http://127.0.0.1:$PORT/"
    if [ -n "$host_override" ]; then
        bind_addr="$host_override"
        base="http://$host_override:$PORT/"
    elif adb reverse "tcp:$PORT" "tcp:$PORT" >/dev/null 2>&1; then
        reverse_ok=1
    else
        device_ip="$(device_wlan_ip)"
        [ -n "$device_ip" ] || {
            echo "ERROR: adb reverse is not supported on $ADB_DEVICE, and its wlan0 IPv4 address could not be determined for a LAN fallback." >&2
            exit 1
        }
        host_ip="$(lan_host_for "$device_ip")"
        [ -n "$host_ip" ] || {
            echo "ERROR: adb reverse is not supported on $ADB_DEVICE, and no host route to its address $device_ip was found for a LAN fallback." >&2
            exit 1
        }
        bind_addr="$host_ip"
        base="http://$host_ip:$PORT/"
        echo "adb reverse not supported on $ADB_DEVICE; serving on the LAN at $base"
    fi

    cat > "$dir/releases.json" <<EOF
[{"tag_name":"$version","html_url":"${base}release","draft":false,"prerelease":true,
  "assets":[{"name":"$asset","browser_download_url":"$base$asset","size":$size,"digest":"sha256:$sum"}]}]
EOF
    printf '{"version":"%s"}\n' "$version" > "$dir/pak.json"

    adb shell "mkdir -p '$DATA_DIR' && echo '$base' > '$DATA_DIR/update_source_override'" </dev/null
    echo "Serving $version ($size bytes) on $base for $ADB_DEVICE. Open Settings → App updates → Check now."
    echo "Ctrl-C stops serving; run '$0 off' to remove the override."
    cd "$dir" && python3 -m http.server "$PORT" --bind "$bind_addr"
}

case "${1:-}" in
    serve) shift; serve "$@" ;;
    off)   off ;;
    *)     usage ;;
esac

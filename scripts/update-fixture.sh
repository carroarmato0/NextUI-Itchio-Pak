#!/bin/sh
# Serves a NextUI pak zip as a fake GitHub release, so the in-place install can
# be tested on a device before two real releases carry the feature
# (docs/superpowers/specs/2026-10-07-nextui-in-place-install-design.md §6).
#
#   update-fixture.sh serve <pak.zip>            serve it as the newest rc
#   update-fixture.sh serve --broken <pak.zip>   same, but itchio replaced by the
#                                                device's busybox (exits at once:
#                                                the rollback case)
#   update-fixture.sh off                        stop using the override
#
# The device reaches the server through `adb reverse` on 127.0.0.1:8765.
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."
# shellcheck disable=SC1091
. "$SCRIPT_DIR/adb.sh"

PORT=8765
BASE="http://127.0.0.1:$PORT/"
DATA_DIR="/mnt/SDCARD/.userdata/shared/Itch-io"

usage() { sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

off() {
    adb_use nextui
    adb shell "rm -f '$DATA_DIR/update_source_override'" </dev/null
    adb reverse --remove "tcp:$PORT" 2>/dev/null || true
    echo "Override removed from $ADB_DEVICE."
}

serve() {
    broken=0
    if [ "${1:-}" = "--broken" ]; then broken=1; shift; fi
    [ $# -eq 1 ] && [ -f "$1" ] || usage
    adb_use nextui

    dir="$(mktemp -d)"
    trap 'rm -rf "$dir"; adb reverse --remove "tcp:$PORT" 2>/dev/null || true' EXIT INT TERM
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
    cat > "$dir/releases.json" <<EOF
[{"tag_name":"$version","html_url":"${BASE}release","draft":false,"prerelease":true,
  "assets":[{"name":"$asset","browser_download_url":"$BASE$asset","size":$size,"digest":"sha256:$sum"}]}]
EOF
    printf '{"version":"%s"}\n' "$version" > "$dir/pak.json"

    adb reverse "tcp:$PORT" "tcp:$PORT" >/dev/null
    adb shell "mkdir -p '$DATA_DIR' && echo '$BASE' > '$DATA_DIR/update_source_override'" </dev/null
    echo "Serving $version ($size bytes) on $BASE for $ADB_DEVICE. Open Settings → App updates → Check now."
    echo "Ctrl-C stops serving; run '$0 off' to remove the override."
    cd "$dir" && python3 -m http.server "$PORT" --bind 127.0.0.1
}

case "${1:-}" in
    serve) shift; serve "$@" ;;
    off)   off ;;
    *)     usage ;;
esac

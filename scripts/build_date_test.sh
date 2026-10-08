#!/bin/sh
# The wrong-clock check in internal/netstate needs the build date in the binary.
set -e
. "$(dirname "$0")/lib/ldflags.sh"
FLAGS="$(BUILD_DATE=2026-09-26T10:00:00+02:00 LDFLAGS_FOR v1 abc)"
echo "$FLAGS" | grep -q "internal/netstate.buildDate=2026-09-26T10:00:00+02:00" || {
    echo "FAIL: LDFLAGS_FOR does not set netstate.buildDate: $FLAGS" >&2; exit 1; }
echo "PASS: build date in ldflags"

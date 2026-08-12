#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
# Select bundled SDL2 libs for this device family.
# cpuinfo hwserial contains TG5050 on the Smart Pro S; all other TrimUI devices
# fall through to tg5040.  Miyoo devices expose /usr/miyoo.
if [ -d /usr/miyoo ]; then
    PLATFORM_LIB="$PAK_DIR/lib/my355"
elif grep -q "TG5050" /proc/cpuinfo 2>/dev/null; then
    PLATFORM_LIB="$PAK_DIR/lib/tg5050"
else
    PLATFORM_LIB="$PAK_DIR/lib/tg5040"
fi

# Remove stale versioned SDL2 files left by previous pak versions.  Only the
# SONAME files (libSDL2-2.0.so.0, libSDL2_ttf-2.0.so.0) are needed at runtime;
# the versioned siblings are never referenced directly by the dynamic linker.
rm -f "$PLATFORM_LIB"/libSDL2-2.0.so.0.* \
      "$PLATFORM_LIB"/libSDL2_ttf-2.0.so.0.* 2>/dev/null || true

# Prefer the device-native SDL2 when available (it is tuned for the device's
# display and audio backends).  The bundled LoveRetro SDL2 in PLATFORM_LIB
# serves as a working fallback on devices where the native path is absent.
NATIVE_SDL_LIB=""
for _d in /usr/trimui/lib /usr/miyoo/lib /usr/lib /usr/local/lib; do
    if [ -f "$_d/libSDL2-2.0.so.0" ]; then
        NATIVE_SDL_LIB="$_d"
        break
    fi
done
unset _d
export LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+$NATIVE_SDL_LIB:}$PLATFORM_LIB:$LD_LIBRARY_PATH"
export PATH="$PAK_DIR:$PATH"
# The device has no system CA certificate store; point Go's TLS stack at the
# bundle we ship so HTTPS requests to itch.io can be verified correctly.
export SSL_CERT_FILE="$PAK_DIR/assets/ca-certificates.crt"
mkdir -p "$HOME"
# The binary loads assets/font.ttf relative to the working directory, so a
# failed cd would start it with no fonts rather than not at all.
cd "$PAK_DIR" || exit 1
# Optional profiling flags written by ./scripts/debug.sh profile commands.
# Absent in normal operation; present only during a profiling session.
# Word-splitting is intentional — the file contains space-separated flags.
PROFILE_FLAGS=""
if [ -f "$PAK_DIR/.profile-flags" ]; then
    PROFILE_FLAGS="$(cat "$PAK_DIR/.profile-flags")"
fi
# shellcheck disable=SC2086
exec "$PAK_DIR/itchio" $PROFILE_FLAGS "$@"

#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
# Build LD_LIBRARY_PATH: device-native SDL2 first (built for this device's
# display/audio backend), then pak's own lib dir (SDL2_ttf etc.), then system.
NATIVE_SDL_LIB=""
for _d in /usr/trimui/lib /usr/miyoo/lib; do
    if [ -f "$_d/libSDL2-2.0.so.0" ]; then
        NATIVE_SDL_LIB="$_d"
        break
    fi
done
unset _d
export LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+$NATIVE_SDL_LIB:}$PAK_DIR/lib:$LD_LIBRARY_PATH"
export PATH="$PAK_DIR:$PATH"
# The device has no system CA certificate store; point Go's TLS stack at the
# bundle we ship so HTTPS requests to itch.io can be verified correctly.
export SSL_CERT_FILE="$PAK_DIR/assets/ca-certificates.crt"
mkdir -p "$HOME"
cd "$PAK_DIR"
exec "$PAK_DIR/itchio-pak"

#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
# Select the bundled SDL2 directory for this device family.
#
# $PLATFORM is what NextUI exports; the /usr/miyoo and cpuinfo probes are the
# fallback for launches that arrive without it.  h700 is deliberately absent:
# NextUI installs its own mali-fbdev SDL2 there and the pak ships none, so there
# is no directory to select.
BUNDLED_LIB=""
case "${PLATFORM:-}" in
    h700)   BUNDLED_LIB="" ;;
    my355|tg5050|tg5040)
            BUNDLED_LIB="$PAK_DIR/lib/$PLATFORM" ;;
    *)
        if [ -d /usr/miyoo ]; then
            BUNDLED_LIB="$PAK_DIR/lib/my355"
        elif grep -q "TG5050" /proc/cpuinfo 2>/dev/null; then
            BUNDLED_LIB="$PAK_DIR/lib/tg5050"
        else
            BUNDLED_LIB="$PAK_DIR/lib/tg5040"
        fi
        ;;
esac
PLATFORM_LIB="$BUNDLED_LIB"

# If the firmware ships a complete SDL2 pair of its own — both
# libSDL2-2.0.so.0 and libSDL2_ttf-2.0.so.0 in $SYSTEM_PATH/lib — our bundled
# copy is redundant, and the porting contract asks us not to ship one
# alongside the firmware's, so drop it from LD_LIBRARY_PATH here. A partial
# pair (say, a future NextUI release that adds SDL2 to $SYSTEM_PATH/lib but
# not SDL2_ttf) does not trigger this: PLATFORM_LIB stays set, and
# NATIVE_SDL_LIB below already puts $SYSTEM_PATH/lib ahead of it on
# LD_LIBRARY_PATH, so the bundled directory becomes a harmless fallback that
# supplies only whatever the firmware's copy is missing.
#
# Residual risk: if an H700 launch ever arrived without $PLATFORM, the probes
# above would select a bundled directory (lib/tg5040, since they know nothing
# about h700). $SYSTEM_PATH is normally unset alongside $PLATFORM too —
# NextUI exports both together — so this guard would not fire either, and
# stock Anbernic's /usr/lib SDL2 2.0.12 could load ahead of the bundled
# SDL2_ttf. Considered unlikely, since NextUI exports $PLATFORM to every pak
# it launches; the ld_libs: line in the startup log is what would reveal it
# if it ever happened.
if [ -n "${SYSTEM_PATH:-}" ] && \
   [ -f "$SYSTEM_PATH/lib/libSDL2-2.0.so.0" ] && \
   [ -f "$SYSTEM_PATH/lib/libSDL2_ttf-2.0.so.0" ]; then
    PLATFORM_LIB=""
fi

# Remove the pre-rename binary left behind when upgrading over an older pak.
# Installing writes the new "itchio" alongside it rather than replacing it, so
# without this every upgraded device carries a dead 14MB copy for ever.
rm -f "$PAK_DIR/itchio-pak" 2>/dev/null || true

# Remove stale versioned SDL2 files left by previous pak versions.  Only the
# SONAME files (libSDL2-2.0.so.0, libSDL2_ttf-2.0.so.0) are needed at runtime;
# the versioned siblings are never referenced directly by the dynamic linker.
# Cleans BUNDLED_LIB, not PLATFORM_LIB: the firmware-pair guard above can zero
# PLATFORM_LIB for LD_LIBRARY_PATH purposes while the bundled directory it
# zeroed still exists on disk, and still deserves cleaning up.
if [ -n "$BUNDLED_LIB" ]; then
    rm -f "$BUNDLED_LIB"/libSDL2-2.0.so.0.* \
          "$BUNDLED_LIB"/libSDL2_ttf-2.0.so.0.* 2>/dev/null || true
fi

# Prefer the SDL2 the firmware installed for itself, then the device-native one.
#
# $SYSTEM_PATH/lib comes first because when the firmware ships its own SDL2 it
# is authoritative by definition.  On H700 that is .system/h700/lib, and the
# alternative is stock Anbernic's /usr/lib SDL2 2.0.12 — mali and dummy video
# only, with no libSDL2_ttf beside it.
#
# Verified over ADB that $SYSTEM_PATH/lib holds no libSDL2 on tg5040 and
# tg5050 at firmware NextUI-20260719-0, so on those two this falls through to
# exactly the directory they used before. NOT verified on my355 — no Miyoo
# Flip was available — so my355 behaving the same way is assumed, not
# checked; the Flip is the platform whose .system layout is least
# predictable elsewhere in this codebase, so this assumption is the most
# likely one on this branch to be wrong.
#
# The bundled LoveRetro SDL2 in PLATFORM_LIB is the last resort, for devices
# where no system copy exists.
NATIVE_SDL_LIB=""
for _d in ${SYSTEM_PATH:+"$SYSTEM_PATH/lib"} /usr/trimui/lib /usr/miyoo/lib /usr/lib /usr/local/lib; do
    if [ -f "$_d/libSDL2-2.0.so.0" ]; then
        NATIVE_SDL_LIB="$_d"
        break
    fi
done
unset _d
# Prepend only our own directories; the inherited value is kept, never replaced.
export LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+$NATIVE_SDL_LIB:}${PLATFORM_LIB:+$PLATFORM_LIB:}$LD_LIBRARY_PATH"
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

#!/bin/sh
# HELP: Browse itch.io and download games straight to your device
# ICON: itchio
# GRID: Itch.io

# Sourcing this is mandatory: it provides GET_VAR/SET_VAR and SETUP_APP, and
# muOS's documentation is emphatic that it must never be removed.
# shellcheck source=/dev/null  # lives on the device, not in this repo
. /opt/muos/script/var/func.sh

APP_BIN="itchio"

# SETUP_APP restores the CPU governor, points HOME at the board's home
# directory, records the foreground process so muOS can suspend and resume us,
# and sets up the whole SDL environment — including the controller mapping that
# decides which face button confirms. The empty second argument leaves the
# button layout as the user configured it rather than forcing one.
SETUP_APP "$APP_BIN" ""

# -----------------------------------------------------------------------------

# Resolve our own directory rather than hardcoding a card. muOS bind-mounts the
# application directory from whichever card holds it, and hardcoding /mnt/mmc
# breaks on devices with a second card.
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$APP_DIR" || exit 1

# Keep config, inventory and caches beside the app. SETUP_APP points HOME at
# /root, which lives on the system partition and is replaced by a muOS update;
# the application directory is on the SD card and survives one.
ITCHIO_DATA_DIR="$APP_DIR/data"
export ITCHIO_DATA_DIR
mkdir -p "$ITCHIO_DATA_DIR"

# The device has no system CA store, so point Go's TLS stack at the bundle we
# ship or every HTTPS request to itch.io fails verification.
SSL_CERT_FILE="$APP_DIR/assets/ca-certificates.crt"
export SSL_CERT_FILE

# Put our menu icon where the frontend looks for it.
#
# The ICON header above names a glyph, but glyphs are resolved from the *active
# theme*, not from the application directory — so a third-party app has to
# install its own, which is what every other one does. The theme decides the
# size (26, 34 and 47 px in the stock theme alone), so each destination is
# matched against a glyph already sitting there rather than assumed.
#
# Re-run on every launch so the icon reappears after the user switches theme,
# but only when our copy is actually newer, to keep startup quick.
INSTALL_GLYPH() {
    GLYPH_SRC="$APP_DIR/glyph/itchio.png"
    [ -f "$GLYPH_SRC" ] || return 0

    ACTIVE_THEME="$(GET_VAR "config" "theme/active")"
    [ -n "$ACTIVE_THEME" ] || return 0
    THEME_DIR="$MUOS_STORE_DIR/theme/$ACTIVE_THEME"
    [ -d "$THEME_DIR" ] || return 0

    if ! command -v convert >/dev/null 2>&1; then
        LOG_INFO "$0" 0 "ITCHIO" "No convert binary; leaving the menu glyph to the theme default"
        return 0
    fi

    find "$THEME_DIR" -type d -name muxapp 2>/dev/null | while IFS= read -r GLYPH_DIR; do
        # Size against a glyph already in this directory. Themes disagree about
        # how big these are — 22, 26, 34 and 47 px are all in use — and guessing
        # is what makes an icon look wrong next to its neighbours. app.png is the
        # generic one every stock theme ships, but a custom theme may not have
        # it, so fall back to any other glyph and skip the directory entirely
        # rather than invent a size.
        GLYPH_SIZE=""
        for REF in "$GLYPH_DIR/app.png" "$GLYPH_DIR"/*.png; do
            [ -f "$REF" ] || continue
            case "$REF" in *"/itchio.png") continue ;; esac
            REF_SIZE="$(identify -format '%w' "$REF" 2>/dev/null)"
            case "$REF_SIZE" in
                '' | *[!0-9]*) continue ;;
            esac
            GLYPH_SIZE="$REF_SIZE"
            break
        done
        if [ -z "$GLYPH_SIZE" ]; then
            LOG_INFO "$0" 0 "ITCHIO" "No reference glyph in $GLYPH_DIR; skipping"
            continue
        fi

        # Rewrite when our artwork is newer, and also when the theme's glyph size
        # has changed since we last ran — a user who resizes their glyphs would
        # otherwise keep our old one at the old size for ever.
        GLYPH_DEST="$GLYPH_DIR/itchio.png"
        if [ -e "$GLYPH_DEST" ] && [ ! "$GLYPH_SRC" -nt "$GLYPH_DEST" ]; then
            DEST_SIZE="$(identify -format '%w' "$GLYPH_DEST" 2>/dev/null)"
            [ "$DEST_SIZE" = "$GLYPH_SIZE" ] && continue
        fi

        if convert "$GLYPH_SRC" -resize "${GLYPH_SIZE}x${GLYPH_SIZE}" "$GLYPH_DEST" 2>/dev/null; then
            LOG_INFO "$0" 0 "ITCHIO" "Installed menu glyph at ${GLYPH_SIZE}px in $GLYPH_DIR"
        else
            LOG_INFO "$0" 0 "ITCHIO" "Could not write menu glyph to $GLYPH_DEST"
        fi
    done
}

INSTALL_GLYPH

# No bundled SDL2 here on purpose: muOS ships its own patched build, and that is
# the one that honours SDL_ROTATION, SDL_HQ_SCALER and SDL_BLITTER_DISABLED.
exec ./"$APP_BIN"

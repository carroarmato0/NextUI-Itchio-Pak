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

# No bundled SDL2 here on purpose: muOS ships its own patched build, and that is
# the one that honours SDL_ROTATION, SDL_HQ_SCALER and SDL_BLITTER_DISABLED.
exec ./"$APP_BIN"

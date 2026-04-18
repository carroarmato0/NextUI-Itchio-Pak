# Itch.io Pak for NextUI

![CI](../../actions/workflows/ci.yml/badge.svg)

A NextUI Pak for TrimUI and MagicX handheld gaming devices that lets you browse, discover, and download Game Boy / Game Boy Color ROM files directly from Itch.io's "made-with-gb-studio" category — all on-device, no PC required.

## Supported Devices

| Device | Platform code |
|---|---|
| TrimUI Brick | `tg5040` |
| TrimUI Smart Pro | `tg5050` |
| MagicX 355M | `my355` |

## Prerequisites

- NextUI installed and running on your device
- WiFi connected (the pak fetches data from Itch.io at runtime)

## Installation

1. Download the latest `.pakz` file from the [Releases](../../releases) page.
2. Extract the contents to the `Tools/` folder on your SD card.
3. Boot into NextUI — the **Itch.io** tool will appear in the Tools menu.

## Usage / Controls

| Button | Action |
|---|---|
| D-pad | Navigate lists |
| A | Select / confirm |
| B | Back |
| Start | Open Settings screen |

## Optional: Itch.io API Key

Free games can be downloaded without authentication. To purchase or download paid games you own, enter your Itch.io API key in the **Settings** screen (press Start from the main list). Generate an API key at <https://itch.io/user/settings/api-keys>.

## Development

### Requirements

- Go 1.22+
- Docker or Podman (for cross-compilation)
- `libsdl2-dev`, `libsdl2-ttf-dev`, `libsdl2-image-dev` (for native headless builds)

### Common commands

```bash
# Run tests (headless, in container)
make test

# Build native binary (requires local SDL2 dev libs)
make build-native

# Cross-compile for all supported platforms
make build-all

# Assemble release zips in dist/
make release

# Deploy to device over ADB
make deploy-adb
```

### Build tags

- `headless` — disables SDL2 rendering; used for CI and unit tests

### Project layout

```
cmd/itchio-pak/       — main binary
internal/
  itchio/             — Itch.io client (RSS feed, game scraping, download)
  renderer/           — SDL2 renderer, image cache, QR
  roms/               — ROM scoring/selection
  settings/           — JSON config
  ui/                 — Screen-based UI (list, detail, download, settings, ROM picker)
scripts/              — build, test, release, deploy, debug helpers
docker/               — dev container image
assets/               — font and other static assets
```

### Contributing

1. Fork the repository and create a feature branch.
2. Make your changes and ensure `make test` passes.
3. Open a pull request — CI will run automatically.

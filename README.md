# Itch.io Pak for NextUI

![CI](../../actions/workflows/ci.yml/badge.svg)

<img src="docs/screenshots/main.png" alt="Game list" width="800"/>

An unofficial community Pak for NextUI on TrimUI and MagicX handheld gaming
devices. Browse, discover, and download Game Boy / Game Boy Color ROM files
directly from itch.io's "made-with-gb-studio" category — all on-device, no PC
required.

> **Disclaimer:** This is an unofficial community project, not affiliated with
> or endorsed by itch.io.

> **Parental advisory:** A built-in content filter blocks game detail pages that
> contain known mature or sensitive tags. It is enabled by default. See
> [Parental Controls](#parental-controls) for details and limitations.

---

## Supported Devices

| Device | Platform code | Status |
|---|---|---|
| TrimUI Brick | `tg5040` | Tested |
| TrimUI Smart Pro | `tg5050` | |
| MagicX 355M | `my355` | |

---

## Features

### Game browsing
- Scrollable list of GB Studio games from itch.io's "made-with-gb-studio" category
- Live cover art thumbnails alongside the list (LRU image cache, loaded in background)
- Paged loading — L/R buttons jump between pages of 36 games
- Total game count displayed in the header

### Game detail
- Cover art and screenshot gallery (L/R to browse)
- Game title, author, price or "Free" badge
- Scrollable description (plain text, converted from the game's HTML)
- QR code for every game — scan to open the itch.io page in a browser
- Download button (A) — disabled for paid games when no API key is set

### Downloading (free games)
- Full free download flow without requiring an itch.io account
- When a game has multiple `.gb`/`.gbc` files, a file picker is shown so you can choose
- Progress bar with percentage and downloaded/total size
- Files saved directly to the correct ROM folder:
  - `.gb` → `Roms/Game Boy (GB)/`
  - `.gbc` → `Roms/Game Boy Color (GBC)/`
- On download failure, a QR code is shown so you can try from a browser

### Parental advisory
- **Mature Content filter** — ON/OFF toggle. Blocks detail pages tagged with explicit adult content
  (`adult`, `gore`, `hentai`, `nsfw`, `nudity`, `porn`, and similar). Default: ON.
- **Sensitive Topics filter** — ON/OFF toggle with per-tag control. Blocks detail pages tagged
  with potentially sensitive topics (`gay`, `gender`, `lesbian`, `lgbtq`, `sexy`, `transgender`).
  Individual tags can be enabled or disabled independently. Default: all ON.
- When a filter triggers, a full-screen "Grown-Ups Only" cover replaces the detail view. Only
  **B** (go back) is available — there is no in-game bypass.
- A parent or guardian can adjust the filters in **Settings** (press Start from any screen).

### Settings
- **API Key** — store your itch.io API key for paid game access (masked in the UI)
- **ROM Selection mode** — `auto` (best file chosen automatically) or `ask` (always show picker)
- **Clear Image Cache** — removes cached cover art from `/tmp`
- **Mature Content** — enable or disable the mature content filter
- **Sensitive Topics** — enable or disable the sensitive topics filter (with per-tag control)

### Controls

| Button | Action |
|---|---|
| D-pad up/down | Navigate list / scroll detail page |
| D-pad up/down (hold) | Auto-scroll with acceleration |
| D-pad L/R | Previous/next page in game list |
| L / R shoulder | Previous/next screenshot in detail view |
| A | Select / confirm / download |
| B | Back |
| Start | Open Settings from any screen |

---

## Installation

1. Download the latest `.pakz` file from the [Releases](../../releases) page.
2. Extract the contents to the `Tools/` folder on your SD card.
3. Boot into NextUI — the **Itch.io** tool will appear in the Tools menu.
4. Connect to WiFi before launching (the pak fetches data from itch.io at runtime).

---

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs/screenshots/game.png" alt="Game detail" width="480"/><br/>
      <sub>Game detail — cover art, screenshots, QR code and description</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/settings.png" alt="Settings" width="480"/><br/>
      <sub>Settings — API key, ROM selection mode, cache management</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/download.png" alt="Download in progress" width="480"/><br/>
      <sub>Download — progress bar with live percentage and size</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/downloaded.png" alt="Download complete" width="480"/><br/>
      <sub>Download complete — saved path shown, ready to play</sub>
    </td>
  </tr>
</table>

---

## Optional: itch.io API Key

Free games can be downloaded without any account or API key.

To download paid games you already own, enter your itch.io API key in the
**Settings** screen (press Start from any screen). Generate an API key at
<https://itch.io/user/settings/api-keys>.

---

## Parental Controls

The pak includes a built-in parental advisory system. When a game's detail page contains tags
from the configured filter lists, a full-screen warning replaces the detail view. The child can
only press **B** to go back — there is no way to continue without a parent disabling the filter
in Settings.

### How it works

Tags are **not available in itch.io's RSS feed**. They are scraped from each game's detail page
when that game is opened. This means filtering only applies at the moment a game is viewed, not
before. The game list itself is always unfiltered.

### Limitations

> **This is not a bulletproof solution.** Parents should be aware of the following:

- **Tag-based, not rating-based** — itch.io has no machine-readable content rating system.
  Filtering relies entirely on tags that game creators choose to apply. A creator can omit tags
  or use non-standard wording, causing content to slip through undetected.
- **Scrape-time only** — tags are fetched when a game detail page is opened. The game list
  is always shown in full; cover art alone may hint at content.
- **Curated tag list** — the filter covers known tags but the list is not exhaustive. New or
  unusual tags will not be caught until a future app update adds them.
- **No PIN protection** — the Settings screen is accessible to anyone who presses Start. A
  determined older child may discover that the filter can be toggled off. The advisory screen
  deliberately gives no hint of this to minimise curiosity.
- **No substitute for supervision** — parental involvement remains the most effective safeguard.

### Filter categories

| Category | Default | Tags covered |
|---|---|---|
| **Mature Content** | ON | `adult`, `boobs`, `eroge`, `erotic`, `femdom`, `gore`, `hentai`, `lewd`, `nsfw`, `nudity`, `porn`, `softcore`, `tits`, `titties`, `xxx`, `yaoi`, `yuri` |
| **Sensitive Topics** | ON (all tags) | `gay`, `gender`, `lesbian`, `lgbtq`, `sexy`, `transgender` |

Sensitive Topics supports per-tag control — a parent can allow some topics while blocking others.

---

## Known Limitations / To-Do

- **Paid game download is not yet implemented.** The API key field and
  ownership check exist, but the paid download path (which requires using
  itch.io's authenticated upload API rather than the free web flow) is not
  complete. Currently a paid game falls back to the free flow, which will fail
  if the game has no free download available. Paid download support is planned.

- **`.pocket` and other non-ROM files** are filtered out — only `.gb` and
  `.gbc` files are shown.

- **No search** — the game list is the full "made-with-gb-studio" category in
  itch.io's default sort order.

- **CSRF token expiry** — if the ROM picker is left open for a long time before
  selecting a file, the resolver may reject the request. Simply back out and
  re-initiate the download.

---

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

# Deploy to a connected device over ADB
make deploy-adb

# Stream the live log from the device
make debug-logs
```

### Build tags

- `headless` — disables SDL2 rendering; used for CI and unit tests

### Project layout

```
cmd/itchio-pak/       — main binary entry point
internal/
  itchio/             — itch.io client: RSS feed, page scraping, download flow
  renderer/           — SDL2 drawing layer, image cache, QR code generation
  roms/               — ROM type detection, destination folder mapping
  settings/           — JSON config read/write
  ui/                 — screen-based UI (list, detail, fetch, ROM picker, download, settings)
lib/{tg5040,my355}/   — bundled SDL2 .so files (tg5050 shares tg5040's libs)
docs/                 — interaction flow reference and screenshots
scripts/              — build, test, release, deploy, debug, screenshot helpers
docker/               — cross-compilation container image
assets/               — font.ttf, CA certificate bundle
testdata/             — captured HTML/RSS fixtures for offline unit tests
```

For a detailed explanation of how the itch.io web API is used, see
[`docs/itchio-interaction-flow.md`](docs/itchio-interaction-flow.md).

### Contributing

1. Fork the repository and create a feature branch.
2. Make your changes and ensure `make test` passes.
3. Open a pull request — CI will run automatically.

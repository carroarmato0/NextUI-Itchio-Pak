# Itch.io Pak for NextUI

![CI](../../actions/workflows/ci.yml/badge.svg)

<img src="docs/screenshots/main.png" alt="Game list" width="800"/>

An unofficial community Pak for NextUI on TrimUI and MagicX handheld gaming
devices. Browse, discover, and download Game Boy / Game Boy Color ROM files
directly from itch.io's "made-with-gb-studio" category — all on-device, no PC
required.

> **Disclaimer:** This is an unofficial community project, not affiliated with
> or endorsed by itch.io.

> **Content filters:** Built-in filters let you block or flag game detail pages
> by theme — mature content, LGBTQ+ content, heavy themes, substance use, and
> suggestive content. Mature Content is on by default; others are opt-in. See
> [Content Filters](#content-filters) for details and limitations.

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

### Content filters
- **Mature Content** is **on by default**; all other filters are opt-in.
- **Mature Content** — blocks explicit adult content. Single on/off toggle.
- **LGBTQ+ Content** — per-tag filter for LGBTQ+ themes and representation.
- **Heavy Themes** — per-tag filter for potentially distressing narrative topics
  (grief, loss, suicide, trauma, abuse, and similar).
- **Substance Use** — single toggle for drug and alcohol themes.
- **Sexual Content** — single toggle for suggestive but non-explicit content.
- When a filter triggers, a full-screen **Content Warning** cover replaces the
  detail view. Press **B** to go back or **Start** to open Settings and adjust
  your filters.
- Filters are configured in **Settings** (press Start from any screen).

### Settings
- **API Key** — store your itch.io API key for paid game access (masked in the UI)
- **ROM Selection mode** — `auto` (best file chosen automatically) or `ask` (always show picker)
- **Clear Image Cache** — removes cached cover art from `/tmp`
- **Mature Content** — block explicit adult content (default: on)
- **LGBTQ+ Content** — filter LGBTQ+ tags with per-tag control (default: off)
- **Heavy Themes** — filter distressing narrative themes with per-tag control (default: off)
- **Substance Use** — filter drug and alcohol themes (default: off)
- **Sexual Content** — filter suggestive non-explicit content (default: off)

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

## Content Filters

The pak includes a built-in content filter system. Filters are useful for anyone
who wants to be aware of — or avoid — specific themes before opening a game,
whether that is a parent managing what their child encounters, or an adult who
prefers not to encounter certain content unexpectedly.

When a game's tags match an active filter, a **Content Warning** screen replaces
the detail view. Press **B** to go back, or **Start** to open Settings and adjust
your filters.

### Configuring filters

Press **Start** from any screen to open **Settings**, then scroll to the content
filter section. Each category can be toggled independently:

- **Mature Content** — covers explicit adult content. Defaults to **on**.
- **LGBTQ+ Content** — covers LGBTQ+ themes and representation. Supports
  per-tag control so you can allow some topics while filtering others.
  Defaults to **off**.
- **Heavy Themes** — covers potentially distressing narrative content: grief,
  loss, suicide, trauma, abuse, and similar. Supports per-tag control.
  Defaults to **off**.
- **Substance Use** — covers drug and alcohol themes. Defaults to **off**.
- **Sexual Content** — covers suggestive but non-explicit content. Defaults
  to **off**.

The specific tags covered by each category are listed and togglable directly
in the Settings screen on the device.

### Limitations

> **Filtering is best-effort, not comprehensive.** Be aware of the following:

- **Tag-based only** — itch.io has no machine-readable content rating system.
  Filtering relies entirely on tags that game creators choose to apply. A
  creator can omit tags or use non-standard wording, and content will not be
  caught.
- **Scrape-time only** — tags are fetched when a game's detail page is opened.
  The game list is always unfiltered; cover art alone may hint at content.
- **Curated tag list** — the filter covers known tags but the list is not
  exhaustive. New or community-specific tags may not be included until a
  future update.
- **No substitute for awareness** — filters reduce unexpected encounters but
  cannot guarantee coverage. When in doubt, check the game's itch.io page
  directly.

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

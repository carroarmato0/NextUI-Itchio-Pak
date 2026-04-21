# Itch.io Pak for NextUI

![CI](../../actions/workflows/ci.yml/badge.svg)
[![Ko-Fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/carroarmato0)

<img src="docs/screenshots/main.png" alt="Game list" width="800"/>

An unofficial community Pak for NextUI on TrimUI and MagicX handheld gaming
devices. Browse, discover, and download Game Boy / Game Boy Color ROM files
directly from itch.io's "made-with-gb-studio" category — all on-device, no PC
required.

> **Disclaimer:** This is an unofficial community project, not affiliated with
> or endorsed by itch.io.

> **Content filters:** Built-in filters let you block or flag game detail pages
> by theme — adult content, queer content, heavy themes, and substance use.
> Adult Content, Heavy Themes, and Substance Use are on by default; Queer Content
> is opt-in. See [Content Filters](#content-filters) for details and limitations.

---

## Supported Devices

| Device | Platform code | Status |
|---|---|---|
| TrimUI Brick | `tg5040` | Tested |
| TrimUI Smart Pro | `tg5040` | Untested |
| TrimUI Smart Pro S | `tg5050` | Untested |
| MagicX 355M | `my355` | Untested |

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

### Downloading
- Download free games without an itch.io account
- Download paid games you already own using your itch.io API key
- When a game has multiple `.gb`/`.gbc` files, a file picker is shown
- Progress bar with live percentage and downloaded/total size
- Files saved directly to the correct ROM folder:
  - `.gb` → `Roms/Game Boy (GB)/`
  - `.gbc` → `Roms/Game Boy Color (GBC)/`
- When **ROM Location** is set to `ask`, a directory browser lets you choose the destination folder before each download; the last chosen path is remembered per file type
- On download failure, a QR code is shown so you can try from a browser

### Content filters
- **Adult Content**, **Heavy Themes**, and **Substance Use** are **on by default**.
  **Queer Content** is opt-in (off by default).
- **Adult Content** — per-tag filter covering explicit and suggestive content
  (nudity, gore, innuendo, and similar).
- **Queer Content** — per-tag filter for LGBTQ+ themes and representation.
- **Heavy Themes** — per-tag filter for potentially distressing narrative topics
  (grief, loss, suicide, trauma, abuse, and similar).
- **Substance Use** — single toggle for drug and alcohol themes.
- When a filter triggers, a full-screen **Content Warning** cover replaces the
  detail view. Press **B** to go back or **Start** to open Settings and adjust
  your filters.
- Filters are configured in **Settings** (press Start from any screen).

### Settings
- **API Key** — shows `FOUND` (green) when an itch.io API key is configured, enabling paid game downloads
- **ROM Selection mode** — `auto` (best file chosen automatically) or `ask` (always show picker)
- **ROM Location** — `auto` (saves to the default folder for the file type) or `ask` (directory browser shown before each download; remembers last path per file type)
- **Clear Image Cache** — removes cached cover art from `/tmp`
- **Content Moderation** — configure per-category content filters
- **About** — app description, version, and QR code linking to the project page

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

There are two release files on the [Releases](../../releases) page. Both support
all devices — the difference is the directory structure inside:

| File | What's inside | Use when |
|---|---|---|
| `Itch-io.pak.zip` | Pak files only (no folder wrapper) | Pak Store install, or manual install where you place it in the right platform folder yourself |
| `Itch-io.pakz` | Full `Tools/<platform>/Itch-io.pak/` tree | Manual install — extract to SD card root and all platforms are set up at once |

### Via the Pak Store (recommended)

Open the Pak Store on your device, find **Itch-io**, and press **A** to install.
The Pak Store downloads and installs `Itch-io.pak.zip` automatically.

### Manual install — `Itch-io.pak.zip`

Use this if you want to install without the Pak Store and prefer to place files
yourself.

1. Download `Itch-io.pak.zip` from the [Releases](../../releases) page.
2. Create the destination folder on your SD card for your device:
   - TrimUI Brick / Smart Pro: `Tools/tg5040/Itch-io.pak/`
   - TrimUI Smart Pro S: `Tools/tg5050/Itch-io.pak/`
   - Miyoo Flip: `Tools/my355/Itch-io.pak/`
3. Extract the contents of the zip **into** that folder (the folder should contain
   `launch.sh`, `itchio-pak`, `pak.json`, etc. directly — not a nested subfolder).
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

### Manual install — `Itch-io.pakz` (all platforms at once)

Use this if you want to set up all supported platforms in one step, or if you
are preparing an SD card that will be used across multiple device types.

1. Download `Itch-io.pakz` from the [Releases](../../releases) page.
2. Rename it to `Itch-io.pakz.zip` (most tools require a `.zip` extension to extract).
3. Extract the contents directly to the **root** of your SD card. The archive
   already contains the correct `Tools/<platform>/Itch-io.pak/` structure.
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

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
  <tr>
    <td align="center">
      <img src="docs/screenshots/content_filters.png" alt="Content filters" width="480"/><br/>
      <sub>Content Moderation — per-category filter toggles in Settings</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/content_warning.png" alt="Content warning" width="480"/><br/>
      <sub>Content Warning — shown instead of detail view when a filter triggers</sub>
    </td>
  </tr>
</table>

---

## itch.io API Key (paid games)

Free games can be downloaded without any account or API key.

To download paid games you already own, you need to configure your itch.io
API key. The Settings screen shows **FOUND** (green) when a key is active.

### Generating your API key

1. Log in to itch.io in a browser.
2. Go to <https://itch.io/user/settings/api-keys>.
3. Click **Generate new API key** and copy the key.

### Adding the key to the Pak

The Pak does not include an on-screen keyboard, so the key is set by editing
the config file directly. The easiest way is via ADB while your device is
connected over USB:

```sh
# TrimUI Brick / Smart Pro (tg5040 / tg5050)
adb shell 'cat > /mnt/SDCARD/.userdata/shared/Itch-io/config.json' << 'EOF'
{
  "api_key": "YOUR_API_KEY_HERE",
  "rom_selection": "auto"
}
EOF
```

You can also copy the file to your SD card directly if you prefer not to use
ADB. The config file is at `.userdata/shared/Itch-io/config.json` relative to
the SD card root.

Restart the Pak after saving — the Settings screen will show **API Key: FOUND**
once the key is loaded.

---

## Content Filters

The pak includes a built-in content filter system. Filters are useful for anyone
who wants to be aware of — or avoid — specific themes before opening a game,
whether that is someone who prefers not to encounter certain content
unexpectedly, or a parent managing what their child encounters.

When a game's tags match an active filter, a **Content Warning** screen replaces
the detail view. Press **B** to go back, or **Start** to open Settings and adjust
your filters.

### Configuring filters

Press **Start** from any screen to open **Settings**, then scroll to the content
filter section. Each category can be toggled independently:

- **Adult Content** — covers explicit and suggestive material (nudity, gore,
  innuendo, and similar). Supports per-tag control. Defaults to **on**.
- **Heavy Themes** — covers potentially distressing narrative content: grief,
  loss, suicide, trauma, abuse, and similar. Supports per-tag control.
  Defaults to **on**.
- **Substance Use** — covers drug and alcohol themes. Defaults to **on**.
- **Queer Content** — covers LGBTQ+ themes and representation. Supports
  per-tag control so you can allow some topics while filtering others.
  Defaults to **off** (opt-in).

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

- **No in-app keyboard for API key entry.** The API key must be set by editing
  `config.json` directly (see [itch.io API Key](#itch-io-api-key-paid-games)).

- **`.pocket` and other non-ROM files** are filtered out — only `.gb` and
  `.gbc` files are shown.

- **No search** — the game list is the full "made-with-gb-studio" category in
  itch.io's default sort order.

- **CSRF token expiry** — if the ROM picker is left open for a long time before
  selecting a file, the resolver may reject the request. Back out and
  re-initiate the download.

- **Free download scraping is brittle** — itch.io can change its page structure
  without notice, which would break the free download flow. The paid API path
  is more stable.

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

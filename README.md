# Itch-io — browse and download itch.io games on your handheld

![CI](../../actions/workflows/ci.yml/badge.svg)
[![Ko-Fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/carroarmato0)

<img src="docs/screenshots/main.png" alt="Game list" width="800"/>

An unofficial community app for **NextUI** and **muOS** on TrimUI and Miyoo Flip
handheld gaming devices. Browse, discover, and download homebrew ROM games for
Game Boy, Game Boy Color, Game Boy Advance, NES/Famicom, Sega Genesis, and
Pico-8 directly from itch.io — all on-device, no PC required.

> **Disclaimer:** This is an unofficial community project, not affiliated with
> or endorsed by itch.io.

> **Content filters:** Built-in filters let you block or flag game detail pages
> by theme — adult content, queer content, heavy themes, and substance use.
> Adult Content, Heavy Themes, and Substance Use are on by default; Queer Content
> is opt-in. See [Content Filters](#content-filters) for details and limitations.

---

## Supported Firmware and Devices

### NextUI

| Device | Platform code | Status |
|---|---|---|
| TrimUI Brick | `tg5040` | Tested |
| TrimUI Smart Pro | `tg5040` | Tested |
| TrimUI Smart Pro S | `tg5050` | Tested |
| Miyoo Flip | `my355` | Tested |

### muOS

One ARM64 build covers every muOS device, because muOS ships its own SDL2 and
the binary needs nothing newer than glibc 2.17.

| Device | Status |
|---|---|
| TrimUI Smart Pro | Tested |
| Other muOS devices | Should work; untested |

---

## NextUI and muOS

Itch-io does the same job on both, and most of this README applies to either.
The two firmwares organise storage differently, though, and a few features only
exist on one of them.

| | NextUI | muOS |
|---|---|---|
| ROMs go to | `Roms/<System>/`, fixed names | a folder under `ROMS/` — see below |
| Box art | `.media/` beside the ROM | `MUOS/info/catalogue/<System>/box/` |
| Soundtracks | `Music/<Game>/` | `MUOS/music/<Game>/` |
| Settings, inventory, cache | `.userdata/shared/Itch-io/` | `data/` inside the application folder |
| Follows the system colour palette | yes | no — uses its own theme |
| Save and save-state migration | yes | no |
| Choice of GBA emulator folder | yes | no — muOS picks the core |
| Choice of Pico-8 runtime | yes | no — one Pico-8 folder |

### ROM folders on muOS

muOS has no required folder names — the documentation says folders "can be named
whatever you want" — so Itch-io looks for one you already have before making its
own. For a Game Boy download it will use `gb`, `Nintendo Game Boy`, `Game Boy` or
`gameboy`, whichever exists, on either SD card. Only if none exists does it
create muOS's own short name: `gb`, `gbc`, `gba`, `nes`, `md` or `pico8`.

A folder it creates has no emulator assigned yet. That is muOS's own behaviour —
the first time you launch something from a new folder it asks you to pick a core.

### Why some features are missing on muOS

They are switched off rather than approximated. Save migration is the clearest
case: muOS assigns an emulator core per folder, chosen by you *after* the ROM is
already in place, so at download time there is nothing to derive a save path
from. Writing a guess would put files somewhere you would never find them, which
is worse than not writing them at all.

### The menu icon

muOS resolves application icons from the active theme rather than from the
application itself, so Itch-io installs a correctly sized icon into your active
theme's `glyph/muxapp/` folders the first time it runs. Switch theme and it
reinstalls itself on the next launch.

---

## Features

### Game browsing
- Scrollable list of homebrew ROM games sourced from multiple itch.io tag feeds, merged and deduplicated into a single catalogue covering Game Boy, Game Boy Color, Game Boy Advance, NES/Famicom, Sega Genesis, and Pico-8
- Live cover art thumbnails alongside the list with support for animated GIF cover art
- Pages of 36 games — D-pad automatically turns the page at the top or bottom of the list; D-pad Left/Right jumps pages directly
- On first launch the list loads live from the network while a full cache is built in the background
- On subsequent launches the full game list loads instantly from the on-device cache
- Cache auto-refreshes after 24 hours; manual refresh available in Settings
- Total game count displayed in the header
- Games already downloaded to the device are marked with a `[DL]` badge
- When a background check detects a new upstream file for a downloaded game, its badge changes to `[UP]` — press **X** from the game list to dismiss the notification
- If a downloaded game has been removed from itch.io (HTTP 404/410), its badge changes to `[!]` — press **X** to dismiss

### Sorting, filtering and search

Press **SELECT** from the game list to open the **Filter & Search** overlay. From there you can:

- **Platform** — show games for a specific system (All, GB, GBC, GBA, NES, MD, Pico-8) or all at once
- **Sort** — choose a sort mode:

  | Mode | Description |
  |---|---|
  | RSS | Feed order — newest from itch.io (default) |
  | A-Z | Alphabetical ascending |
  | Z-A | Alphabetical descending |
  | New | By publication date, newest first |
  | DL | Downloaded only — pending-update games (`[UP]`) first, removed (`[!]`) second, then the rest |
  | Free | Free games only |
  | Paid | Paid games only |
  | Owned | Owned games only |

- **Search** — free-text search across game titles and authors; press **A** on the search field to open the virtual keyboard

Press **SELECT** again (or **A** from the keyboard) to apply. Press **B** to dismiss without changes, or **Y** to clear all active filters.

The active platform and sort mode are shown as pills in the header and saved automatically for the next launch. In A-Z / Z-A mode, **L1/R1** jump directly to the next/previous letter boundary. In all other modes, **L1/R1** cycle through sort modes.

### Game detail
- Cover art and screenshot gallery (L/R to browse); animated GIF cover art plays inline
- Game title, author, price or "Free" badge
- Scrollable description with basic HTML formatting preserved (paragraphs, headings, bullet and numbered lists)
- QR code for every game — scan to open the itch.io page in a browser
- Download button (A) — disabled for paid games when no API key is set
- Downloaded files are listed with their on-device paths; press **Y** to manage, delete, or toggle title-based filename for the game
- Game titles and descriptions in non-Latin scripts render correctly — the bundled font set covers Arabic, Cyrillic, Devanagari, Hebrew, Japanese/CJK, and Thai automatically, with no configuration required

### Downloading
- Download free games without an itch.io account
- Download paid games you already own using your itch.io API key
- When a game has multiple ROM files, a file picker is shown
- Progress bar with live percentage and downloaded/total size
- Files saved directly to the correct ROM folder:
  - `.gb` → `Roms/Game Boy (GB)/`
  - `.gbc` → `Roms/Game Boy Color (GBC)/`
  - `.gba` → `Roms/Game Boy Advance (GBA)/`
  - `.nes` → `Roms/Nintendo Entertainment System (FC)/`
  - `.md` / `.gen` / `.smd` → `Roms/Sega Genesis (MD)/`
  - `.p8` / `.p8.png` → `Roms/Pico-8 (P8)/` or `Roms/Pico-8 (PICO)/` depending on the configured **Pico-8 Core** (see Settings)
- Multi-file Pico-8 games (games that ship as several `.p8` carts) are extracted into their own subdirectory inside the Pico-8 ROM folder, preserving the relative paths from the ZIP
- When **ROM Location** is set to `ask`, a directory browser lets you choose the destination folder before each download; the last chosen path is remembered per file type
- On download failure, a QR code is shown so you can try from a browser
- **Bundle purchases** — if you own a game both individually and as part of one or more itch.io bundles, a purchase picker lists each transaction (labelled `Individual purchase` or `Bundle: <name>`) so you can choose which to download from

### Game management
- Downloaded games are tracked in an on-device inventory
- From the game detail screen, press **Y** to delete downloaded ROMs:
  - Single-file games show a confirmation prompt with the filename and path
  - Multi-file games open a **Manage Downloads** screen where you can delete files individually or all at once with **Delete all**
- After deletion the `[DL]` badge is removed and the Download button becomes available again

### Unified naming
When **Use game title as filename** is enabled (the default), downloaded ROMs are automatically renamed to match the game's title on itch.io. For example, a file named `gb-studio-export.gb` becomes `Doomslinger Dungeon.gb`.

- **Global toggle** — in Settings, **Use game title as filename** turns the feature on or off for all future downloads
- **Per-game toggle** — press **Y** from a game's detail screen to enable or disable title-based naming for that specific game; this option appears only when a download exists and the global toggle is on
  - Multi-file games have a **Use game title as filename** toggle row at the bottom of the **Manage Downloads** screen
- When toggling a game that already has a ROM on device, a guided flow offers to rename the existing file and — if save data is detected — rename the matching SRAM save and save states at the same time
  - Saves and states can be renamed or skipped independently; skipped files are left at their original paths and will need to be renamed manually before the emulator can load them
- If two different games produce the same sanitised filename, a ` (2)` suffix is appended automatically to avoid collisions

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

### Theming

Itch-io can follow NextUI's own colour palette, so it looks like part of the
system rather than a separate app. Turn it on with **NextUI Theme** in Settings.

<img src="docs/screenshots/theme-macchiato.png" alt="Itch-io using the Catppuccin Macchiato palette" width="800"/>

- Reads the active palette from NextUI's own settings — no configuration in Itch-io
- Works with every palette NextUI ships, including the light ones, and with any
  custom palette you drop into `Palettes/` on the SD card
- The Settings row names the palette in use, e.g. `NextUI Theme: On (Catppuccin Macchiato)`
- Selection highlights, list text, header and footer bars, pills and hint text all
  follow the palette; status badges tint to it too
- Update (`[UP]`) and error (`[!]`) badges deliberately keep their amber and red so
  they still stand out whatever the palette
- Switch palettes in NextUI and Itch-io picks up the change the next time it starts
- Leave the setting off to keep Itch-io's own dark theme

<table>
  <tr>
    <td align="center">
      <img src="docs/screenshots/themes-mosaic.png" alt="The game list shown in four different NextUI palettes" width="960"/><br/>
      <sub>The same screen under four NextUI palettes — Catppuccin Macchiato, Catppuccin Latte, Catppuccin Mocha and Teal Powder</sub>
    </td>
  </tr>
</table>

### Power management

The power button behaves the same way it does with emulators on NextUI:

- **Short press** — device goes to sleep; Itch-io stays in memory and resumes exactly where you left it when you wake the device.
- **Hold 2 seconds** — device shuts down cleanly.

If a background task (ROM download, game list cache build, inventory check) is running when you press the power button, a full-screen **"Please wait"** overlay is shown until the task finishes. The action fires automatically — no confirmation or extra button press needed.

### Settings
- **API Key** — shows `WORKING` (green) when an itch.io API key is configured and validated. Press **A** on the row to enter a new key using the built-in virtual keyboard; when a key is already set, press **A** to re-run the validation test or press **Y** to edit the key
- **ROM Selection mode** — `auto` (best file chosen automatically) or `ask` (always show picker)
- **ROM Location** — `auto` (saves to the default folder for the file type) or `ask` (directory browser shown before each download; remembers last path per file type)
- **Pico-8 Core** — selects which Pico-8 emulator downloaded `.p8` / `.p8.png` files are destined for:
  - `FakeO8 (default)` — saves to `Roms/Pico-8 (P8)/`, used by NextUI's built-in FakeO8 core. Free to use; compatible with most single-cartridge games.
  - `Pico-8 (official)` — saves to `Roms/Pico-8 (PICO)/`, used by the [minui-pico-8-pak](https://github.com/josegonzalez/minui-pico-8-pak). Requires a **paid copy of Pico-8** (a licensed BIOS file must be present); in return it offers broader game compatibility and full **multi-cart support** for games that ship as several linked cartridges.

  > **Note:** Some Pico-8 games on itch.io are designed to run only inside the official Pico-8 runtime and will not work correctly under FakeO8. If a game behaves incorrectly or refuses to start, try switching to the official core.

  Switching cores instantly moves all previously downloaded Pico-8 files (ROMs and cover art) to the new folder — no manual file management needed. Switching back moves them back.
- **Use game title as filename** — when `ON` (default), downloaded ROMs are renamed to match the itch.io game title; set to `OFF` to keep the original upload filename
- **NextUI Theme** — when `On`, Itch-io follows the colour palette configured in
  NextUI, and the row shows which one is active. Only appears when NextUI's
  settings file is present. Defaults to `Off`
- **Log Level** — `Info` (default) records key events and all errors. Set to `Debug` to capture the full HTTP request/response flow — useful when reporting a bug involving a download failure or a feed that won't load. The log file is written to `.userdata/<platform>/logs/itchio.log` on the SD card.
- **Clear Image Cache** — removes cached cover art from `/tmp`
- **Refresh Game List** — re-fetches the full game list from itch.io across all platform feeds with a live progress screen showing how many games have been retrieved; the cache is updated on completion. Press **B** at any time to cancel the fetch cleanly — no partial cache is written
- **Update Inventory** — manually triggers a background check for new upstream files, removed games, and missing cover art across all inventory entries; the right side of the row shows when the last check ran (`just now`, `Xm ago`, `Xh ago`, or `Xd ago`) or `never` if no check has run yet
- **Content Moderation** — configure per-category content filters
- **About** — app description, version, and QR code linking to the project page

### Controls

| Button | Action |
|---|---|
| D-pad up/down | Navigate list / scroll detail page |
| D-pad up/down (hold) | Auto-scroll with acceleration |
| D-pad left / right | Jump one page forward/back in game list; previous/next screenshot in detail view |
| L1 / R1 (game list, A-Z mode) | Jump to previous/next letter boundary |
| L1 / R1 (game list, other modes) | Cycle sort mode backward/forward |
| A | Select / confirm / download |
| B | Back / cancel |
| X | Dismiss update (`[UP]`) or removal (`[!]`) notification for the selected game |
| Y | Manage / delete downloaded ROMs, or edit API key in Settings |
| SELECT | Open Filter &amp; Search overlay (game list) |
| Start | Open Settings from any screen |
| Power (short press) | Sleep — resumes at the same screen on wake |
| Power (hold 2 s) | Shutdown — waits for active tasks to finish first |

---

## Installation

Release files on the [Releases](../../releases) page are named after the firmware
they are for. Pick the row that matches your device:

| File | Firmware | What's inside | Use when |
|---|---|---|---|
| `Itch-io.pak.zip` | NextUI | Pak files only (no folder wrapper) | Pak Store install |
| `Itch-io.NextUI.<version>.pak.zip` | NextUI | Pak files only (no folder wrapper) | Manual install where you place the files in the right platform folder yourself |
| `Itch-io.NextUI.<version>.pakz` | NextUI | Full `Tools/<platform>/Itch-io.pak/` tree | Manual install — extract to SD card root and all platforms are set up at once |
| `Itch-io.muOS.<version>.muxapp` | muOS | The application directory | Install through muOS's Archive Manager |

The two NextUI `.pak.zip` files are identical; the unversioned name exists
because that is what the Pak Store fetches.

### Via the Pak Store (recommended)

Open the Pak Store on your device, find **Itch-io**, and press **A** to install.
The Pak Store downloads and installs `Itch-io.pak.zip` automatically.

### Manual install — `Itch-io.NextUI.<version>.pak.zip`

Use this if you want to install without the Pak Store and prefer to place files
yourself.

1. Download `Itch-io.NextUI.<version>.pak.zip` from the [Releases](../../releases) page.
2. Create the destination folder on your SD card for your device:
   - TrimUI Brick / Smart Pro: `Tools/tg5040/Itch-io.pak/`
   - TrimUI Smart Pro S: `Tools/tg5050/Itch-io.pak/`
   - Miyoo Flip: `Tools/my355/Itch-io.pak/`
3. Extract the contents of the zip **into** that folder (the folder should contain
   `launch.sh`, `itchio`, `pak.json`, etc. directly — not a nested subfolder).
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

### Manual install — `Itch-io.NextUI.<version>.pakz` (all platforms at once)

Use this if you want to set up all supported platforms in one step, or if you
are preparing an SD card that will be used across multiple device types.

1. Download `Itch-io.NextUI.<version>.pakz` from the [Releases](../../releases) page.
2. Rename it to end in `.zip` (most tools require a `.zip` extension to extract).
3. Extract the contents directly to the **root** of your SD card. The archive
   already contains the correct `Tools/<platform>/Itch-io.pak/` structure.
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

### muOS install

1. Download `Itch-io.muOS.<version>.muxapp` from the [Releases](../../releases) page.
2. Copy it into the `ARCHIVE` folder on your SD card — do not rename it or
   unpack it; muOS installs it as-is.
3. On the device: **Applications → Archive Manager**, select the file, press **A**.
4. **Itch.io** now appears under Applications.
5. Connect to Wi-Fi before launching.

Updating works the same way: install the new `.muxapp` over the old one. Your
settings, inventory and caches live in the application's own `data/` folder and
survive both an update and a muOS system update.


---

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs/screenshots/filter-search.png" alt="Filter and search" width="480"/><br/>
      <sub>Filter &amp; Search — platform, sort mode, and free-text search in one overlay</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/keyboard.png" alt="Virtual keyboard" width="480"/><br/>
      <sub>Virtual keyboard — used for search and API key entry; ABC/abc/0-9 pages</sub>
    </td>
  </tr>
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
  <tr>
    <td align="center">
      <img src="docs/screenshots/adult_filter.png" alt="Adult content filter" width="480"/><br/>
      <sub>Adult Content filter — per-tag toggles</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/heavy_filter.png" alt="Heavy themes filter" width="480"/><br/>
      <sub>Heavy Themes filter — per-tag toggles</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/queer_filter.png" alt="Queer content filter" width="480"/><br/>
      <sub>Queer Content filter — per-tag toggles</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/bundle.png" alt="Bundle purchase picker" width="480"/><br/>
      <sub>Bundle purchase picker — choose which transaction to download from</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/delete.png" alt="Delete confirmation" width="480"/><br/>
      <sub>Delete confirmation — shown before removing a ROM from the device</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/theme-macchiato.png" alt="NextUI theme applied" width="480"/><br/>
      <sub>NextUI theme — Itch-io following the device's Catppuccin Macchiato palette</sub>
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

### Adding the key to Itch-io

#### Option 1 — Built-in virtual keyboard (recommended)

Open **Settings** (press **Start** from any screen), navigate to **API Key**,
and press **A**. A virtual keyboard appears where you can type the key
directly on-device. Confirm with the **OK** key; Itch-io validates the key
immediately and shows `WORKING` on success.

To update an existing key, navigate to **API Key** in Settings and press **Y**
to open the virtual keyboard pre-filled with the current value.

#### Option 2 — Browser-based file manager

With your device connected over USB, open
**[https://dashboard.loveretro.games/](https://dashboard.loveretro.games/)**
in a browser. Use the built-in file manager to navigate to
`.userdata/shared/Itch-io/config.json` and edit the `"api_key"` field directly
— no command line required, and only the fields you change are affected.

#### Option 3 — SD card

1. Power off the device and remove the SD card.
2. Open `.userdata/shared/Itch-io/config.json` in a text editor.
3. Add or update the `"api_key"` field, leaving all other fields untouched.
4. Save, reinsert the SD card, and boot.

#### Option 4 — ADB (pull → edit → push)

```sh
# Download the current config to your computer
adb pull /mnt/SDCARD/.userdata/shared/Itch-io/config.json config.json

# Open config.json in a text editor, add or update the "api_key" field:
#   "api_key": "YOUR_API_KEY_HERE"
# Leave all other fields unchanged, then push the file back:
adb push config.json /mnt/SDCARD/.userdata/shared/Itch-io/config.json
```

If you have never launched Itch-io and no config file exists yet, you can
create a minimal one:

```json
{
  "api_key": "YOUR_API_KEY_HERE"
}
```

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

- **Cloudflare may block RSS feed requests (HTTP 403).** itch.io is protected by
  Cloudflare, which uses browser fingerprinting to detect bot traffic. Despite the
  app mimicking Chrome headers and TLS fingerprint, Cloudflare may still issue a
  temporary block — especially on networks or IP ranges it has not seen before.

  **What you will see:** An error message on the game list or cache-refresh screen
  that reads *"Cloudflare blocked the request (HTTP 403)"*.

  **What you can do:**
  1. **Visit itch.io in a browser on the same WiFi network.** Your device and your
     phone or laptop share the same public IP address. If Cloudflare presents a
     human-verification challenge in the browser and you pass it, the IP is marked
     as human traffic. Return to Itch-io and press **A** (game list) or retry the
     refresh — it will often succeed immediately afterwards.
  2. **Wait a few minutes and retry.** Cloudflare challenges are sometimes
     temporary. Itch-io retries the request each time you press **A** on the error
     screen.
  3. **Try a different network.** Switching WiFi networks (e.g. a mobile hotspot)
     gives Itch-io a fresh public IP that may not be challenged.

- **Animated GIF thumbnail conversion is best-effort.** When a game's cover art
  is an animated GIF, a static PNG thumbnail is derived from it automatically
  using a colour-variance heuristic (the frame with the most colour diversity is
  chosen). The result depends on how the game developer structured the GIF — a
  GIF that opens on a black frame, uses unusual disposal methods, or has few
  visually distinct frames may still produce a poor thumbnail.

- **Some Pico-8 games have no downloadable files.** A number of Pico-8 titles on
  itch.io are published as browser-only experiences with no file uploads at all.
  When this happens Itch-io shows a clear "No downloads available" message with a
  QR code so you can open the game's itch.io page and play it in a browser instead.

- **Multi-cart Pico-8 games require the official Pico-8 core.** Games that ship as
  several linked `.p8` or `.p8.png` cartridges (using Pico-8's built-in cart-chaining
  mechanism) can only be launched correctly by the official Pico-8 runtime. FakeO8
  does not support cart-chaining; attempting to play a multi-cart game under FakeO8
  will typically only run the first cartridge. Switch to the **Pico-8 (official)**
  core in Settings — this requires a paid copy of Pico-8 with the BIOS file in place.

- **`.pocket` and other non-ROM files** are filtered out — only `.gb`, `.gbc`,
  `.gba`, `.nes`, `.md`, `.gen`, `.smd`, `.p8`, and `.p8.png` files are shown.

- **CSRF token expiry** — if the ROM picker is left open for a long time before
  selecting a file, the resolver may reject the request. Back out and
  re-initiate the download.

- **Free download scraping is brittle** — itch.io can change its page structure
  without notice, which would break the free download flow. The paid API path
  is more stable.

- **"Pay What You Want" games with a mandatory minimum price show as free.**
  itch.io reports a price of `0` in its RSS feed for games configured as
  "name your price", even when the creator has set a non-zero minimum purchase
  amount. Itch-io cannot distinguish these from genuinely free games using feed
  data alone, so they are labelled **Free** and appear in the `[FREE]` filter.
  You can recognise them on itch.io by their **"Download Now"** button (instead
  of "Buy Now") and a note that a minimum purchase price is required. If you
  attempt to download one without owning it, the download will fail and a QR
  code will be shown so you can complete the purchase in a browser first.

- **Unified naming and save format changes** — the save/state migration flow reads your current `saveFormat` and `stateFormat` settings from `minuisettings.txt` at the time of migration. If you later change the save format in NextUI's settings, existing saves and states will not be automatically re-migrated to the new naming scheme. You would need to rename them manually or re-run the per-game toggle to trigger the flow again.

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

# Cross-compile every target (all firmwares, all devices)
make build-all

# Cross-compile one firmware, or one target
./scripts/build.sh nextui
./scripts/build.sh nextui/tg5040

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
  power/              — power button detection via evdev; sleep/shutdown callback
  renderer/           — SDL2 drawing layer, image cache, QR code generation
  roms/               — ROM type detection, destination folder mapping
  settings/           — JSON config read/write
  ui/                 — screen-based UI (list, detail, fetch, ROM picker, download, settings)
bin/<firmware>/<device>/ — built binaries, e.g. bin/nextui/tg5040/itchio
lib/<toolchain>/      — SDL2 .so files harvested from each toolchain sysroot
docs/                 — interaction flow reference and screenshots
scripts/              — build, test, release, deploy, debug, screenshot helpers
scripts/targets.sh    — single source of truth for firmware x device build targets
docker/               — cross-compilation container image
assets/               — font.ttf, CA certificate bundle
testdata/             — captured HTML/RSS fixtures for offline unit tests
```

For a detailed explanation of how the itch.io web API is used, see
[`docs/itchio-interaction-flow.md`](docs/itchio-interaction-flow.md).

### Branches

| Branch | Holds |
|---|---|
| `main` | What has been released. Only ever updated by merging `dev` for a release. |
| `dev` | Work accumulating towards the next release. May be ahead of the last release. |
| `feature/*` | One change in progress, branched from `dev` and merged back into it. |

Work goes `feature/* → dev → main`. `main` therefore always matches the newest
stable release, which is what the Pak Store installs.

Pre-release builds for testers are cut from `dev` and tagged `vX.Y.Z-rcN`:

```bash
./scripts/release.sh                          # test, build every target, package
./scripts/release-github.sh --dry-run --prerelease
./scripts/release-github.sh --prerelease      # publish
```

`--prerelease` marks the GitHub release *and* withholds the unversioned
`Itch-io.pak.zip`. The Pak Store finds updates by matching that filename against
a release's assets, so a build without it cannot reach people on a stable
release — which does not depend on the store also honouring GitHub's
pre-release flag.

A full release is the same commands without `--prerelease`, run on `main` after
merging `dev` and bumping the version in `pak.json`.

### Contributing

1. Fork the repository and create a feature branch from `dev`.
2. Make your changes and ensure `make test` passes.
3. Open a pull request against `dev` — CI will run automatically.

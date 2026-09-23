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

---

## Install

**NextUI, with the Pak Store:** find **Itch-io** and press **A**. Done.

**Everything else** — manual NextUI installs, muOS, and the full device list —
is in the [installation guide](docs/install.md).

| Firmware | Devices |
|---|---|
| NextUI | TrimUI Brick, Smart Pro, Smart Pro S, Miyoo Flip, Anbernic RG XX family |
| muOS | Every muOS device (one ARM64 build) |

Connect to WiFi before launching.

---

## What it does

- **Browse thousands of homebrew games** from itch.io, merged from multiple tag
  feeds into one catalogue — GB, GBC, GBA, NES, Genesis and Pico-8
- **Download straight to the right ROM folder**, with cover art, so games are
  ready to play without touching a PC
- **Search, sort and filter** by platform, name, date, or whether you own or
  have already downloaded a game
- **Paid games you own** download too, using your [itch.io API key](docs/api-key.md)
- **Name-your-own-price games show the developer's suggested amount**, so you
  can decide whether to support them
- **Tracks what you have downloaded** and flags games with updates (`[UP]`) or
  that vanished from itch.io (`[!]`)
- **Renames files to the game's real title**, so `gb-studio-export.gb` becomes
  `Doomslinger Dungeon.gb` — saves and save states can follow
- **Follows your NextUI colour palette**, including light themes
- **Content filters** for adult, heavy, substance and queer themes — the first
  three on by default, queer content opt-in
- **Sleeps and resumes** with the power button, like any emulator

The [feature reference](docs/features.md) covers all of it in detail, including
the caveats.

---

## Controls

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

## Documentation

| Guide | Covers |
|---|---|
| [Installation](docs/install.md) | Every install method, supported devices, NextUI vs muOS differences |
| [Feature reference](docs/features.md) | Everything the app does, and the limits of each feature |
| [API key setup](docs/api-key.md) | Four ways to add your itch.io key for paid games |
| [Development](docs/development.md) | Building, testing, releasing, project layout, contributing |

---

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs/screenshots/game.png" alt="Game detail" width="480"/><br/>
      <sub>Game detail — cover art, screenshots, QR code and description</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/filter-search.png" alt="Filter and search" width="480"/><br/>
      <sub>Filter &amp; Search — platform, sort mode, and free-text search in one overlay</sub>
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
      <img src="docs/screenshots/settings.png" alt="Settings" width="480"/><br/>
      <sub>Settings — API key, ROM selection mode, cache management</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/theme-macchiato.png" alt="NextUI theme applied" width="480"/><br/>
      <sub>NextUI theme — Itch-io following the device's Catppuccin Macchiato palette</sub>
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

<img src="docs/screenshots/themes-mosaic.png" alt="The game list shown in four different NextUI palettes" width="960"/>

<sub>The same screen under four NextUI palettes — Catppuccin Macchiato, Catppuccin Latte, Catppuccin Mocha and Teal Powder</sub>

---

## Support

Found a bug, or got an Anbernic RG XX to test on? [Open an issue](../../issues) —
H700 reports are especially welcome, since nobody working on Itch-io owns one.

If you find this useful, you can [buy me a coffee](https://ko-fi.com/carroarmato0).

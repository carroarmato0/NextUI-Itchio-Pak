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
- **Paid games are listed for everyone**, and the ones you own download too:
  [sign in with your phone](docs/sign-in.md) by scanning a QR code — nothing to type
- **Name-your-own-price games show the developer's suggested amount**, so you
  can decide whether to support them
- **Tracks what you have downloaded** and flags games with updates (`[UP]`) or
  that vanished from itch.io (`[!]`)
- **Renames files to the game's real title**, so `gb-studio-export.gb` becomes
  `Doomslinger Dungeon.gb` — saves and save states can follow
- **Follows your NextUI colour palette**, including light themes
- **Content filters** for adult, heavy, substance and queer themes — the first
  three on by default, queer content opt-in
- **Keeps working when Wi-Fi drops** — cached games and covers stay usable, an
  `Offline` chip says why nothing new arrives, and everything picks up by itself
  when the connection is back
- **Tells you when a new version is out**, with a small notice and
  **Settings → App updates** — Stable or release-candidate channel. On muOS it
  can save the update for Archive Manager to install
- **Sleeps and resumes** with the power button, like any emulator

The [feature reference](docs/features.md) covers all of it in detail, including
the caveats.

---

## Controls

| Button | Action |
|---|---|
| D-pad up/down | Navigate list / scroll detail page / move through Settings |
| D-pad up/down (hold) | Auto-scroll with acceleration |
| D-pad left / right | Jump one page forward/back in game list; previous/next screenshot in detail view |
| L1 / R1 (game list, A-Z mode) | Jump to previous/next letter boundary |
| L1 / R1 (game list, other modes) | Cycle sort mode backward/forward |
| L1 / R1 (Settings) | Jump to the previous/next section |
| A | Select / confirm / download |
| B | Back / cancel (also cancels a Save to ARCHIVE download) |
| X | Dismiss update (`[UP]`) or removal (`[!]`) notification for the selected game |
| Y | Manage / delete downloaded ROMs, or sign out in Settings |
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
| [Signing in](docs/sign-in.md) | Sign in with a QR code to download paid games you own |
| [App updates](docs/features.md#app-updates) | The update notice, channels, the Pak Store and muOS Save to ARCHIVE |
| [Development](docs/development.md) | Building, testing, releasing, project layout, contributing |

---

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs/screenshots/game.png" alt="Game detail" width="480"/><br/>
      <sub>Game detail — cover art, screenshots, QR code and the developer's suggested donation</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/paid-game.png" alt="A paid game you do not own" width="480"/><br/>
      <sub>A paid game you don't own — its price, and a QR code to buy it on itch.io</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/settings.png" alt="Settings" width="480"/><br/>
      <sub>Settings — grouped into sections, values on the right; L1/R1 jump between sections</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/signin.png" alt="Sign in with a QR code" width="480"/><br/>
      <sub>Sign in — scan the QR code with your phone and check the code matches</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/update-notice.png" alt="Update notice" width="480"/><br/>
      <sub>Update notice — shown once per new version, top right</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/app-updates.png" alt="Settings, App updates" width="480"/><br/>
      <sub>App updates — channel, check now, and the release notes as a QR code</sub>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/screenshots/filter-search.png" alt="Filter and search" width="480"/><br/>
      <sub>Filter &amp; Search — platform, sort mode, and free-text search in one overlay</sub>
    </td>
    <td align="center">
      <img src="docs/screenshots/download.png" alt="Download in progress" width="480"/><br/>
      <sub>Download — progress bar with live percentage and size</sub>
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

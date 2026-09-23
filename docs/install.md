# Installing Itch-io

[← Back to the README](../README.md)

Itch-io runs on **NextUI** and **muOS**. Pick the section for your firmware.

---

## Supported devices

### NextUI

| Device | Platform code | Status |
|---|---|---|
| TrimUI Brick | `tg5040` | Tested |
| TrimUI Smart Pro | `tg5040` | Tested |
| TrimUI Smart Pro S | `tg5050` | Tested |
| Miyoo Flip | `my355` | Tested |
| Anbernic RG28XX, RG34XX, RG34XX SP, RG SP | `h700` | Untested — please report |
| Anbernic RG35XX Plus / H / SP / Pro | `h700` | Untested — please report |
| Anbernic RG40XX H, RG40XX V, RG Cube XX | `h700` | Untested — please report |

H700 support requires a NextUI build that includes it. Nobody working on
Itch-io owns one of these handhelds, so every H700 report is genuinely useful —
the log at `.userdata/h700/logs/itchio.log` records the model, the screen size
and which buttons you pressed.

### muOS

One ARM64 build covers every muOS device, because muOS ships its own SDL2 and
the binary needs nothing newer than glibc 2.17.

| Device | Status |
|---|---|
| TrimUI Smart Pro | Tested |
| Other muOS devices | Should work; untested |

---

## Which file do I need?

Release files on the [Releases](../../../releases) page are named after the
firmware they are for.

| File | Firmware | What's inside | Use when |
|---|---|---|---|
| `Itch-io.pak.zip` | NextUI | Pak files only (no folder wrapper) | Pak Store install |
| `Itch-io.NextUI.<version>.pak.zip` | NextUI | Pak files only (no folder wrapper) | Manual install where you place the files in the right platform folder yourself |
| `Itch-io.NextUI.<version>.pakz` | NextUI | Full `Tools/<platform>/Itch-io.pak/` tree | Manual install — extract to SD card root and all platforms are set up at once |
| `Itch-io.muOS.<version>.muxapp` | muOS | The application directory | Install through muOS's Archive Manager |

---

## NextUI — via the Pak Store (recommended)

Open the Pak Store on your device, find **Itch-io**, and press **A** to install.
The Pak Store downloads and installs `Itch-io.pak.zip` automatically.

## NextUI — manual install, one device

Use this if you want to install without the Pak Store and prefer to place files
yourself.

1. Download `Itch-io.NextUI.<version>.pak.zip` from the [Releases](../../../releases) page.
2. Create the destination folder on your SD card for your device:
   - TrimUI Brick / Smart Pro: `Tools/tg5040/Itch-io.pak/`
   - TrimUI Smart Pro S: `Tools/tg5050/Itch-io.pak/`
   - Miyoo Flip: `Tools/my355/Itch-io.pak/`
3. Extract the contents of the zip **into** that folder (the folder should contain
   `launch.sh`, `itchio`, `pak.json`, etc. directly — not a nested subfolder).
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

## NextUI — manual install, every device at once

Use this if you want to set up all supported platforms in one step, or if you
are preparing an SD card that will be used across multiple device types.

1. Download `Itch-io.NextUI.<version>.pakz` from the [Releases](../../../releases) page.
2. Rename it to end in `.zip` (most tools require a `.zip` extension to extract).
3. Extract the contents directly to the **root** of your SD card. The archive
   already contains the correct `Tools/<platform>/Itch-io.pak/` structure.
4. Reinsert the SD card and boot into NextUI — **Itch-io** will appear in Tools.
5. Connect to WiFi before launching.

## muOS

1. Download `Itch-io.muOS.<version>.muxapp` from the [Releases](../../../releases) page.
2. Copy it into the `ARCHIVE` folder on your SD card — do not rename it or
   unpack it; muOS installs it as-is.
3. On the device: **Applications → Archive Manager**, select the file, press **A**.
4. **Itch.io** now appears under Applications.
5. Connect to Wi-Fi before launching.

Updating works the same way: install the new `.muxapp` over the old one. Your
settings, inventory and caches live in the application's own `data/` folder and
survive both an update and a muOS system update.

---

## NextUI and muOS differences

Itch-io does the same job on both. The two firmwares organise storage
differently, though, and a few features only exist on one of them.

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

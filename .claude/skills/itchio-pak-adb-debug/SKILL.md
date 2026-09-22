---
name: itchio-pak-adb-debug
description: Use when debugging Itch-io on a live handheld over USB — connecting over ADB on NextUI or muOS, picking the right device when several are attached, streaming logs, launching by hand to see startup output, profiling, and capturing framebuffer screenshots.
---

# Itch-io — ADB Live Device Debugging

The binary is `itchio` and the log is `itchio.log`. Enable ADB on the device
first: NextUI **Settings → Developer → ADB over USB**; muOS exposes it through
its own settings. Then connect a **data-capable** USB cable.

## ⚠️ Never pick a device by position

More than one handheld is usually attached — a NextUI one and a muOS one. **Do
not use `adb devices | awk 'NR==2'`**, and do not trust the USB descriptors:
a TrimUI Smart Pro reports itself as `product:occam model:Nexus_4 device:mako`,
and a TrimUI Brick reports a garbled `model:__B`.

`scripts/adb.sh` probes the device instead, and every project script sources it:

```sh
. scripts/adb.sh
adb_firmware <serial>     # -> nextui | muos | unknown
adb_select [firmware]     # -> the serial to use, or a readable error
adb_use [firmware]        # pins ANDROID_SERIAL so later bare `adb` calls target it
```

Selection order: `$ADB_SERIAL`, then `$ANDROID_SERIAL`, then the only attached
device, then the only attached device running the requested firmware. Anything
ambiguous is an error listing what was found — never a guess.

Firmware is probed on the device itself:

| Probe | Firmware |
|---|---|
| `[ -d /opt/muos ]` | muos |
| `[ -d /mnt/SDCARD/.system ]` | nextui |

Override any of it with `ADB_SERIAL=<serial>`.

## Deploying

`deploy.sh` pushes from `dist/`, which only `release.sh` populates — see the
`itchio-pak-build` skill for why `build.sh` then `deploy.sh` ships a stale
binary.

```sh
DEPLOY_PLATFORM=tg5040 ./scripts/deploy.sh   # NextUI pak via ADB (default platform: tg5040)
./scripts/deploy.sh /run/media/user/SD       # NextUI pak to a mounted SD card
./scripts/deploy.sh assets                   # only assets/, when fonts or the CA bundle changed
./scripts/deploy.sh muos                     # muOS app dir straight into the application store
./scripts/deploy.sh muxapp                   # the .muxapp into ARCHIVE/, to install the way a user would
```

`DEPLOY_PLATFORM` takes `tg5040|tg5050|my355|h700`. Each mode picks its own
firmware, so with one NextUI and one muOS device attached none of them need a
serial.

**`adb push` is an overlay, not a sync.** It never deletes. Two consequences:
user data under muOS's `data/` survives a redeploy (good), and stale files the
release no longer ships stay behind for ever (see `.profile-flags` below).

`deploy.sh muos` skips Archive Manager, so it does **not** prove the archive is
well formed. Use `muxapp` mode for that.

## Key paths on device

### NextUI

| Path | Contents |
|---|---|
| `/mnt/SDCARD/Tools/<platform>/Itch-io.pak/` | Pak files — `itchio`, `launch.sh`, `pak.json`, `assets/`, `lib/` |
| `/mnt/SDCARD/.userdata/<platform>/logs/itchio.log` | Runtime log |
| `/mnt/SDCARD/.userdata/shared/Itch-io/` | Config and inventory |
| `/mnt/SDCARD/.system/<platform>/lib/` | The firmware's own SDL2, which `launch.sh` prefers |
| `/tmp/itchio/cache/` | Image cache (volatile) |
| `/mnt/SDCARD/Roms/<System>/` | Downloaded ROMs |

`<platform>` is `tg5040`, `tg5050`, `my355` or `h700`.

### muOS

| Path | Contents |
|---|---|
| `<rom mount>/MUOS/application/Itch-io/` | The application — `itchio`, `mux_launch.sh`, `glyph/`, `assets/`, `version.txt` |
| `<rom mount>/MUOS/application/Itch-io/data/` | Config, inventory **and the log** — deliberately beside the app, because `SETUP_APP` points `$HOME` at `/root`, which a muOS update replaces |
| `<rom mount>/ARCHIVE/` | Where a `.muxapp` goes to be installed |

Get the mount rather than assuming `/mnt/mmc`:
```sh
adb shell 'cat /opt/muos/device/config/storage/rom/mount'
```

## `scripts/debug.sh` — NextUI only

Every command pins the **NextUI** device (`adb_use nextui`). There is no muOS
equivalent; on muOS, read the log out of the app's `data/` directory by hand.

```sh
./scripts/debug.sh logs       # stream the log, starting at the most recent "starting" line
./scripts/debug.sh push       # build for DEPLOY_PLATFORM, push binary + launch.sh
./scripts/debug.sh run        # build, push, then run launch.sh over ADB showing all output
./scripts/debug.sh pull-log   # pull the log here
./scripts/debug.sh pull-cache # pull /tmp/itchio/cache/ to ./debug-cache/
./scripts/debug.sh shell      # interactive shell
```

**`push` and `run` rebuild and overwrite the binary.** When the point is to test
the artifact a release actually produced, do not use them — deploy from `dist/`
and launch by hand.

### Profiling

```sh
./scripts/debug.sh profile        # cpu + mem; then launch via the device menu
./scripts/debug.sh profile-cpu
./scripts/debug.sh profile-mem
./scripts/debug.sh profile-live   # pprof on :6060, forwarded over ADB
./scripts/debug.sh pull-profile   # fetch to ./debug-profiles/
./scripts/debug.sh profile-restore
go tool pprof bin/nextui/tg5040/itchio ./debug-profiles/itchio-cpu.prof
```

These write `.profile-flags` into the pak directory; `launch.sh` reads it and
word-splits it into the exec. File-based on purpose — you launch normally from
the device menu, with no ADB attached and no framebuffer flicker.

## Launching by hand

The app reads its environment from the firmware. A bare `adb shell ./itchio`
gets none of it and will misbehave in ways that are not bugs.

### NextUI

```sh
adb shell '
export SDCARD_PATH=/mnt/SDCARD
export PLATFORM=tg5040
export DEVICE=brick
export SYSTEM_PATH=$SDCARD_PATH/.system/$PLATFORM
export SHARED_USERDATA_PATH=$SDCARD_PATH/.userdata/shared
cd /mnt/SDCARD/Tools/tg5040/Itch-io.pak && ./launch.sh 2>&1
'
```
`$DEVICE` is the SKU (`brick`, `brickpro`, `smartpro`, or an H700 model) and is
what names the handheld in the log. Values come from NextUI's own
`.system/<platform>/paks/MinUI.pak/launch.sh`.

### muOS — use `mux_launch.sh`, and know what skipping it costs

`SETUP_APP` (from `/opt/muos/script/var/func.sh`) restores the CPU governor,
registers the foreground process so muOS can suspend and resume the app, and —
via `SETUP_SDL_ENVIRONMENT` — exports **`SDL_GAMECONTROLLERCONFIG`**, grepped
out of `gamecontrollerdb.txt` for the device's `sdl/name`.

Running `./itchio` directly therefore logs:

```
input: joystick 0 "..." — no controller mapping and none available, its buttons will not reach the UI
```

**That warning is an artifact of skipping the launcher, not a defect.** Check
before reporting it:

```sh
adb shell '. /opt/muos/script/var/func.sh; GET_VAR "device" "sdl/name"'
adb shell 'grep -h "<that name>" /usr/lib/gamecontrollerdb.txt'
```

A direct run is still fine for checking firmware detection, storage resolution,
SDL loading and the feed — just not input. Running the real `mux_launch.sh` over
ADB does test input, at the cost of leaving muOS's foreground-process bookkeeping
pointing at a process the frontend did not start.

## Startup lines worth reading

The first ~20 lines of the log answer most questions before any theorising:

| Line | Tells you |
|---|---|
| `itchio <version> starting` / `git commit:` | Whether the build you think you deployed is the one running |
| `firmware:` / `device:` / `device sku:` | Detection worked, and which of the eleven H700 models it is |
| `storage: root= data=` | Where it will write |
| `ld_libs:` | **Which SDL2 won.** Should start with the firmware's own directory |
| `sdl: runtime <v>` | The version actually loaded. On H700, `2.0.12` means it found stock Anbernic's instead of NextUI's — a bug even if things look fine |
| `input: controller N` / `input: joystick N` | `joystick` means no game-controller mapping, so button events reach nothing |
| `input: face buttons` | `swapped` (TrimUI) vs `direct`, and the A/B arrangement |
| `display:` | The geometry the layout code is working to |
| `theme: available=` | `false` on muOS is correct — NextUI theming is switched off there |

## Gotchas

- **`adb shell` exit codes lie.** It returns 0 regardless of what the remote
  command did. Gate on output, not `$?`.
- **Strip CRLF.** Device output carries `\r`; pipe through `tr -d '\r'` before
  comparing or capturing into a variable.
- **`pkill` does not exist** on these devices. Kill by PID:
  ```sh
  adb shell 'ps | grep "[i]tchio" | awk "{print \$1}" | while read p; do kill -9 "$p"; done'
  ```
- **`.profile-flags` survives every deploy.** `adb push` never deletes, so a
  device left profiling in one session keeps profiling for months. If the log
  says `profiling: cpu=...` when you did not ask for it, run
  `./scripts/debug.sh profile-restore`. `profiling: off` is the normal state.
- **`screenshot.sh` is NextUI-only and reads `/dev/fb0`.** It has no muOS or
  h700 support. **On tg5050 `/dev/fb0` is all zeros** — that display is DRM/KMS,
  so framebuffer captures come out blank; use `SDL_RenderReadPixels` instead.
  See the memory entry "tg5050 /dev/fb0 is dead".
- **A cold first run gets rate-limited.** Fetching every tag feed draws HTTP 429
  from itch.io; the app backs off and retries (`retry 1/3 after 2s`) and
  completes. Expected, not a regression.
- **Offscreen renders are not pixel-identical to the device for text** — the
  device's SDL2_ttf rasterises differently. Capture from hardware when the pixels
  are the point. See the `itchio-pak-device-screenshot` skill.

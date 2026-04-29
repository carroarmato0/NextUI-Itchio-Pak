---
name: itchio-pak-adb-debug
description: Use when debugging the Itch.io NextUI Pak on a live TrimUI or Miyoo device via USB — connecting over ADB, streaming logs, inspecting crash output, checking SDL2 errors, pushing fixes, or investigating filesystem state on the device.
---

# Itch.io Pak — ADB Live Device Debugging

ADB must be enabled in NextUI settings before the first connection (Settings → Developer → ADB over USB).
Once enabled, connect via a data-capable USB cable; `adb devices` lists the device immediately.
This enables live debugging without SD card swaps.

## Quick Reference

```sh
# Verify device is connected and recognised
adb devices

# Interactive shell on the device
adb shell

# Stream Pak log in real time (Ctrl-C to stop); replace tg5040 with tg5050 or my355 as needed
adb shell "tail -f /mnt/SDCARD/.userdata/tg5040/logs/itchio-pak.log"

# Pull log to host for inspection
adb pull /mnt/SDCARD/.userdata/tg5040/logs/itchio-pak.log .

# Check if Pak process is running and see its stderr/stdout
adb shell "ps | grep itchio"

# Push a newly compiled binary without SD card ejection
adb push bin/tg5040/itchio-pak /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio-pak

# Push entire pak directory (incremental)
adb push build/tg5040/Itch-io.pak/. /mnt/SDCARD/Tools/tg5040/Itch-io.pak/

# Pull image cache to inspect what was downloaded/cached
adb pull /tmp/itchio-pak/cache/ ./debug-cache/

# Inspect config on device
adb shell "cat /mnt/SDCARD/.userdata/shared/Itch-io/config.json"

# Check available disk space on SD card
adb shell "df -h /mnt/SDCARD"

# List downloaded ROMs
adb shell "ls -la '/mnt/SDCARD/Roms/Game Boy Color (GBC)/'"
```

## Platform Detection via ADB

The pak detects the platform at runtime using:
- presence of `/usr/miyoo` → `my355`
- `grep TG5050 /proc/cpuinfo` matches → `tg5050`
- otherwise → `tg5040`

You can reproduce this from ADB:
```sh
# Miyoo?
adb shell "[ -d /usr/miyoo ] && echo my355"

# TrimUI Smart Pro S?
adb shell "grep -q TG5050 /proc/cpuinfo && echo tg5050 || echo tg5040"
```

## Capturing SDL2 / Binary Crash Output

The binary's stderr is not visible from the NextUI menu. To capture it:

```sh
# Launch the pak binary directly from ADB shell and see all output live
adb shell "cd /mnt/SDCARD/Tools/tg5040/Itch-io.pak && ./itchio-pak 2>&1 | tee /tmp/pak-run.log"

# Pull the combined stdout+stderr log after a crash
adb pull /tmp/pak-run.log .
```

## `scripts/debug.sh` — One-Command Debug Session

The project includes `scripts/debug.sh` which wraps common workflows:

```sh
./scripts/debug.sh logs       # Stream itchio-pak.log live
./scripts/debug.sh push       # Build (if needed) and push binary to connected device
./scripts/debug.sh run        # Push binary then launch it directly over ADB shell (shows all output)
./scripts/debug.sh pull-cache # Pull /tmp/itchio-pak/cache/ to ./debug-cache/
./scripts/debug.sh pull-log   # Pull itchio-pak.log to current directory
./scripts/debug.sh shell      # Open interactive ADB shell
```

## Enabling ADB on the Device

ADB must be enabled in NextUI settings before the first connection:
- Navigate to NextUI **Settings → Developer**
- Enable **ADB over USB**
- Connect a data-capable USB cable (not charge-only)
- On host: `adb devices` should show the device

## Key Paths on Device

| Path | Contents |
|------|----------|
| `/mnt/SDCARD/Tools/tg5040/Itch-io.pak/` | Pak files (binary, assets, lib) |
| `/mnt/SDCARD/.userdata/shared/Itch-io/config.json` | User config (API key, ROM mode) |
| `/mnt/SDCARD/.userdata/{platform}/logs/itchio-pak.log` | Runtime log (platform = tg5040/tg5050/my355) |
| `/tmp/itchio-pak/cache/` | Image cache (volatile, cleared on exit) |
| `/mnt/SDCARD/Roms/Game Boy Color (GBC)/` | Downloaded GBC ROMs |
| `/mnt/SDCARD/Roms/Game Boy (GB)/` | Downloaded GB ROMs |

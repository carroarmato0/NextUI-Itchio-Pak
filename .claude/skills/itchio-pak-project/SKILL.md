---
name: itchio-pak-project
description: Use when starting any work session on the Itch.io NextUI Pak project, or when unsure about architecture decisions, platform codes, directory layout, key library choices, or ROM destination paths for this specific project.
---

# Itch.io Pak — Project Reference

## Stack
- **Language:** Go 1.22+
- **UI renderer:** SDL2 via `github.com/veandco/go-sdl2` (CGo)
- **QR codes:** `github.com/skip2/go-qrcode` (pure Go)
- **Cross-compile:** Docker or Podman with LoveRetro toolchain images
- **Config format:** JSON at `$SHARED_USERDATA_PATH/Itchio/config.json`

## Supported Platforms (all ARM64)

| Code | Device | Resolution |
|------|--------|-----------|
| `tg5040` | TrimUI Brick (1024×768) + Smart Pro (1280×720) | ARM64 |
| `tg5050` | TrimUI Smart Pro S (1280×720) | ARM64 |
| `my355` | Miyoo Flip (640×480) | ARM64 |

Single binary covers all three. SDL2 `.so` files may differ per platform (extracted from toolchain images).

## Key Directories

```
cmd/itchio-pak/       Entry point
internal/itchio/      HTTP client, RSS feed, page scraping, download flow
internal/ui/          Screen definitions (list, detail, settings, download, pickers, filters)
internal/renderer/    SDL2 drawing layer + LRU image cache
internal/roms/        ROM type detection, destination folder logic
internal/settings/    JSON config read/write
internal/inventory/   Owned/downloaded game tracking, update detection
internal/logger/      Levelled file logger (writes to $HOME/itchio-pak.log)
internal/power/       Power management — wakeup from sleep, clean shutdown
lib/{tg5040,tg5050,my355}/  Bundled SDL2 .so files
assets/               font.ttf, icon.png (Itch.io logo — unofficial use)
testdata/             Captured HTML/RSS fixtures for offline unit tests
scripts/              test.sh, build.sh, release.sh, deploy.sh, screenshot.sh
docker/               Dockerfile.dev (host/CI builds), Dockerfile.platform (cross-compile)
```

## UI Layout
- **Game list screen:** SDL2 split panel — left 55% scrollable text list, right 45% live cover art (updates as user scrolls, LRU-cached from /tmp)
- **Game detail screen:** Cover art + screenshot thumbnails (L/R to browse) + metadata + QR code (every game) + Download / Back actions
- **Settings:** API key entry, ROM selection mode (auto / ask), content moderation filters, tag filter, manage downloads, clear cache, about

## ROM Placement
```
.gb  → /mnt/SDCARD/Roms/Game Boy (GB)/
.gbc → /mnt/SDCARD/Roms/Game Boy Color (GBC)/
```
Auto mode: prefer `.gbc > .gb`, skip `.pocket` and non-ROM files.
Ask mode: show file picker before downloading.

## Button Mapping (NextUI conventions)
- D-pad: navigate · A: confirm · B: back
- L/R: page game list forward/back; or prev/next screenshot
- Start: open settings from anywhere

## Key Design Decisions
- Paid games shown in list with price badge; QR + "Add API Key" shortcut on detail screen
- Free download scraping falls back to QR display if Itch.io page structure changes (never silent failure)
- Image cache: LRU 50 entries, resized JPEGs in `/tmp/itchio-pak/cache/`, cleared on exit
- All platforms use same binary; only SDL2 libs may differ
- `internal/inventory/` tracks which games have been downloaded and detects when store listings are updated or removed
- Power events (sleep/wake, shutdown) handled via `internal/power/`; SDL event queue flushed on wake to suppress buffered inputs
- README must state: *"Unofficial community Pak, not affiliated with or endorsed by Itch.io / Leafo"*

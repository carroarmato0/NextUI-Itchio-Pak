# Itch.io NextUI Pak — Design Specification

**Date:** 2026-04-17  
**Status:** Approved  
**Project:** `/home/carroarmato0/Applications/Development/NextUI/Paks/Itch.io`

---

## 1. Overview

A NextUI Pak for the TrimUI Brick Hammer (and other supported devices) that lets users browse, discover, and download Game Boy / Game Boy Color ROM files from the Itch.io "made-with-gb-studio" category — entirely on-device over WiFi, without a companion PC.

The Pak presents a game-launcher-style interface with a split-panel game list (scrollable titles on the left, live cover art on the right), a rich detail screen per game, and a fully automated ROM download flow. Paid games are shown transparently with price badges and QR codes; users who configure an Itch.io API key gain direct download access to their purchased games.

**Disclaimer (required in all user-facing surfaces):** This is an unofficial community Pak, not affiliated with or endorsed by Itch.io / Leafo.

---

## 2. Supported Devices & Platforms

All three supported platforms are ARM64 Linux. A single compiled binary covers all of them; only bundled SDL2 shared libraries may differ per platform.

| Platform code | Device | Resolution |
|---|---|---|
| `tg5040` | TrimUI Brick (1024×768) + TrimUI Smart Pro (1280×720) | ARM64 |
| `tg5050` | TrimUI Smart Pro S (1280×720) | ARM64 |
| `my355` | Miyoo Flip (640×480) | ARM64 |

---

## 3. Technology Stack

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.22+ | Static binary, strong stdlib for HTTP/JSON/regex, easy ARM64 cross-compile |
| UI renderer | SDL2 via `github.com/veandco/go-sdl2` | Required for split-panel layout with live cover art; proven by ScrapeGoat pak |
| QR generation | `github.com/skip2/go-qrcode` | Pure Go, no CGo, generates PNG in-memory |
| Image cache | stdlib `image/jpeg` + `golang.org/x/image` | Pure Go resize/decode, no extra deps |
| Cross-compile | LoveRetro Docker/Podman toolchain images | SDL2 + ARM64 toolchain pre-configured per platform |
| Config storage | JSON file | Simple, human-readable, survives Pak updates |

---

## 4. Project Structure

```
Itch.io.pak/
├── launch.sh                        # Entry: set env, LD_LIBRARY_PATH, exec binary
├── pak.json                         # Pak Store metadata
├── README.md                        # User guide: setup, API key, purchasing guide
├── CONTRIBUTING.md                  # Developer guide: prerequisites, build, test, deploy
├── Makefile                         # Targets: test, build-native, build-all, release, deploy, clean
│
├── cmd/itchio-pak/
│   └── main.go                      # Wire deps, parse env/flags (--headless for CI), start UI loop
│
├── internal/
│   ├── itchio/
│   │   ├── client.go                # HTTP client: cookie jar, redirect handling, timeouts
│   │   ├── feed.go                  # RSS feed fetch + pagination (page=N, 40/page)
│   │   ├── game.go                  # Game page scraping: metadata, cover, screenshots, uploads
│   │   ├── download.go              # Free download flow (CSRF POST → signed URL → stream)
│   │   └── download_auth.go         # Authenticated download flow (API key path)
│   ├── ui/
│   │   ├── screen_list.go           # Split-panel game list screen
│   │   ├── screen_detail.go         # Game detail: cover, screenshots slideshow, QR, actions
│   │   ├── screen_settings.go       # Settings screens (API key, ROM mode, cache, about)
│   │   └── screen_download.go       # Download progress screen
│   ├── renderer/
│   │   ├── renderer.go              # SDL2 init, window creation, main event/draw loop
│   │   ├── image_cache.go           # LRU image cache: fetch URL → resize → /tmp → SDL2 texture
│   │   └── qr.go                    # QR PNG generation + SDL2 texture creation
│   ├── roms/
│   │   └── roms.go                  # Upload scoring, ROM type → destination folder mapping
│   └── settings/
│       └── settings.go              # JSON config read/write at $HOME/config.json
│
├── bin/
│   ├── tg5040/itchio-pak            # ARM64 binary (output of cross-compile for tg5040)
│   ├── tg5050/itchio-pak            # ARM64 binary (output of cross-compile for tg5050)
│   └── my355/itchio-pak             # ARM64 binary (output of cross-compile for my355)
│                                    # Note: all three binaries are identical ARM64 builds;
│                                    # separate dirs exist so release.sh can assemble each pak
│
├── lib/
│   ├── tg5040/                      # SDL2 .so files for tg5040/tg5050 (Allwinner)
│   └── my355/                       # SDL2 .so files for my355 (Rockchip)
│
├── assets/
│   ├── font.ttf                     # Font for SDL2 text rendering
│   └── icon.png                     # Itch.io logo (nominative fair use, unofficial)
│
├── testdata/
│   ├── rss_page1.xml                # Captured RSS feed page 1
│   ├── game_page_free.html          # Free game page (for game_id/csrf extraction tests)
│   ├── game_page_paid.html          # Paid game page (for paid gate detection tests)
│   └── download_page.html           # Download page (for upload link parsing tests)
│
├── scripts/
│   ├── test.sh                      # Run tests; --coverage outputs coverage.html
│   ├── build.sh                     # native | tg5040 | tg5050 | my355 | all; --runtime flag
│   ├── release.sh                   # Build all → dist/*.pak.zip + dist/all/*.pakz
│   ├── deploy.sh                    # ADB auto-detect or SD card path argument
│   └── debug.sh                     # Live device debugging: logs, push, run, pull-cache, shell
│
└── docker/
    ├── Dockerfile.dev               # Dev/test image: Go + SDL2 dev libs (x86_64)
    └── Dockerfile.build             # Cross-compilation environment reference (informational)
```

---

## 5. launch.sh

```sh
#!/bin/sh
PAK_DIR="$(dirname "$0")"
PAK_NAME="$(basename "$PAK_DIR")"
PAK_NAME="${PAK_NAME%.*}"
export HOME="$SHARED_USERDATA_PATH/$PAK_NAME"
export LD_LIBRARY_PATH="$PAK_DIR/lib:$LD_LIBRARY_PATH"
export PATH="$PAK_DIR:$PATH"
mkdir -p "$HOME"
exec "$PAK_DIR/itchio-pak"
```

In each packaged `.pak.zip`, `lib/` sits at the pak root and contains the SDL2 `.so` files for that specific platform. During development, the source tree has `lib/tg5040/` and `lib/my355/`; `release.sh` copies the right subdirectory into each platform's pak before zipping.

---

## 6. User Interface

### 6.1 Navigation & Button Mapping

| Button | Action |
|---|---|
| D-pad ↕ | Navigate list / settings items |
| A | Confirm / select |
| B | Back |
| L / R | Page game list forward/back; or prev/next screenshot in detail view |
| Start | Open settings from any screen |

### 6.2 Screen: Game List (entry point)

SDL2 split panel:
- **Left 55%:** Scrollable game list. Each row: game title + price badge (`free` in green, `$N.NN` in amber). Current selection highlighted.
- **Right 45%:** Cover art for the currently highlighted game, updated live as the user scrolls. Pre-fetches cover images for the 5 items above and below the current selection. Falls back to a placeholder if cover not yet loaded.
- **Footer:** Page indicator (`Page 1 / 33 · 40 shown`).
- **L/R buttons** page forward/back through the 40-game pages.

### 6.3 Screen: Search

Pressing **Y** from the game list screen opens a text input prompt using the SDL2 on-screen keyboard. The search query is appended to the RSS URL (`?q={query}`) to filter results server-side. Results replace the current game list; pressing B returns to the unfiltered list.

### 6.4 Screen: Game Detail

Opened by pressing A on any game in the list.

- Cover art (full width, top)
- Screenshot thumbnails below cover (L/R to browse as slideshow)
- Metadata: title, author, genre, price
- Short description (truncated with scroll if needed)
- QR code (always present — links to the game's Itch.io store page regardless of price)
- Actions:
  - **Free game:** `⬇ Download` button
  - **Paid game (no API key):** `🔒 Requires purchase` + QR + `+ Add API Key` shortcut
  - **Paid game (API key set, game owned):** `⬇ Download` button
  - **Back** always available

### 6.5 Screen: ROM File Picker (conditional)

Only shown when `rom_selection = "ask"` AND the game has multiple `.gb`/`.gbc` uploads.  
Displays a list of available ROM files; user selects one before download proceeds.

### 6.6 Screen: Download Progress

Shows filename, progress bar (streamed byte count / total), and destination path. On completion shows success message and destination path. On failure shows error and QR fallback.

### 6.7 Screens: Settings

Accessible from any screen via Start. Contains:

1. **Itch.io API Key** — text entry (masked); stored in config; enables paid game downloads
2. **ROM Selection** — toggle: `Auto (best ROM)` / `Always ask`
3. **Clear Image Cache** — deletes `/tmp/itchio-pak/cache/`
4. **About** — version, unofficial disclaimer, link QR to GitHub repo

---

## 7. Data Flow

```
RSS Feed (page N)
  └─ itchio.FetchGames(page, query) → []Game
        │
        ├─ renderer pre-fetches CoverURL for ±5 visible items → LRU image cache
        │
        └─ user selects game
              └─ itchio.FetchGameDetail(url) → GameDetail
                    │
                    ├─ renderer: detail screen (cover, screenshots, QR)
                    │
                    └─ user presses Download
                          └─ roms.SelectUpload(uploads, settings.ROMMode)
                                │  auto: score .gbc(2) > .gb(1), skip others
                                │  ask:  show file picker
                                └─ itchio.Download(upload, apiKey)
                                      │  free:  CSRF POST → signed URL → stream
                                      │  auth:  API key → download key → stream
                                      └─ roms.DestinationPath(ext) → write file
```

---

## 8. Itch.io Integration

### 8.1 RSS Feed

```
GET https://itch.io/games/made-with-gb-studio.xml?page=N
```

40 entries per page (~33 pages for ~1,299 games). Each entry provides: title, game URL, cover image URL (embedded in description HTML), price, author (from subdomain).

### 8.2 Free Game Download Flow

```
1. GET https://{user}.itch.io/{game}
   Extract: data-game_id, csrf_token

2. POST https://{user}.itch.io/{game}/download_url
   Body: csrf_token={token}&suggested_amount=0
   → JSON: { "url": "..." }

3. GET {url}  → HTML download page

4. Parse upload links → filter .gb / .gbc

5. GET {upload_link}  → stream to destination (follows Cloudflare R2 redirect)
```

### 8.3 Authenticated Download Flow

When API key is configured and game is owned:

```
1. GET https://{user}.itch.io/{game}  → extract game_id (same as free flow step 1)
2. GET https://itch.io/api/1/{key}/game/{game_id}/download_keys  → verify ownership
3. GET {upload_link} with Authorization: Bearer {api_key}  → stream to destination
```

`upload_link` comes from parsing the game page uploads list (same `game.go` scrape used in the free flow). The API key adds the auth header; the ownership check in step 2 gates whether the download button is shown at all.

### 8.4 Failure Fallback

Any HTTP error, unexpected page structure, or paid gate encountered without a valid key:
- Show QR code for game store URL
- Show human-readable error message
- Log to `$HOME/itchio-pak.log`
- Never fail silently

---

## 9. ROM Management

### Destination Folders

| Extension | Destination |
|---|---|
| `.gb` | `/mnt/SDCARD/Roms/Game Boy (GB)/` |
| `.gbc` | `/mnt/SDCARD/Roms/Game Boy Color (GBC)/` |

### Upload Scoring (auto mode)

```go
func scoreUpload(filename string) int {
    switch filepath.Ext(strings.ToLower(filename)) {
    case ".gbc": return 2
    case ".gb":  return 1
    default:     return 0  // skip
    }
}
```

Picks highest-scoring upload. If score is 0 (no ROM files found), falls back to QR + error message.

---

## 10. Image Cache

- Location: `/tmp/itchio-pak/cache/`
- Format: JPEG, resized to max 640px wide (preserving aspect ratio)
- Eviction: LRU, max 50 entries
- Pre-fetch: 5 items above + below current scroll position
- Cleared on Pak exit and by "Clear Image Cache" in Settings

---

## 11. Configuration

Stored at `$SHARED_USERDATA_PATH/Itchio/config.json`:

```json
{
  "api_key": "",
  "rom_selection": "auto"
}
```

| Key | Values | Default |
|---|---|---|
| `api_key` | Itch.io API key string, or `""` | `""` |
| `rom_selection` | `"auto"` \| `"ask"` | `"auto"` |

---

## 12. Build System

### Container Runtime Detection (`scripts/build.sh`)

```sh
detect_runtime() {
    case "${CONTAINER_RUNTIME:-}" in
        docker|podman) echo "$CONTAINER_RUNTIME"; return ;;
    esac
    if command -v podman >/dev/null 2>&1; then echo "podman"
    elif command -v docker >/dev/null 2>&1; then echo "docker"
    else echo ""; fi
}
```

Priority: explicit `$CONTAINER_RUNTIME` env var → `--runtime` CLI flag → podman → docker.

### LoveRetro Toolchain Images

| Platform | Image |
|---|---|
| tg5040 | `ghcr.io/loveretro/tg5040-toolchain:latest` |
| tg5050 | `ghcr.io/loveretro/tg5050-toolchain:latest` |
| my355 | `ghcr.io/loveretro/my355-toolchain:latest` |

### Scripts

| Script | Purpose |
|---|---|
| `scripts/test.sh [--coverage]` | `go test -race ./...`; optional HTML coverage report |
| `scripts/build.sh native` | Build for host machine (requires local SDL2 dev libs) |
| `scripts/build.sh tg5040\|tg5050\|my355` | Cross-compile via container toolchain |
| `scripts/build.sh all` | Cross-compile all three platforms |
| `scripts/release.sh` | Tests → build all → create `dist/` artifacts |
| `scripts/deploy.sh [path]` | ADB auto-detect or SD card path |

### Developer Prerequisites

- **Docker or Podman** — required for all build and test work; Go, SDL2 dev libs, and cross-compilation toolchains are all provided by containers
- **ADB** (`android-tools` / `android-platform-tools`) — required only for `deploy.sh` and `debug.sh` over USB; not needed if deploying via SD card

`deploy.sh` and `debug.sh` run directly on the host (they need USB/ADB access that cannot be cleanly passed into a container). All other scripts run inside containers.

### Container Boundary

| Script | Runs in container | Reason |
|---|---|---|
| `test.sh` | Yes — dev image | Needs Go; no device access |
| `build.sh native` | Yes — dev image | Needs Go + SDL2 dev libs |
| `build.sh tg5040/tg5050/my355` | Yes — LoveRetro toolchain image | Needs ARM64 cross-toolchain |
| `release.sh` | Yes — orchestrates above | No device access |
| `deploy.sh` | **No — host** | Needs USB/ADB |
| `debug.sh` | **No — host** | Needs USB/ADB |

### Dev Container (`docker/Dockerfile.dev`)

Used by `test.sh` and `build.sh native`. Built once and cached locally under the image tag `itchio-pak-dev`.

```dockerfile
FROM golang:1.22-bookworm
RUN apt-get update && apt-get install -y \
    libsdl2-dev libsdl2-ttf-dev libsdl2-image-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
```

### Container Self-Re-Invocation Pattern

`test.sh`, `build.sh`, and `release.sh` transparently re-invoke themselves inside the correct container when not already running in one:

```sh
# At the top of each containerised script:
IMAGE="itchio-pak-dev"
if [ -z "${IN_CONTAINER:-}" ]; then
    RUNTIME="$(detect_runtime)"
    $RUNTIME image inspect "$IMAGE" >/dev/null 2>&1 || \
        $RUNTIME build -t "$IMAGE" -f docker/Dockerfile.dev .
    exec $RUNTIME run --rm \
        -v "$(pwd):/workspace" \
        -w /workspace \
        -e IN_CONTAINER=1 \
        "$IMAGE" "$0" "$@"
fi
# rest of script runs here, already inside the container
```

`build.sh tg5040` uses the LoveRetro toolchain image instead of the dev image — same pattern, different `IMAGE` value. `deploy.sh` and `debug.sh` have no such guard and run straight through on the host.

### Release Artifacts

```
dist/
  tg5040/Itch.io.pak.zip     # Single-device zip (Pak Store)
  tg5050/Itch.io.pak.zip
  my355/Itch.io.pak.zip
  all/Itch.io.pakz           # Multi-device bundle (extract to SD card root)
```

The `.pakz` internal structure:
```
Tools/
  tg5040/Itch.io.pak/
  tg5050/Itch.io.pak/
  my355/Itch.io.pak/
```

---

## 13. Testing Strategy

### Unit Tests (no network, no device, no container)

| Package | What is tested |
|---|---|
| `internal/itchio` | RSS parsing, game_id/csrf extraction, upload link parsing, paid gate detection — all using `testdata/` fixtures via `httptest.NewServer` |
| `internal/roms` | Upload scoring, extension → folder mapping |
| `internal/settings` | Config read/write round-trip |

### CI

GitHub Actions runs `scripts/test.sh` on every push. No Docker/device needed.

SDL2 renderer is excluded from automated tests via `//go:build !headless`. The `--headless` flag in `main.go` skips SDL2 initialisation, allowing the binary to be exercised in CI without a display.

### Manual Testing

The SDL2 renderer and full download flow are tested manually on device (or via `build.sh native` + local SDL2). `scripts/deploy.sh` accelerates the edit-deploy-test loop.

### Live Device Debugging via ADB

ADB (Android Debug Bridge) is available on TrimUI devices running NextUI out of the box — no enable step required. Connect via a data-capable USB cable and `adb devices` will list the device immediately.

`scripts/debug.sh` wraps the common workflows:

| Command | What it does |
|---|---|
| `./scripts/debug.sh push` | Cross-compile and push binary to connected device |
| `./scripts/debug.sh run` | Push binary then launch directly via ADB shell (captures all stdout/stderr live) |
| `./scripts/debug.sh logs` | Stream `itchio-pak.log` from device in real time |
| `./scripts/debug.sh pull-log` | Pull log file to host for inspection |
| `./scripts/debug.sh pull-cache` | Pull `/tmp/itchio-pak/cache/` to `./debug-cache/` |
| `./scripts/debug.sh shell` | Open interactive ADB shell on the device |

Running the binary directly via `debug.sh run` is particularly valuable — it captures SDL2 errors and Go panics that are otherwise invisible when launching through the NextUI menu. This is the primary tool for diagnosing crashes, rendering issues, and download flow failures on real hardware.

Both ADB (`./scripts/deploy.sh`) and SD card (`./scripts/deploy.sh /run/media/user/SD`) are fully supported deployment paths.

---

## 14. Pak Store Metadata (`pak.json`)

```json
{
  "name": "Itch.io",
  "version": "1.0.0",
  "type": "tool",
  "description": "Browse and download GB/GBC games from Itch.io's GB Studio collection.",
  "author": "Carroarmato0",
  "repo_url": "https://github.com/carroarmato0/NextUI-Itchio-Pak",
  "release_filename": "Itch.io.pak.zip",
  "platforms": ["tg5040", "tg5050", "my355"]
}
```

---

## 15. Documentation Requirements

### README.md (user-facing)

Must cover:
1. What this Pak does and the unofficial disclaimer
2. Installation (Pak Store + manual zip install)
3. How to browse and download free games
4. How to purchase a game on Itch.io (QR code workflow)
5. How to obtain an Itch.io API key (Settings → API Keys on itch.io)
6. How to configure the API key in the Pak (Settings → Itch.io API Key)
7. ROM placement locations

### CONTRIBUTING.md (developer-facing)

Must cover:
1. Prerequisites: Docker or Podman only (everything else is containerised)
2. Container runtime selection (podman/docker auto-detect, `$CONTAINER_RUNTIME` override, `--runtime` flag)
3. Clone and first build (`scripts/build.sh native` — dev container built automatically)
4. Running tests (`scripts/test.sh --coverage` — runs inside dev container)
5. Cross-compiling for a device (`scripts/build.sh tg5040` — uses LoveRetro toolchain image)
6. Deploying to a connected device (`scripts/deploy.sh`)
7. Live debugging via ADB (`scripts/debug.sh` — no device setup required; covers streaming logs, direct binary launch, capturing crash output)
8. Adding new testdata fixtures (how to capture live HTML responses for offline tests)
8. Creating a release (`scripts/release.sh`)
9. Project structure walkthrough (key packages and their responsibilities)

---

## 16. Out of Scope (v1.0)

- Game rating/review display (not available via RSS or unauthenticated API)
- Wishlisting or marking games as "want to play"
- Automatic ROM library scanning or duplicate detection
- Support for non-GB-Studio games or other ROM formats
- Offline browsing / full catalogue cache

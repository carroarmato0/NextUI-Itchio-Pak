# Itch.io Pak — Claude Reference

Unofficial NextUI Pak that lets users browse and download GB/GBC games from Itch.io directly on handheld devices. Written in Go, rendered with SDL2, cross-compiled for ARM64. Not affiliated with Itch.io / Leafo.

## Critical Constraints

- **Target arch:** ARM64 only — no x86 binaries ship to devices
- **No X11/Wayland/PulseAudio** on devices — SDL2 runs in raw framebuffer mode
- **Cross-compile required** for device targets — use the container scripts, never `go build` directly for tg5040/tg5050/my355
- **Single binary** covers all three platforms; only the bundled SDL2 `.so` files differ per platform
- **CGo is required** — SDL2 bindings use CGo; pure-Go-only builds use the `headless` build tag (CI only)

## Supported Platforms

| Code | Device | Resolution |
|------|--------|------------|
| `tg5040` | TrimUI Brick (1024×768) + Smart Pro (1280×720) | ARM64 |
| `tg5050` | TrimUI Smart Pro S (1280×720) | ARM64 |
| `my355` | Miyoo Flip (640×480) | ARM64 |

## Key Commands

```sh
./scripts/test.sh                  # Run tests (containerised)
./scripts/build.sh native          # Build for host (containerised)
./scripts/build.sh tg5040          # Cross-compile for TrimUI
./scripts/build.sh all             # Cross-compile all three platforms
./scripts/release.sh               # Build + package dist/ artifacts
./scripts/deploy.sh                # Push to connected device via ADB
./scripts/debug.sh logs            # Stream device log live
./scripts/dev-screenshot.sh --all --out-dir /tmp/itchio-screenshots
./scripts/palette-audit.sh              # render every scene x palette, fail on unreadable text
go run ./cmd/devshot --list             # list the offscreen scenes
```

All also available as `make` targets: `test`, `build-native`, `build-all`, `release`, `deploy`, `clean`.

## Offscreen screenshots — `cmd/devshot`

Renders real screens to PNG on the host: no device, no display, no network.
Scenes live in `internal/ui/dev_scenes.go` (inside package `ui`, because the
states worth capturing — an open modal, a scrolled page — are unexported).

```sh
go run ./cmd/devshot --scene detail --palette "Catppuccin Latte"
go run ./cmd/devshot --scene detail --full          # whole scrollable page, fold-marked
go run ./cmd/devshot --all --palettes all --sheet --audit
go run ./cmd/devshot --scene list --cover-url http://127.0.0.1:8000/c.png --settle 2s
```

`--audit` records every text draw with the background sampled underneath it and
reports low-contrast pairs, so it judges the frame actually produced rather than
theme accessors in isolation. `scripts/palette-audit.sh` wraps this over all 18
bundled palettes (`testdata/palettes/`) and runs as part of `test.sh`.

**Render at 1024x768.** Font size scales with height (`h/22`) but pill padding and
`LayoutFor` constants are fixed pixels, so any other size changes the layout and
the output stops matching the device.

**Offscreen is not pixel-identical to the device for text.** Geometry matches
exactly, but the device uses the bundled SDL2_ttf from `lib/tg5040` while the
host uses its own, and the two rasterise glyphs differently (median channel
delta ~100 on a footer strip). Use it for the colour audit and for layout work;
capture from the device when the pixels themselves are the point — gate each
capture on the pak logging `palette="<name>"`, since the theme is read once at
startup and the framebuffer survives a relaunch.


## Key Directories

```
cmd/itchio-pak/    Entry point (main.go, main_sdl.go, main_headless.go)
internal/itchio/   HTTP client, RSS feed, scraper, download flows
internal/ui/       Screen definitions (screen_*.go)
internal/renderer/ SDL2 drawing layer + LRU image cache
internal/roms/     ROM type detection, destination folder logic
internal/settings/ JSON config read/write
internal/inventory/Owned/downloaded game tracking, update detection
internal/logger/   Levelled file logger → $HOME/itchio-pak.log
internal/power/    Sleep/wake/shutdown handling
assets/            font.ttf + 4 fallback fonts, ca-certificates.crt
testdata/          HTML/RSS fixtures for offline unit tests
```

## Coding Standards

- All new and modified code **must** include structured log calls at key points — see memory entry "Logging standards" for the full checklist (goroutines, cache ops, HTTP calls, file I/O).
- No `fmt.Println` / `log.Printf` in production paths — use `internal/logger`.
- **No colour literals in drawing code** — every colour comes from `internal/theme`.
  `scripts/no-color-literals.sh` (run by `test.sh`) rejects numeric RGB triples and
  `uint8` channel arithmetic like `bg[0]+20`, which wraps on light palettes.
- SDL2 renderer code is excluded from CI via `//go:build !headless`; headless-safe logic goes in separate files.
- Tests use `httptest.NewServer` with fixtures from `testdata/` — no live network calls in tests.

## Known Gotchas

- **SDL_ttf `GlyphMetrics` always succeeds** (returns `.notdef`) — do not use it for font coverage detection; parse the cmap table directly. See memory entry "SDL_ttf GlyphMetrics unreliable".
- **Itch.io owned-keys last page:** the API returns `{}` (object) not `[]` (array) when exhausted — use `json.RawMessage` and check `raw[0] == '['` before unmarshaling. See `auth_validate.go` for the pattern.
- **Screenshot output:** always write to `/tmp/itchio-screenshots/`, never to `docs/screenshots/` — that directory is populated manually by the developer after design approval.

## Skills — When to Use Which

| Skill | When |
|-------|------|
| `itchio-pak-project` | Architecture decisions, platform codes, ROM destination paths |
| `itchio-pak-build` | Build/release/deploy questions, container runtime, artifact structure |
| `itchio-pak-adb-debug` | Live device debugging, ADB workflows, log streaming |
| `itchio-pak-itch-scraping` | RSS feed, scraper logic, download flows, testdata fixtures |
| `itchio-pak-device-screenshot` | Capturing a screenshot from real hardware to compare against mockups |

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
```

All also available as `make` targets: `test`, `build-native`, `build-all`, `release`, `deploy`, `clean`.

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

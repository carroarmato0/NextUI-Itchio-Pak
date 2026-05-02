---
name: itchio-pak-device-screenshot
description: Use when you need to verify how a UI screen looks on the real device — deploy the current build, launch directly to a specific screen, wait for it to be ready, and capture a framebuffer screenshot for comparison against the mockup.
---

# Itch.io Pak — Device Screenshot Workflow

Use this skill any time you need to see what a screen looks like on real hardware or compare a code change against a mockup.

## Output Directory Rules

> **AI/Claude rule: ALWAYS use `/tmp/itchio-screenshots/` as the output directory. Never write screenshots to `docs/screenshots/` or anywhere else inside the repository.**

`docs/screenshots/` is reserved exclusively for the developer to populate manually after approving a design. It is not an AI action. If you are Claude or any other AI assistant using this skill, you must only ever write screenshots to `/tmp/itchio-screenshots/`.

## Prerequisites

- Device connected via USB with ADB enabled (NextUI Settings → Developer → ADB over USB)
- `adb devices` shows the device
- `ffmpeg` installed on host (used to convert the raw framebuffer to PNG)
- Docker or Podman installed (for the cross-compile step)

## Quick Reference

```sh
# All screens
./scripts/dev-screenshot.sh --all --out-dir /tmp/itchio-screenshots

# Single screen
./scripts/dev-screenshot.sh --screen list    --out /tmp/itchio-screenshots/list.png
./scripts/dev-screenshot.sh --screen detail  --out /tmp/itchio-screenshots/detail.png
./scripts/dev-screenshot.sh --screen settings --out /tmp/itchio-screenshots/settings.png

# Skip rebuild when the binary hasn't changed
./scripts/dev-screenshot.sh --all --no-build --out-dir /tmp/itchio-screenshots

# Keep the app alive after capture for manual inspection
./scripts/dev-screenshot.sh --screen list --keep-alive --out /tmp/itchio-screenshots/list.png

# Miyoo Flip (640×480)
DEPLOY_PLATFORM=my355 ./scripts/dev-screenshot.sh --all --out-dir /tmp/itchio-screenshots
```

> **Note for developers:** To promote approved screenshots to the repo, run `./scripts/dev-screenshot.sh --all --out-dir docs/screenshots` manually. This is a deliberate developer action, not part of any AI workflow.

## How It Works

`scripts/dev-screenshot.sh` runs these steps automatically:

1. **Cross-compile** for the target platform (`scripts/build.sh <platform>`)
2. **Push the binary** via `adb push`
3. **Kill** any existing running instance
4. **Clear the device log** so readiness polling does not match stale entries
5. **Launch** the app with `DEV_START_SCREEN=<screen>` and `LOG_LEVEL=debug`
6. **Poll the device log** for a screen-specific readiness pattern (up to 30 s)
7. **Wait** an extra settle period for the SDL frame to finish rendering (default 1.5 s)
8. **Capture** the raw framebuffer via `adb shell dd` and convert with ffmpeg
9. **Kill** the app (unless `--keep-alive`)

## Supported Screens

| `--screen` | Readiness pattern | Notes |
|---|---|---|
| `list` | `cache:` in log | Waits for game cache to load; shows list with badges |
| `detail` | `detail:` in log | Auto-navigates to the detail of the first cached game |
| `settings` | `platform=` in log | Settings screen renders immediately after SDL init |

### Adding More Screens

To support a new screen value, add a case in `internal/ui/dev_start.go` → `NewDevStartScreen()` and a readiness pattern in `scripts/dev-screenshot.sh` → the `WAIT_PATTERN` case block.

## Comparing Screenshots vs Mockups

```sh
# Open both side-by-side in your file manager / image viewer
xdg-open /tmp/itchio-screenshots/list.png &
xdg-open docs/mockups/list.html &

# Side-by-side composite with ImageMagick (after converting the HTML mockup)
chromium --headless --screenshot=/tmp/mockup-list.png \
    --window-size=640,480 docs/mockups/list.html 2>/dev/null
convert +append /tmp/mockup-list.png /tmp/itchio-screenshots/list.png /tmp/compare-list.png
xdg-open /tmp/compare-list.png
```

## Reading the Logs

Enable debug logging to see the app state:

```sh
# Stream live log while the app is running
./scripts/debug.sh logs

# Pull log after the fact
./scripts/debug.sh pull-log
cat itchio-pak.log
```

Key log lines to watch for:

| Pattern | Meaning |
|---|---|
| `display: WxH` | SDL initialized, window created |
| `platform=` | App startup header logged |
| `cache: loaded N games` | Game list ready from disk cache |
| `cache: serving page 1` | List is displaying from cache |
| `detail: N screenshots` | Detail screen finished loading data |
| `dev: DEV_START_SCREEN=` | Confirms env var was picked up |

## Increasing Settle Time for Slow Loads

If the screenshot is captured before the UI is fully rendered (e.g., cover art still loading), increase the settle time:

```sh
./scripts/dev-screenshot.sh --screen list --settle 4
```

## Troubleshooting

**"timed out waiting for pattern"** — The app might be stuck loading data. Run `./scripts/debug.sh logs` and watch what the last log line is. Increase `--timeout` if network is slow.

**Screenshot is all-black or shows the NextUI menu** — The app did not get focus. Check that `launch.sh` ran correctly: `adb shell "cat /tmp/pak-dev.log"`.

**Wrong resolution** — Specify `--platform` explicitly if auto-detection fails.

**`nohup` not found** — Try `DEPLOY_PLATFORM=tg5040 ./scripts/debug.sh run` manually to confirm the device shell works, then check if `nohup` is in `/usr/bin/nohup` on the device.

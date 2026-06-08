---
name: itchio-pak-build
description: Use when building, testing, cross-compiling, releasing, or deploying the Itch.io NextUI Pak. Covers script commands, Make targets, container runtime selection, toolchain images, and release artifact structure.
---

# Itch.io Pak — Build & Release Reference

## Quick Commands

| Action | Command |
|--------|---------|
| Run tests | `./scripts/test.sh` |
| Tests + coverage HTML | `./scripts/test.sh --coverage` |
| Build for host machine | `./scripts/build.sh native` |
| Build one platform | `./scripts/build.sh tg5040` |
| Build all platforms | `./scripts/build.sh all` |
| Create release artifacts | `./scripts/release.sh` |
| Deploy via ADB | `./scripts/deploy.sh` |
| Deploy to SD card | `./scripts/deploy.sh /run/media/user/SD` |
| Capture device screenshot | `./scripts/screenshot.sh` |

All commands also available as `make <target>`: `test`, `build-native`, `build-all`, `release`, `deploy`, `clean`.

## ⚠️ Build vs Deploy — Critical Distinction

`build.sh <platform>` and `deploy.sh` are **NOT** a paired workflow:

- `build.sh tg5040` → writes binary to `bin/tg5040/itchio-pak` **only**
- `deploy.sh` → pushes `dist/Itch-io.pak/` which is only populated by `release.sh`

**Running `build.sh` then `deploy.sh` deploys a stale binary from a previous release.**

### For quick dev/test iteration (code changes only):
```sh
./scripts/build.sh tg5040
adb push bin/tg5040/itchio-pak /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio-pak
```

### For a full release deploy (binary + assets + libs):
```sh
./scripts/release.sh   # builds all platforms, populates dist/
./scripts/deploy.sh    # pushes dist/Itch-io.pak/ to device
```

## Container Runtime Selection

`scripts/build.sh` auto-detects at runtime:
1. If `$CONTAINER_RUNTIME` env var is set → use it
2. Else if `podman` found → use podman (**default when both present**)
3. Else if `docker` found → use docker
4. Per-invocation override: `./scripts/build.sh --runtime docker tg5040`

## LoveRetro Toolchain Images

```
ghcr.io/loveretro/tg5040-toolchain:latest
ghcr.io/loveretro/tg5050-toolchain:latest
ghcr.io/loveretro/my355-toolchain:latest
```

These images contain SDL2 compiled for the target device (no X11/PulseAudio/Wayland — only libm/libdl/libpthread/libc). `docker/Dockerfile.platform` layers Go 1.22 (from Alpine) on top of the LoveRetro toolchain to create per-platform build images (`itchio-pak-tg5040-dev` etc.). The tg5040 toolchain lacks `SDL2_ttf.pc`; `Dockerfile.platform` creates it automatically.

## Developer Prerequisites

- **Docker or Podman** — required for build/test; Go, SDL2, and cross-toolchains are all containerised
- **zip** — required on the host for `release.sh` artifact assembly
- **ADB** (`android-tools`) — required only for `deploy.sh` and `debug.sh` over USB; skip if using SD card

`deploy.sh` and `debug.sh` run on the host directly — USB/ADB cannot be cleanly passed into containers.
`release.sh` runs on the host and orchestrates test.sh and build.sh (which manage their own containers).

## Container Boundary

| Script | Container? |
|--------|-----------|
| `test.sh` | Yes — `itchio-pak-dev` image |
| `build.sh native` | Yes — `itchio-pak-dev` image |
| `build.sh tg5040/tg5050/my355` | Yes — `itchio-pak-$PLATFORM-dev` (LoveRetro + Go) |
| `build.sh all` | No — spawns three per-platform containers sequentially |
| `release.sh` | **No — host** (orchestrates other scripts) |
| `deploy.sh` | **No — host** (needs USB/ADB) |
| `debug.sh` | **No — host** (needs USB/ADB) |

## Dev Container (`docker/Dockerfile.dev`)

Go 1.22 + SDL2 dev libs (x86_64 Debian Bullseye). Used by `test.sh` and `build.sh native`.  
Built and cached automatically on first run. Image tag: `itchio-pak-dev`.

## Platform Container (`docker/Dockerfile.platform`)

Built with `--build-arg PLATFORM=tg5040` (or tg5050/my355). Layers Go 1.22 from Alpine on top of the LoveRetro toolchain image. Image tags: `itchio-pak-tg5040-dev`, `itchio-pak-tg5050-dev`, `itchio-pak-my355-dev`. Built and cached automatically on first use.

## Container Self-Re-Invocation

Scripts re-invoke themselves inside the correct container if `$IN_CONTAINER` is unset:
```sh
if [ -z "${IN_CONTAINER:-}" ]; then
    $RUNTIME run --rm -v "$(pwd):/workspace" -w /workspace \
        -e IN_CONTAINER=1 "$IMAGE" "$0" "$@"
fi
```

## Release Artifact Structure

```
dist/
  Itch-io.pak.zip     # Single zip — all three lib dirs inside, works on all devices
  Itch-io.pakz        # Multi-device bundle (extract to SD card root)
```

### Inside `Itch-io.pak.zip`
```
Itch-io.pak/
  itchio-pak           (ARM64 binary — tg5040 build, works on all platforms)
  launch.sh
  pak.json
  assets/
  lib/
    tg5040/            (LoveRetro SDL2 2.26 for TrimUI Brick / Smart Pro)
    tg5050/            (LoveRetro SDL2 2.32 for TrimUI Smart Pro S)
    my355/             (LoveRetro SDL2 2.33 for Miyoo Flip)
```

### Inside `Itch-io.pakz` (multi-device)
```
Tools/
  tg5040/Itch-io.pak/   itchio-pak (tg5040 binary) + lib/tg5040/
  tg5050/Itch-io.pak/   itchio-pak (tg5050 binary) + lib/tg5050/
  my355/Itch-io.pak/    itchio-pak (my355 binary)  + lib/my355/
```

## CI (GitHub Actions)
Runs `scripts/test.sh` on every push. No Docker/device needed.
SDL2 renderer excluded via build tag: `//go:build !headless`
`--headless` flag in `main.go` skips SDL2 init for CI.

## launch.sh Pattern
```sh
#!/bin/sh
PAK_DIR="$(dirname "$0")"
# Detect platform → select lib dir (tg5050/tg5040/my355)
# Search /usr/trimui/lib, /usr/miyoo/lib, /usr/lib, /usr/local/lib for native SDL2
# Native SDL2 goes first in LD_LIBRARY_PATH; bundled LoveRetro SDL2 is the fallback
export LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+$NATIVE_SDL_LIB:}$PLATFORM_LIB:$LD_LIBRARY_PATH"
exec "$PAK_DIR/itchio-pak"
```

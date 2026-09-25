---
name: itchio-pak-build
description: Use when building, testing, cross-compiling, releasing, or deploying Itch-io (the NextUI/muOS app). Covers script commands, Make targets, build targets, container runtime selection, toolchain images, release artifact structure, and publishing to GitHub.
---

# Itch-io — Build & Release Reference

The binary is `itchio`. "Pak" is NextUI's packaging format, so it appears in
NextUI artifact names (`Itch-io.pak/`, `pak.json`, `launch.sh`) and nowhere else
— muOS packaging has none of it.

`scripts/targets.sh` is the single source of truth for what gets built and
shipped. Every script and the Makefile source it. When this file and
`targets.sh` disagree, `targets.sh` wins.

## Quick Commands

| Action | Command |
|--------|---------|
| Run tests | `./scripts/test.sh` |
| Tests + coverage HTML | `./scripts/test.sh --coverage` |
| Build for host machine | `./scripts/build.sh native` |
| Build one target | `./scripts/build.sh nextui/tg5040` |
| Build one firmware | `./scripts/build.sh nextui` |
| Build everything | `./scripts/build.sh all` |
| Create release artifacts | `./scripts/release.sh` |
| Publish a GitHub release | `./scripts/release-github.sh` |
| Publish a test build | `./scripts/release-github.sh --prerelease` |
| Deploy to NextUI via ADB | `./scripts/deploy.sh` |
| Deploy to NextUI SD card | `./scripts/deploy.sh /run/media/user/SD` |
| Deploy to muOS via ADB | `./scripts/deploy.sh muos` |
| Deploy the muOS installer | `./scripts/deploy.sh muxapp` |
| Stream the device log | `./scripts/debug.sh logs` |
| Capture device framebuffer | `./scripts/screenshot.sh` |
| Render screens offscreen | `go run ./cmd/devshot --all` |

Make targets: `test`, `test-coverage`, `build-native`, `build-nextui`,
`build-muos`, `build-nextui-tg5040`, `build-nextui-tg5050`, `build-nextui-my355`,
`build-muos-arm64`, `build-all`, `release`, `deploy`, `deploy-sd` (needs `SD=`),
`deploy-adb`, `debug-logs`, `debug-push`, `debug-run`, `clean`.
`build-tg5040`/`build-tg5050`/`build-my355` are deprecated aliases from before
firmware and device were separated — removable after one release.

## Build Targets

Declared in `scripts/targets.sh` as `<firmware>:<device>:<toolchain>:<bundle_sdl>:<source>:<max_glibc>`.
Adding a firmware should be one row there plus a packaging rule in `release.sh`.

| Target | Device | Toolchain | Bundles SDL2 | Notes |
|--------|--------|-----------|--------------|-------|
| `nextui/tg5040` | TrimUI Brick + Smart Pro | tg5040 | yes | The portable build. Hard GLIBC_2.17 ceiling enforced by `build.sh`; ships in the single pak zip and is copied by h700 and muOS |
| `nextui/tg5050` | TrimUI Smart Pro S | tg5050 | yes | Toolchain emits GLIBC_2.32; only ever runs on tg5050, so left unconstrained |
| `nextui/my355` | Miyoo Flip | my355 | yes | |
| `nextui/h700` | Anbernic RG XX family | — | **no** | A **copy** of `nextui/tg5040`. NextUI installs its own mali-fbdev SDL2 in `.system/h700/lib`; the pak must ship none |
| `muos/arm64` | Every muOS device | — | **no** | A **copy** of `nextui/tg5040`. muOS ships patched SDL2 2.30 + SDL2_ttf 2.22 |

Only three targets are compiled; two are copies. `compiled_targets()` and
`all_toolchains()` in `targets.sh` are what the build loop uses.

**Do not "fix" the copies into separate compiles.** Each would add ~14MB to the
single zip the Pak Store fetches, paid for by every user, for a byte-equivalent
binary. `targets.sh` records exactly what would have to change if H700 ever
needs a real compile (the row, two CGo env vars in `Dockerfile.toolchain`, and
gating the SDL2 harvest on `target_bundles_sdl`).

## ⚠️ Build vs Deploy — Critical Distinction

`build.sh` and `deploy.sh` are **NOT** a paired workflow:

- `build.sh nextui/tg5040` → writes `bin/nextui/tg5040/itchio` **only**
- `deploy.sh` → pushes `dist/nextui/Itch-io.pak/`, which only `release.sh` populates

**Running `build.sh` then `deploy.sh` deploys a stale binary from a previous release.**

### For quick dev/test iteration (code changes only):
```sh
./scripts/build.sh nextui/tg5040
adb push bin/nextui/tg5040/itchio /mnt/SDCARD/Tools/tg5040/Itch-io.pak/itchio
```
Or `./scripts/debug.sh push`, which does the build-and-push for `$DEPLOY_PLATFORM`.

### For a full release deploy (binary + assets + libs):
```sh
./scripts/release.sh   # tests, builds every target, populates dist/
./scripts/deploy.sh    # pushes dist/nextui/Itch-io.pak/ to the device
```

`build.sh native` writes `bin/host/native/itchio` — host x86_64, for running
the app on the desktop.

## Container Runtime Selection

`scripts/build.sh` and `scripts/test.sh` auto-detect at runtime:
1. `$CONTAINER_RUNTIME` if set to `docker` or `podman`
2. Else `podman` if found (**preferred when both are present**)
3. Else `docker`
4. `build.sh` also takes a per-invocation `--runtime docker nextui/tg5050`

## Toolchain Images

```
ghcr.io/loveretro/tg5040-toolchain:latest
ghcr.io/loveretro/tg5050-toolchain:latest
ghcr.io/loveretro/my355-toolchain:latest
```

These contain SDL2 compiled for the target device (no X11/PulseAudio/Wayland —
only libm/libdl/libpthread/libc). `docker/Dockerfile.toolchain` takes
`--build-arg TOOLCHAIN=<code>` and layers Go 1.27 (from Alpine) on top, tagging
the result `itchio-toolchain-<toolchain>`. The tg5040 toolchain ships
`libSDL2_ttf` but no `SDL2_ttf.pc`; the Dockerfile writes a minimal one so
pkg-config can find it.

A `ghcr.io/loveretro/h700-toolchain` image exists but is **not used** — h700 is
a copy of the tg5040 build. See the reasoning in `targets.sh`.

## Developer Prerequisites

- **Docker or Podman** — required for build/test; Go, SDL2 and the cross-toolchains are all containerised
- **zip** — required on the host for `release.sh` artifact assembly
- **gh** (authenticated) **and jq** — required for `release-github.sh`
- **ADB** (`android-tools`) — required only for `deploy.sh`, `debug.sh` and `screenshot.sh` over USB; skip if installing via SD card
- **ffmpeg** — required by `screenshot.sh` to decode the framebuffer
- **shellcheck** — optional; `test.sh` lints the device launchers when it is present

`deploy.sh`, `debug.sh` and `screenshot.sh` run on the host directly — USB/ADB
cannot be cleanly passed into containers. `release.sh` and `release-github.sh`
also run on the host; `release.sh` orchestrates `test.sh` and `build.sh`, which
manage their own containers.

## Container Boundary

| Script | Container? |
|--------|-----------|
| `test.sh` (Go tests) | Yes — `itchio-dev` image |
| `test.sh` (shell tests, shellcheck, palette audit) | No — host stage, before the container stage |
| `build.sh native` | Yes — `itchio-dev` image |
| `build.sh <compiled target>` | Yes — `itchio-toolchain-<toolchain>` (LoveRetro + Go) |
| `build.sh <firmware>` / `all` | No — spawns the per-toolchain containers sequentially |
| `release.sh` | **No — host** (orchestrates other scripts) |
| `deploy.sh` / `debug.sh` / `screenshot.sh` | **No — host** (needs USB/ADB) |
| `release-github.sh` | **No — host** (needs gh) |

### Images

- `itchio-dev` — `docker/Dockerfile.dev`, Go 1.27 + SDL2 dev libs (x86_64 Debian Bookworm; host-only, so its glibc never reaches a device). Used by `test.sh` and `build.sh native`.
- `itchio-toolchain-{tg5040,tg5050,my355}` — `docker/Dockerfile.toolchain`.

Both are built and cached automatically on first use. `make clean` removes them,
plus the legacy `itchio-pak-*` tags from before the rename.

### Container Self-Re-Invocation

Scripts re-invoke themselves inside the right container when `$IN_CONTAINER` is unset:
```sh
if [ -z "${IN_CONTAINER:-}" ]; then
    $RUNTIME run --rm -v "$(pwd):/workspace" -w /workspace \
        -e IN_CONTAINER=1 "$IMAGE" "$0" "$@"
fi
```

## What `test.sh` Actually Runs

Host stage first, then the Go stage in `itchio-dev`:

1. `release-github_test.sh` (skipped if `jq` is missing)
2. `release-pak_test.sh` — structural assertions on the NextUI archives
3. `release-muxapp_test.sh` — structural assertions on the muOS archive
4. `launch_test.sh` — 13 cases over `launch.sh`'s platform × `$SYSTEM_PATH` matrix
5. `shellcheck` on the device launchers (skipped if absent)
6. `no-color-literals.sh`
7. `palette-audit.sh` — 12 scenes × every palette (NextUI ships 18, plus the built-in default) at each of the four shipping geometries: 1024×768, 640×480, 720×480, 720×720 — 228 renders per geometry
8. `go test -tags headless ./...` in the container
9. `go test ./internal/ui` non-headless, which is the only compile check on SDL2 code

**The archive tests skip silently when `dist/` is empty.** That is why
`release.sh` runs `release-pak_test.sh` and `release-muxapp_test.sh` a second
time after building, against the artifacts it just produced — and it is what
enforces "no artifact may contain `lib/h700/`".

## Release Artifact Structure

`release.sh` refuses to package a dirty tree (`build.sh` would stamp the binary
`<commit>-dirty`, so the version could not be traced to a commit). Pass
`--allow-dirty` only for packaging experiments.

```
dist/
  Itch-io.pak.zip                              # copy of the NextUI pak zip under the bare name pak.json's release_filename points the Pak Store at
  nextui/
    Itch-io.NextUI.<version>.pak.zip           # single pak, all lib dirs inside, installs on any NextUI device
    Itch-io.NextUI.<version>.pakz              # multi-device bundle, extract at SD root
    Itch-io.pak/                               # left unzipped on purpose — this is what deploy.sh pushes
  muos/
    Itch-io.muOS.<version>.muxapp              # installed via muOS Archive Manager
    Itch-io/                                   # left unzipped — deploy.sh muos pushes this
```

### Inside `Itch-io.NextUI.<version>.pak.zip`
```
Itch-io.pak/
  itchio               (ARM64 — the nextui/tg5040 build, used on every NextUI device)
  launch.sh
  pak.json
  assets/
  lib/
    tg5040/            (LoveRetro SDL2 for TrimUI Brick / Smart Pro)
    tg5050/            (LoveRetro SDL2 for TrimUI Smart Pro S)
    my355/             (LoveRetro SDL2 for Miyoo Flip)
```
No `lib/h700/` — h700 links the SDL2 NextUI installs in `.system/h700/lib`, and
a bundled dir would put ours ahead of it. SDL2 files are copied with `cp -L` so
FAT32 SD cards get real files rather than symlinks.

### Inside `Itch-io.NextUI.<version>.pakz`
```
Tools/
  tg5040/Itch-io.pak/   itchio + lib/tg5040/
  tg5050/Itch-io.pak/   itchio + lib/tg5050/
  my355/Itch-io.pak/    itchio + lib/my355/
  h700/Itch-io.pak/     itchio, no lib dir
```
Each platform dir carries its own copy of the binary. Three of the four are
byte-identical (h700 and muOS copy tg5040), which is why the `.pakz` is ~50MB
against the `.pak.zip`'s ~13.8MB. Deduplicating is a known deferred follow-up —
it changes packaging for every platform, and the zip almost everyone installs is
unaffected.

### Inside `Itch-io.muOS.<version>.muxapp`
```
Itch-io/
  itchio               (chmod +x)
  mux_launch.sh        (chmod +x — without the exec bit the app simply does not start)
  mux_lang.ini
  glyph/
  assets/
  version.txt          (firmware-neutral version marker; muOS has no pak.json)
```
No `pak.json`, no `launch.sh`, no SDL2 — all three are NextUI packaging and mean
nothing on muOS.

## Publishing — `release-github.sh`

Reads the version and changelog from `pak.json`, verifies `dist/` was built for
that version, pushes the tag, and creates the GitHub release with every
firmware's artifacts attached.

```sh
scripts/release-github.sh                 # full release
scripts/release-github.sh --prerelease    # test build for testers
scripts/release-github.sh --dry-run       # show the plan and notes, touch nothing
scripts/release-github.sh --print-notes   # notes only (used by its tests)
```

`--prerelease` also **withholds the unversioned `Itch-io.pak.zip`**. The Pak
Store finds updates by matching `pak.json`'s `release_filename` against a
release's assets, so a build with no asset under that name cannot be installed
by it — which is the point of a test build, and does not depend on the store
honouring GitHub's pre-release flag.

Pre-releases are tagged `vX.Y.Z-rcN` and cut from `dev`; full releases come off
`main`.

## CI (GitHub Actions)

⚠️ **CI runs almost none of this project's tests.** `.github/workflows/ci.yml`
runs only `go build -tags headless ./...` and `go test -race -tags headless
./...`. It does **not** run `test.sh`. That excludes the non-headless
`internal/ui` compile pass, `launch_test.sh`, the palette audit and all three
archive structural tests — i.e. everything that has actually caught a bug
recently.

`go build -tags headless` does not compile `main_sdl.go`, so **a green CI check
does not mean the device build compiles.** Run `./scripts/test.sh` locally and a
real cross-compile before claiming a build works. (Deferred follow-up: give CI a
container runtime so it can run `test.sh` itself.)

## launch.sh (NextUI only)

Roughly, in order — read the file for the reasoning, which is load-bearing:

1. `$HOME` → `$SHARED_USERDATA_PATH/<pak name>`
2. Select the bundled lib dir from `$PLATFORM` (`tg5040`/`tg5050`/`my355`; **h700 deliberately selects none**), falling back to probing `/usr/miyoo` and `/proc/cpuinfo` for launches that arrive without `$PLATFORM`
3. If `$SYSTEM_PATH/lib` holds a **complete** SDL2 pair (both `libSDL2-2.0.so.0` and `libSDL2_ttf-2.0.so.0`), drop the bundled dir from `LD_LIBRARY_PATH`. A partial pair does not trigger this, and both shapes of partial stay safe
4. Delete the pre-rename `itchio-pak` binary and stale versioned SDL2 siblings left by older installs
5. Search `$SYSTEM_PATH/lib`, `/usr/trimui/lib`, `/usr/miyoo/lib`, `/usr/lib`, `/usr/local/lib` for a native SDL2 — firmware-provided comes first because when a firmware ships its own SDL2 it is authoritative
6. `LD_LIBRARY_PATH="${NATIVE_SDL_LIB:+…:}${PLATFORM_LIB:+…:}$LD_LIBRARY_PATH"` — prepend only, never replace the inherited value
7. `SSL_CERT_FILE` → the bundled CA bundle (devices have no system CA store)
8. `cd "$PAK_DIR"` (the binary loads `assets/font.ttf` relative to cwd), then `exec ./itchio`

`.profile-flags`, written by `debug.sh profile*`, is read here and word-split
into the exec — absent in normal operation.

`launch_test.sh` covers this matrix. Change `launch.sh` and run it.

## muOS packaging (`packaging/muos/`)

`mux_launch.sh`, `mux_lang.ini` and `glyph/` are copied verbatim into the
`.muxapp`. The glyph resolves from the **active muOS theme**, not the app
directory — see the `itchio-pak-project` skill and the muOS memory entries.

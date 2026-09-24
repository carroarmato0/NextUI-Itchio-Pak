# Development

[← Back to the README](../README.md)

Itch-io is written in Go and rendered with SDL2, cross-compiled for ARM64.

---

## Requirements

- Go 1.22+
- Docker or Podman (for cross-compilation)
- `libsdl2-dev`, `libsdl2-ttf-dev`, `libsdl2-image-dev` (for native headless builds)

---

## Common commands

```bash
# Run tests (headless, in container)
make test

# Build native binary (requires local SDL2 dev libs)
make build-native

# Cross-compile every target (all firmwares, all devices)
make build-all

# Cross-compile one firmware, or one target
./scripts/build.sh nextui
./scripts/build.sh nextui/tg5040

# Assemble release zips in dist/
make release

# Deploy to a connected device over ADB
make deploy-adb

# Stream the live log from the device
make debug-logs
```

`./scripts/test.sh` is the real suite — it runs the no-colour-literal check,
the palette audit across four screen geometries, the headless race tests, and a
non-headless `internal/ui` pass. CI runs only a subset, so run it locally before
opening a pull request.

### Build targets

Build targets are `<firmware>/<device>` and are declared once in
`scripts/targets.sh`, which the Makefile and every script source. Adding a
firmware is a one-row change there.

| Target | Device |
|---|---|
| `nextui/tg5040` | TrimUI Brick, TrimUI Smart Pro |
| `nextui/tg5050` | TrimUI Smart Pro S |
| `nextui/my355` | Miyoo Flip |
| `nextui/h700` | Anbernic RG XX family |
| `muos/arm64` | Every muOS device |

A single binary covers every platform; only the bundled SDL2 `.so` files differ.
The `nextui/tg5040` build is the portable one — it holds a GLIBC 2.17 ceiling,
which is what lets the same binary serve muOS.

### Build tags

- `headless` — disables SDL2 rendering; used for CI and unit tests

Note that `go build -tags headless` does not compile the SDL entry point at all,
so a green headless build does not prove the device build works. Run a real
cross-compile before claiming a build is good.

---

## Project layout

```
cmd/itchio-pak/       — main binary entry point
cmd/devshot/          — offscreen screen renderer (PNG, no device needed)
internal/
  firmware/           — NextUI/muOS detection, firmware-specific paths and capabilities
  inventory/          — downloaded-game tracking, update and removal detection
  itchio/             — itch.io client: RSS feed, page scraping, download flow
  logger/             — levelled file logger
  power/              — power button detection via evdev; sleep/shutdown callback
  renderer/           — SDL2 drawing layer, image cache, QR code generation
  roms/               — ROM type detection, destination folder mapping
  settings/           — JSON config read/write
  text/               — font loading and glyph coverage
  theme/              — colour palettes; every colour in the UI comes from here
  ui/                 — screen-based UI (list, detail, fetch, ROM picker, download, settings)
bin/<firmware>/<device>/ — built binaries, e.g. bin/nextui/tg5040/itchio
lib/<toolchain>/      — SDL2 .so files harvested from each toolchain sysroot
packaging/muos/       — mux_launch.sh, glyph and mux_lang.ini for the muOS application
docs/                 — this documentation, screenshots, specs, plans and release notes
scripts/              — build, test, release, deploy, debug, screenshot helpers
scripts/targets.sh    — single source of truth for firmware x device build targets
docker/               — cross-compilation container image
assets/               — font.ttf, fallback fonts, CA certificate bundle
testdata/             — captured HTML/RSS fixtures for offline unit tests
```

For a detailed explanation of how the itch.io web API is used, see
[`itchio-interaction-flow.md`](itchio-interaction-flow.md).

---

## Things that look wrong but are deliberate

- **The HTTP client sends a real User-Agent and a standard TLS handshake.**
  Earlier versions posed as Chrome (uTLS fingerprint plus `sec-ch-ua` /
  `Sec-Fetch-*` headers) to get past Cloudflare. itch.io asked for an honest
  User-Agent and exempted the pages and endpoints the app uses
  ([issue #4](https://github.com/carroarmato0/NextUI-Itchio-Pak/issues/4)), so
  `internal/itchio/useragent.go` builds one from the detected firmware. Do not
  reintroduce browser impersonation; if a request is challenged, report it on
  that issue instead. The aggressive caching still matters — it keeps request
  volume polite.
- **Every colour comes from `internal/theme`.** `scripts/no-color-literals.sh`
  (run by `test.sh`) rejects numeric RGB triples and `uint8` channel arithmetic,
  which wraps on light palettes.
- **All logging goes through `internal/logger`**, never `fmt.Println` or
  `log.Printf`.

---

## Offscreen screenshots

`cmd/devshot` renders real screens to PNG on the host — no device, no display,
no network:

```sh
go run ./cmd/devshot --list
go run ./cmd/devshot --scene detail --palette "Catppuccin Latte"
./scripts/palette-audit.sh   # every scene x palette x geometry, fails on unreadable text
```

Offscreen output is not pixel-identical to the device for text — the device uses
its own bundled SDL2_ttf. Use it for colour and layout work; capture from
hardware when the pixels themselves matter.

---

## Branches

| Branch | Holds |
|---|---|
| `main` | What the Pak Store installs. Updated by merging `dev`. |
| `dev` | Work accumulating towards the next release. May be ahead of the last release. |
| `feature/*` | One change in progress, branched from `dev` and merged back into it. |

Work goes `feature/* → dev → main`. The newest stable release is always tagged
on `main`, and that is what the Pak Store installs.

Not every merge to `main` is a release. Changes that ship nothing to a device —
documentation, build and debug tooling — can land on `main` without a version
bump or a GitHub release. Anything that changes what reaches a device needs one.
After merging `dev` into `main`, merge `main` back into `dev` so `main` stays an
ancestor of `dev` and `git log main..dev` keeps meaning "pending work".

### Releases

Pre-release builds for testers are cut from `dev` and tagged `vX.Y.Z-rcN`:

```bash
./scripts/release.sh                          # test, build every target, package
./scripts/release-github.sh --dry-run --prerelease
./scripts/release-github.sh --prerelease      # publish
```

`--prerelease` marks the GitHub release *and* withholds the unversioned
`Itch-io.pak.zip`. The Pak Store finds updates by matching that filename against
a release's assets, so a build without it cannot reach people on a stable
release — which does not depend on the store also honouring GitHub's
pre-release flag.

A full release is the same commands without `--prerelease`, run on `main` after
merging `dev` and bumping the version in `pak.json`.

---

## Contributing

1. Fork the repository and create a feature branch from `dev`.
2. Make your changes and ensure `./scripts/test.sh` passes.
3. Open a pull request against `dev` — CI will run automatically.

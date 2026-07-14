# Splore Settings Toggle + Cover Art — Design Spec

**Date:** 2026-06-08
**Status:** Approved

## Overview

Add a user-controlled toggle in Settings that enables or disables the `Splore.p8` launcher cart in the Pico-8 ROM directory. The toggle is only visible when the official `pico8` core is active (hidden for `fakeo8`). Enabling the toggle seeds `Splore.p8` into the ROM directory; disabling it removes it. A custom original pixel-art PNG is bundled alongside the cart as cover art.

This feature builds on the existing `EnsureSploreCart` / `CleanSploreCart` functions in `internal/roms/splore.go`.

---

## Config

Add `Pico8Splore bool` to `settings.Config`:

```go
Pico8Splore bool `json:"pico8_splore"`
```

**Default:** `true` — the cart is seeded automatically the first time the user switches to the official core. With `Pico8Core` defaulting to `"fakeo8"`, the `true` default has no effect until the user actively switches cores.

---

## Settings Screen

New constant `sItemPico8Splore` inserted immediately after `sItemPico8Core` in the `settingsItem` iota.

**Visibility:** item is conditionally included in the drawn items list and skipped in `moveCursor` when `cfg.Pico8Core != "pico8"`, matching the existing pattern used by `sItemMusicLocation` (hidden when music download is off) and `sItemNextUITheme` (hidden when theme unavailable).

**Label:** `"Pico-8 Splore Store: ON"` / `"Pico-8 Splore Store: OFF"`

**Toggle in `activate()`:**
```go
case sItemPico8Splore:
    s.cfg.Pico8Splore = !s.cfg.Pico8Splore
    if s.cfg.Pico8Splore {
        if err := roms.EnsureSploreCart(s.cfg.Pico8Core); err != nil {
            logger.Warn("settings: splore cart: %v", err)
        }
    } else {
        if err := roms.CleanSploreCart(s.cfg.Pico8Core); err != nil {
            logger.Warn("settings: splore cart: %v", err)
        }
    }
    if err := s.cfg.Save(s.cfgPath); err != nil {
        logger.Warn("settings: save: %v", err)
    }
    logger.Info("settings: pico8 splore store changed to %v", s.cfg.Pico8Splore)
```

---

## Logic at Call Sites

### Startup — `cmd/itchio-pak/main_sdl.go`

Replace the current unconditional `EnsureSploreCart` call with:

```go
if cfg.Pico8Splore {
    if err := roms.EnsureSploreCart(cfg.Pico8Core); err != nil {
        logger.Warn("startup: splore cart: %v", err)
    }
} else {
    if err := roms.CleanSploreCart(cfg.Pico8Core); err != nil {
        logger.Warn("startup: splore cart: %v", err)
    }
}
```

The `else` branch cleans any stale file from a prior session where Splore was enabled then disabled between runs.

### Core migration — `internal/ui/screen_pico8_core_migrate.go`

`CleanSploreCart(oldCore)` remains unconditional (always remove from the old directory). `EnsureSploreCart(newCore)` is gated on `s.cfg.Pico8Splore`:

```go
if err := roms.CleanSploreCart(s.oldCore); err != nil {
    logger.Warn("pico8-migrate: clean splore cart: %v", err)
}
if s.cfg.Pico8Splore {
    if err := roms.EnsureSploreCart(s.newCore); err != nil {
        logger.Warn("pico8-migrate: seed splore cart: %v", err)
    }
}
```

---

## Cover Art

### Image spec

- **Size:** 128×128 px
- **Palette:** Pico-8 16-color palette
- **Theme:** Space explorer / cartverse
  - Background: deep space — dark blue (`#1d2b53`) with scattered white/yellow star pixels
  - Centrepiece: small retro pixel-art rocket (≈10×18 px) with a flame trail, mid-flight
  - Title: `SPLORE` in large chunky pixel letters (white, upper half)
  - Subtitle: `CARTVERSE` in smaller pink (`#ff77a8`) letters below the title
  - A few tiny floating cart icons to hint at the BBS browser

### Generation

A standalone Go generator program at `cmd/gen-splore-cover/main.go` draws the image pixel-by-pixel using the standard `image` and `image/png` packages. It writes the result to `internal/roms/splore_cover.png`. The generator is run once, its output is committed, and it is not part of the normal build pipeline.

The file carries a `//go:generate go run ../../cmd/gen-splore-cover/main.go` directive in `internal/roms/splore.go` so it can be re-run with `go generate` if the image ever needs updating.

### Embedding

```go
//go:embed splore_cover.png
var sploreCartCoverPNG []byte
```

Declared at package level in `internal/roms/splore.go` (requires `import _ "embed"`).

### Writing and cleaning

`ensureSploreCartInDir` writes `sploreCartCoverPNG` to `dir + ".media/Splore.png"` after creating the `.media/` directory. Skipped if the file already exists (idempotent, same pattern as the cart file).

`cleanSploreCartInDir` removes `dir + ".media/Splore.png"`; a missing file is not an error. The `.media/` directory itself is left in place — other downloaded games' cover art may live there.

---

## Testing

- `TestEnsureSploreCartInDir_CreatesCoverArt` — verify `.media/Splore.png` is created with non-zero content alongside the cart.
- `TestEnsureSploreCartInDir_CoverArtIdempotent` — verify second call does not rewrite the cover art file (mtime unchanged).
- `TestCleanSploreCartInDir_RemovesCoverArt` — verify `.media/Splore.png` is removed by `cleanSploreCartInDir`.
- `TestCleanSploreCartInDir_ToleratesMissingCoverArt` — verify no error when `.media/Splore.png` is absent.
- Existing tests (`TestEnsureSploreCart_FakeoEightIsNoOp`, `TestEnsureSploreCartInDir_Idempotent`, etc.) must continue to pass.

---

## Out of Scope

- The generator program is a dev-time tool only; it does not run on device or during CI.
- The cover art image is not re-downloaded or updated at runtime.
- No UI changes are needed in the settings screen beyond the new toggle row.

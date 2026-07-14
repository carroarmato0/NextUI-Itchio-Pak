# Pico-8 Support — Design Spec

**Date:** 2026-06-05  
**Status:** Approved

## Overview

Add Pico-8 game support to the Itch.io NextUI Pak. Pico-8 games are published on itch.io in two ROM formats: `.p8` (plain-text cartridge) and `.p8.png` (cartridge embedded as pixel data inside a PNG image). The target ROM directory is `/mnt/SDCARD/Roms/Pico-8 (P8)/`. Multi-cart games (ZIPs containing multiple cartridge files) are extracted to a named subdirectory with an auto-generated M3U playlist.

## Core Technical Challenge

`filepath.Ext("game.p8.png")` returns `.png`, causing `.p8.png` files to be misidentified as regular PNG images and silently dropped throughout the pipeline. The fix is a single `ROMExt()` helper that intercepts this compound extension.

---

## Section 1 — `ROMExt()` Helper

**Location:** `internal/roms/roms.go`

```go
func ROMExt(filename string) string {
    if strings.HasSuffix(strings.ToLower(filename), ".p8.png") {
        return ".p8.png"
    }
    return filepath.Ext(filename)
}
```

### Call sites replaced (6 total)

| File | Function | Current problem |
|------|----------|-----------------|
| `roms/roms.go` | `ScoreUpload` | `.p8.png` scores 0 (treated as image) |
| `roms/zip_classify.go` | `ClassifyEntry` | `.p8.png` classified as `KindOther` |
| `itchio/game.go` | `ParseDownloadPage` | `.p8.png` skipped as PNG image |
| `ui/screen_fetch_uploads.go` | `nextScreen()` | wrong `DestinationDir` for `.p8.png` |
| `ui/screen_zip_inspect.go` | `route()` inner-ROM ext lookup | wrong destination for single `.p8.png` in ZIP |
| `ui/screen_zip_download.go` | `extractROM()` | `.p8.png` extracted to wrong directory |

`itchio/cover_art.go` `coverArtBasename()` is **not changed** — it already produces the correct result: `game.p8.png` → stem `game.p8` → art path `.media/game.p8.png`.

---

## Section 2 — Scoring, Destination, and Feed Platform

### Scoring

`.p8.png` is preferred over `.p8` (mirrors `.gbc` > `.gb`):

| Extension | Score |
|-----------|-------|
| `.p8.png` | 2 |
| `.p8` | 1 |

### New constant and `DestinationDir` cases

```go
// internal/roms/roms.go
const Pico8Dir = "/mnt/SDCARD/Roms/Pico-8 (P8)/"

// DestinationDir — new cases:
case ".p8", ".p8.png":
    return Pico8Dir
```

### `romExts` map (`roms/zip_classify.go`)

```go
".p8": true, ".p8.png": true,
```

### Feed platform (`itchio/platforms.go`)

```go
{
    Code:      "P8",
    Name:      "Pico-8",
    FeedSlugs: []string{"tag-pico-8"},
},
```

### `ParseDownloadPage` (`itchio/game.go`)

Add `.p8` and `.p8.png` to the known-ROM branch so they are included with `NeedsFormat=false`. Requires adding `internal/roms` as an import — no circular dependency (roms imports only stdlib and logger).

---

## Section 3 — Cover Art

### `.p8.png` ROMs

The `.p8.png` file is itself a valid PNG image (the cartridge label art). Instead of downloading a separate cover image from itch.io, the already-downloaded ROM file is copied into the `.media/` directory:

```
ROM:  Pico-8 (P8)/game.p8.png
Art:  Pico-8 (P8)/.media/game.p8.png   ← copy of the ROM file
```

This follows the Pico-8 Pak recommendation and saves one network call per download.

**New function:** `CopyCoverArt(romDestPath string) error` in `internal/itchio/cover_art.go`. Creates `.media/` if needed and copies the ROM file there.

### `.p8` ROMs

`DownloadCoverArt` is called normally. Art is saved as `.media/game.png`.

### Call sites updated

All three download screens check `roms.ROMExt(upload.Filename) == ".p8.png"` and call `CopyCoverArt` instead of `DownloadCoverArt`:

- `ui/screen_download.go`
- `ui/screen_zip_download.go` (`extractROM()`)
- `ui/screen_multi_download.go`

---

## Section 4 — Multi-Cart ZIP Handling and M3U Generation

### Detection

New method on `ZIPManifest` (`roms/zip_classify.go`):

```go
func (m ZIPManifest) IsPico8MultiCart() bool {
    p8Count := len(m.ROMsByExt()[".p8"]) + len(m.ROMsByExt()[".p8.png"])
    return p8Count > 1
}
```

### New helper (`roms/roms.go`)

```go
func Pico8MultiCartDir(gameTitle string) string {
    safe := SanitiseFilename(gameTitle, "")
    if safe == "" {
        safe = "Unknown"
    }
    return Pico8Dir + safe + "/"
}
```

### Device layout

```
Pico-8 (P8)/
  GameName/
    cart1.p8
    cart2.p8
    GameName.m3u     ← lists carts alphabetically, one per line, no path prefix
  GameName/.media/
    GameName.png     ← itch.io cover art, keyed to the .m3u file
```

The M3U lives in the same subdirectory as the carts (satisfying the Pico-8 Pak requirement). Cart filenames are listed alphabetically — no attempt to detect a "main" cart; this matches Pico-8 Pak documentation.

### Route change (`ui/screen_zip_inspect.go` `route()`)

Checked **before** `HasDuplicateROMExt()` so multi-cart never reaches the single-selection picker:

```go
if m.IsPico8MultiCart() {
    subDir := roms.Pico8MultiCartDir(s.game.Title)
    plan.DownloadROMs = true
    plan.ROMDirs = map[string]string{".p8": subDir, ".p8.png": subDir}
    // music and MusicDir handled via existing logic if HasMusic()
    return NewZIPDownloadScreen(...)
}
```

### M3U generation (`ui/screen_zip_download.go`)

After all ROMs are extracted, `writePico8M3U(dir, gameTitle, extractedROMs []string) error` writes the playlist file. The M3U is recorded in inventory with a new `FileTypeM3U` constant. Cover art (`DownloadCoverArt`) is called once using the M3U path, not per-cart.

---

## Section 5 — Testing

All tests go in existing test files.

### `roms/roms_test.go`

- `TestROMExt` (new): `game.p8.png` → `.p8.png`, `game.p8` → `.p8`, `game.png` → `.png`, `GAME.P8.PNG` → `.p8.png`
- `TestScoreUpload`: add `.p8.png` → 2, `.p8` → 1
- `TestDestinationDir`: add `.p8` → `Pico8Dir`, `.p8.png` → `Pico8Dir`

### `roms/zip_classify_test.go`

- `TestClassifyEntry`: add `game.p8` → `KindROM`, `cart.p8.png` → `KindROM`, `image.png` → `KindOther`
- `TestIsPico8MultiCart` (new): 2× `.p8` → true; 1× `.p8` + 1× `.p8.png` → true; 1× `.p8` only → false; 0 Pico-8 files → false

No new testdata fixtures needed — Pico-8 changes are in the ROM/manifest layer, not the HTTP scraping layer.

---

## Files Changed Summary

| File | Change |
|------|--------|
| `internal/roms/roms.go` | `ROMExt()`, `Pico8Dir`, `Pico8MultiCartDir()`, `ScoreUpload`, `DestinationDir` |
| `internal/roms/roms_test.go` | New and extended test cases |
| `internal/roms/zip_classify.go` | `romExts`, `ClassifyEntry`, `IsPico8MultiCart()` |
| `internal/roms/zip_classify_test.go` | New and extended test cases |
| `internal/itchio/platforms.go` | P8 feed platform entry |
| `internal/itchio/game.go` | `ParseDownloadPage` — add `.p8`/`.p8.png`, import `roms` |
| `internal/itchio/cover_art.go` | `CopyCoverArt()` |
| `internal/ui/screen_fetch_uploads.go` | `ROMExt` for ext lookup, `CopyCoverArt` call |
| `internal/ui/screen_zip_inspect.go` | `IsPico8MultiCart` branch in `route()` |
| `internal/ui/screen_zip_download.go` | `ROMExt` in `extractROM()`, `writePico8M3U()`, `CopyCoverArt` call |
| `internal/ui/screen_multi_download.go` | `CopyCoverArt` call |
| `internal/inventory/inventory.go` | `FileTypeM3U` constant |

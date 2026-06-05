# Pico-8 Core Selector — Design Spec

**Date:** 2026-06-06  
**Branch:** feature/pico8-support  
**Status:** Approved

## Background

NextUI ships two Pico-8 emulation options:
- **FakeO8** — built-in free core, system code `P8`, ROMs in `Roms/Pico-8 (P8)/`
- **josegonzalez/minui-pico-8-pak** — official paid Pico-8 binary, system code `PICO`, ROMs in `Roms/Pico-8 (PICO)/`

The Itch.io Pak currently hardcodes the `P8` destination. Users who install the official Pico-8 Pak need their downloaded games in `Pico-8 (PICO)/` instead. This feature adds a settings toggle that switches the active core and instantly migrates all existing Pico-8 downloads to the correct directory.

## Design

### 1. Settings

Add one field to `internal/settings/settings.go`:

```go
Pico8Core string `json:"pico8_core,omitempty"` // "fakeo8" | "pico8"
```

`defaults()` sets it to `"fakeo8"`. Omitempty means existing config files without the field load as the default.

| `Pico8Core` value | ROM directory |
|-------------------|---------------|
| `"fakeo8"` (default) | `/mnt/SDCARD/Roms/Pico-8 (P8)/` |
| `"pico8"` | `/mnt/SDCARD/Roms/Pico-8 (PICO)/` |

### 2. `roms` Package Changes

Replace the hardcoded `Pico8Dir` constant and `Pico8GameDir` function with core-aware helpers:

```go
// Pico8ROMDir returns the ROM destination directory for the given core.
// core: "fakeo8" | "pico8" (any other value falls back to "fakeo8")
func Pico8ROMDir(core string) string

// Pico8GameSubDir returns the subdirectory for a multi-file Pico-8 game.
func Pico8GameSubDir(core, gameTitle string) string
```

`DestinationDir` gains a `pico8Core string` parameter:

```go
func DestinationDir(ext, pico8Core string) string
```

All existing call sites pass `cfg.Pico8Core`. The `savestate.go` and `zip_classify.go` files that reference `Pico8Dir` are updated to call `Pico8ROMDir`.

### 3. Migration

New function in `internal/roms/`:

```go
func MigratePico8Files(inv *inventory.Inventory, invPath, oldDir, newDir string) error
```

Algorithm:
1. `os.MkdirAll(newDir)` — ensure destination root exists
2. Iterate all inventory entries; for each `DownloadedFile` where `DestPath` has prefix `oldDir`:
   - Compute `newPath = newDir + strings.TrimPrefix(destPath, oldDir)`
   - `os.MkdirAll(filepath.Dir(newPath))` — creates game subdirs (e.g. `Pico-8 (PICO)/Poom/`)
   - `os.Rename(oldPath, newPath)` — atomic pointer change on same filesystem, instant
   - Move cover art (best-effort): derive old/new `.media/` paths via `inventory.CoverArtPath`; `os.Rename` if old path exists, skip silently if not
   - On success: `inv.UpdateFile(gameURL, oldPath, file{DestPath: newPath})`
   - On failure: log warning, leave inventory entry unchanged, continue
3. `inv.Save(invPath)` — persist all updated paths in one write
4. Best-effort cleanup: `os.Remove` the old `.media/` dir and old root dir (non-fatal; silently ignored if non-empty due to user-placed files)

Partial failure is safe: any file that failed to move keeps its old `DestPath` in the inventory, so the Pak still knows where it is.

### 4. Settings Screen

A new picker row "Pico-8 Core" is added between the ROM location row and the content filter rows.

Display labels:
- `FakeO8 (default)` → `"fakeo8"`
- `Pico-8 (official)` → `"pico8"`

On value change:
1. Compute `oldDir = Pico8ROMDir(previousValue)`, `newDir = Pico8ROMDir(newValue)`
2. Call `roms.MigratePico8Files(inv, invPath, oldDir, newDir)`
3. On success: save config, show brief status line "Pico-8 files moved to Pico-8 (PICO)/" (or P8)
4. On error: show warning line, do **not** save config (leaves user in consistent state with no partial setting change)

No confirmation prompt — the operation is instant and fully reversible (switching back runs the same migration in reverse).

### 5. Testing

**`internal/roms/` (new tests):**
- `TestPico8ROMDir` — correct path for each core value; unknown value falls back to FakeO8
- `TestDestinationDir_Pico8Core` — `.p8` and `.p8.png` route to correct dir per core
- `TestMigratePico8Files` — temp dirs with dummy ROM + cover art; inventory populated; asserts files at new paths, inventory DestPaths updated, old paths gone
- `TestMigratePico8Files_PartialFailure` — one file made unreadable; others still migrate successfully; inventory consistent

**`internal/settings/` (new tests):**
- `TestPico8CoreDefault` — `defaults()` returns `"fakeo8"`
- `TestPico8CoreRoundtrip` — save/load preserves the value; JSON missing the field loads as `"fakeo8"`

**`internal/inventory/`:** No new tests needed — `UpdateFile` and `Save` are already covered; migration tests exercise the full path.

## Out of Scope

- Artwork in `.res/` subdirectory (NextUI alternate art path) — not currently written by the Pak, so not migrated
- Splore integration — the josegonzalez Pak's `Splore.p8` stub file is user-managed, not Itch.io Pak inventory
- Detecting whether the josegonzalez Pak is actually installed — the setting is a user declaration; if they set "Pico-8 (official)" without the Pak installed, games simply won't launch (same as any missing emulator)

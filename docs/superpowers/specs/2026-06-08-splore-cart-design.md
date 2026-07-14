# Splore Cart Seeding — Design Spec

**Date:** 2026-06-08
**Status:** Approved

## Overview

Seed a `Splore.p8` cartridge into the active Pico-8 ROM directory at startup. When launched on the official Pico-8 binary it opens the built-in BBS browser (`splore()`); on Fake08 it displays an informative message explaining that splore is not supported on this core.

## Cart Content

```
pico-8 cartridge // http://www.pico-8.com
version 16
__lua__
function _init()
  if type(splore)=="function" then
    splore()
  else
    cls(1)
    print("splore not available",4,52,7)
    print("requires official pico-8",4,60,6)
    print("not supported by this core",4,68,13)
  end
end
```

The cart checks at runtime whether `splore` is a callable function before invoking it, so it degrades gracefully on any core that does not implement the BBS browser.

## New Code — `internal/roms/splore.go`

Two exported functions, both logging warnings on error but treating them as non-fatal:

### `EnsureSploreCart(core string) error`

1. Resolves the ROM directory via `Pico8ROMDir(core)`.
2. Creates the directory with `os.MkdirAll` if it does not exist.
3. Writes `Splore.p8` only if the file is not already present (idempotent).
4. Returns any OS error.

### `CleanSploreCart(core string) error`

1. Resolves the ROM directory via `Pico8ROMDir(core)`.
2. Deletes `Splore.p8` if it exists.
3. A missing file is not an error.
4. Returns any OS error other than `os.IsNotExist`.

Cart content stored as a package-level `const sploreCartContent` string.

## Callers

### Startup — `cmd/itchio-pak/main_sdl.go`

Call `roms.EnsureSploreCart(cfg.Pico8Core)` early in the startup sequence (after config is loaded). Log a warning on error; do not abort startup.

### Core migration — `internal/ui/screen_pico8_core_migrate.go`

In the `startMigration` goroutine, after `inventory.MigratePico8Files` succeeds:

1. Call `roms.CleanSploreCart(oldCore)` — best-effort, log warning on error.
2. Call `roms.EnsureSploreCart(newCore)` — best-effort, log warning on error.

Both calls happen before the config is saved so the directory state matches the new core before any further operations.

## Testing — `internal/roms/splore_test.go`

- **Ensure creates dir and file** — call `EnsureSploreCart` on a non-existent temp dir; verify dir and `Splore.p8` are created with correct content.
- **Ensure is idempotent** — call twice; verify the file is not rewritten (mtime unchanged or content identical).
- **Clean removes file** — call `CleanSploreCart` after `Ensure`; verify file is gone.
- **Clean tolerates missing file** — call `CleanSploreCart` with no prior `Ensure`; verify no error returned.

## Error Handling

Both functions are best-effort at the call sites. A failure to seed or clean the cart is logged at `Warn` level and does not interrupt startup or migration. The cart is a convenience utility, not a required game file.

## Out of Scope

- Detecting whether the official Pico-8 binary is actually installed — the cart message handles this gracefully at runtime.
- Updating the cart content when the app upgrades — `EnsureSploreCart` is idempotent (skip-if-exists), so a manual delete will trigger a re-seed on next startup if the content ever needs refreshing.

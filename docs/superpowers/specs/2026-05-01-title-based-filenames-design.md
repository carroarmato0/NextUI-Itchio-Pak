# Title-Based Filenames ("Use game title as filename")

**Date:** 2026-05-01
**Status:** Approved
**Related issue:** https://github.com/carroarmato0/NextUI-Itchio-Pak/issues/3

## Overview

Some itch.io games ship uploads with vague, meaningless filenames (e.g. "Game Boy ROM",
"Analogue Pocket ROM") that don't reflect the game title. When downloaded, these produce
unhelpful ROM filenames on disk. This feature renames downloaded ROMs to match the game
title and chosen extension (e.g. `Doomslinger Dungeon.gb`), making the library easier to
navigate and cover art matching reliable.

The feature is on by default globally and can be disabled per-game. Migration between
modes is user-triggered, never silent or bulk. Save game files are detected before any
rename and the user is prompted before they are touched.

**Out of scope:** `.pocket` file support. Support for that format requires confirming
which NextUI emulator directory handles it; adding it to `DestinationDir()` in `roms.go`
is a one-liner once confirmed.

---

## 1. Inventory Schema

### `DownloadedFile` — one new field

```go
type DownloadedFile struct {
    Filename     string    `json:"filename"`                 // upstream name — never modified
    DestPath     string    `json:"dest_path"`                // actual path on disk
    DownloadedAt time.Time `json:"downloaded_at"`
    UnifiedName  bool      `json:"unified_name,omitempty"`  // true = stored under game title
}
```

`Filename` holds the original upstream name (e.g. `"Game Boy ROM.gb"`) and is never
changed. It remains the key used by `HasPendingUpdates` for update detection — that
function requires **zero changes**. `DestPath` holds the actual on-disk path, which may
now use the game-title-based name. `UnifiedName` records that a rename was applied and is
used by the migration logic to determine current state.

### `Entry` — one new field

```go
UnifiedNamingDisabled bool `json:"unified_naming_disabled,omitempty"` // per-game opt-out
```

No changes to `UpstreamFile`, `Inventory`, or any method signatures.

---

## 2. Settings

### New field on `Config`

```go
UnifiedNaming bool `json:"unified_naming"` // default true, no omitempty
```

`defaults()` sets `UnifiedNaming: true`. Because `Load()` calls `defaults()` before
unmarshalling, old config files without the field inherit the default with no migration
needed. `false` is written to disk without `omitempty` so an explicit user opt-out
survives a save/load cycle.

### UI label

Settings screen toggle: **"Use game title as filename"**
Sub-label: *"Downloaded ROMs are renamed to match the game title."*
A note below reads: *"Applies to new downloads. To rename existing files, use Manage Downloads."*

---

## 3. Download Flow (initial download)

After a successful file write, before `inventory.Add`, a unified-naming check runs.

**Enabled when:** `config.UnifiedNaming && !entry.UnifiedNamingDisabled`
(first-time downloads where no entry exists yet treat the game as "not disabled".)

**If enabled:**
1. Derive virtual filename: `<GameTitle>.<ext>` — sanitise by stripping ` / : ? * " < > | `,
   trimming whitespace, and collapsing repeated spaces.
2. If virtual filename == upstream filename: no rename needed.
3. Rename the just-downloaded file in-place (same directory, no data copy).
4. Rename cover art in `.media/` if it already exists at the old stem (covers re-downloads).
5. Record `DownloadedFile{Filename: upstreamName, DestPath: newPath, UnifiedName: true}`.

**Collision guard:** if `<GameTitle>.<ext>` already exists and is not the file just
downloaded, append ` (2)` before the extension: `<GameTitle> (2).<ext>`.

**If disabled:** record `DownloadedFile{Filename: upstreamName, DestPath: originalPath,
UnifiedName: false}`. No rename.

The download completion state on `screen_download` shows the final filename:
```
Download complete
Saved as: Doomslinger Dungeon.gb
Location: /mnt/SDCARD/Roms/Game Boy (GB)/
```

---

## 4. Update Download Flow

When downloading an update for a game that already has `UnifiedName: true`, a new screen
**`screen_rename_confirm`** is inserted after the format/location pickers and before
`screen_download`.

```
Rename to game title?

  Upstream filename : Game Boy ROM v2.gb
  Will be saved as  : Doomslinger Dungeon.gb

  [A] Confirm    [B] Keep original name
```

- **Confirm:** proceeds with unified naming (`UnifiedName: true`).
- **Keep original name:** downloads under the upstream filename (`UnifiedName: false`).
  This is a one-time choice for this file; it does not change `UnifiedNamingDisabled`.

This screen is skipped entirely for first-time downloads.

---

## 5. Migration Logic

File: `internal/inventory/migrate.go`

```go
func MigrateFile(inv *Inventory, gameURL string, file DownloadedFile,
                 gameTitle string, enable bool) (MigrateResult, error)
```

Migration is triggered only when the user explicitly toggles the per-game setting from
Manage Downloads. Changes to the global setting apply to future downloads only.

### Enabling unified naming (`enable=true`)

1. Derive virtual filename (same sanitisation as §3).
2. If virtual filename == current basename of `DestPath`: mark `UnifiedName=true`, done.
3. Check for save game at old path (see §6). If found, push `screen_save_migrate` before
   continuing; abort if user cancels.
4. `os.Rename` ROM file.
5. `os.Rename` cover art in `.media/` if present.
6. Update `DownloadedFile.DestPath`, set `UnifiedName=true`.
7. `inv.Save(path)`.

### Disabling unified naming (`enable=false`)

1. Target filename is `DownloadedFile.Filename` (stored upstream name).
2. If `UnifiedName` is already `false`: no-op.
3. Check for save game at current (virtual) path → prompt.
4. `os.Rename` ROM back to upstream name.
5. `os.Rename` cover art back.
6. Update `DownloadedFile.DestPath`, set `UnifiedName=false`.
7. `inv.Save(path)`.

### Error handling

If any filesystem operation fails mid-migration, the function returns the error and leaves
the inventory unchanged. The error message names the specific file that needs attention. The
inventory is written only after all renames succeed.

---

## 6. Save Game Handling

### Path derivation

New helper: `internal/roms/savegame.go`

```go
func SaveGamePath(romDestPath string) string
```

Maps ROM directory → save directory and appends `.sav` to the full ROM basename:

| ROM directory           | Save directory              |
|-------------------------|-----------------------------|
| `Game Boy (GB)/`        | `/mnt/SDCARD/Saves/GB/`     |
| `Game Boy Color (GBC)/` | `/mnt/SDCARD/Saves/GBC/`    |

Example:
```
/mnt/SDCARD/Roms/Game Boy (GB)/Doomslinger Dungeon.gb
  → /mnt/SDCARD/Saves/GB/Doomslinger Dungeon.gb.sav
```

### Prompt — `screen_save_migrate`

```
Save file detected

  A save file exists for this game:
  GB/Doomslinger Dungeon.gb.sav

  Rename it to match the new ROM name?
  If you skip this, your save will not
  load until renamed manually.

  [A] Rename save   [B] Skip
```

- **Rename save:** `os.Rename` old save → new save. On failure, abort the ROM rename and
  return the error. The inventory is not updated.
- **Skip:** migration continues without touching the save. The user accepts the orphan.

### Overwrite guard

If a save already exists at the new target path:

```
A save already exists at the new path.
Overwrite it?   [A] Yes   [B] Cancel
```

Cancelling aborts the entire migration. This protects against an incompatible save from a
different version of the game being silently overwritten.

---

## 7. UI / UX

### `screen_settings` — global toggle

New row in the existing settings list:

```
Use game title as filename   [ON]
```

Changing this saves `config.UnifiedNaming` immediately. No migration is triggered.

### `screen_manage_downloads` — per-game toggle

A secondary action on each game entry, activated with **Y**:

```
Doomslinger Dungeon

  [A] Delete download
  [Y] Disable title filename   (or "Enable title filename" when currently disabled)
  [B] Back
```

Selecting the toggle invokes `MigrateFile`. If a save is detected, `screen_save_migrate`
is pushed first. A confirmation banner is shown on success.

When the global setting is off, the per-game toggle row is greyed out and non-interactive —
`UnifiedNamingDisabled` is only consulted when `config.UnifiedNaming` is true.

### New screens

| Screen                 | Purpose                                                   |
|------------------------|-----------------------------------------------------------|
| `screen_rename_confirm` | Re-prompt before update download when unified naming active |
| `screen_save_migrate`   | Save game detect / rename prompt during migration          |

---

## 8. Invariants & Edge Cases

- `DownloadedFile.Filename` is **write-once** at download time. Nothing modifies it after.
- `HasPendingUpdates`, `CoverArtPath`, and `VerifyAndClean` require no changes.
- A game with multiple downloaded files (e.g. both `.gb` and `.gbc`) migrates each file
  independently; each gets its own save-game check.
- If two files in the same game entry would produce the same virtual filename (same title,
  same extension), the collision guard appends ` (2)` on the second.
- The global setting change does **not** retroactively rename existing downloads. The note
  in the Settings screen communicates this.

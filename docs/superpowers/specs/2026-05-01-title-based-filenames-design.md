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

`MigrateFile` must surface save-game and save-state prompts to the user before it touches
any files. To keep the function testable without a live UI, the caller passes a
`SaveDataCallback` interface:

```go
type SaveDataCallback interface {
    // AskRenameExistingSave is called when a SRAM save exists at the current ROM path.
    // Returns true if the user wants to rename it.
    AskRenameExistingSave(savePath string) bool
    // AskOverwriteExistingSave is called when a SRAM save already exists at the new path.
    // Returns true if the user wants to overwrite it.
    AskOverwriteExistingSave(newSavePath string) bool
    // AskRenameExistingStates is called when one or more save-state files are found.
    // statePaths contains only the paths that actually exist on disk.
    // Returns true if the user wants to rename them all.
    AskRenameExistingStates(statePaths []string) bool
}

type MigrateResult struct {
    ROMRenamed      bool
    CoverArtRenamed bool
    SaveRenamed     bool
    SaveSkipped     bool
    StatesRenamed   []string // paths of state files successfully renamed
    StatesSkipped   []string // paths of state files left untouched (user skipped)
    NewDestPath     string
}

// MigrateFormats carries the user's configured save and state format indices,
// read from /mnt/SDCARD/.userdata/shared/minuisettings.txt before calling.
type MigrateFormats struct {
    SaveFormat           int  // 0=MinUI, 1=Retroarch SRM (compressed), 2=Generic, 3=Retroarch SRM (uncompressed)
    StateFormat          int  // 0=MinUI, 1/2=Retroarch-ish (legacy), 3/4=Retroarch
    UseExtractedFileName bool // mirrors useExtractedFileName from minuisettings.txt
}

func MigrateFile(inv *Inventory, invPath string, gameURL string, file DownloadedFile,
                 gameTitle string, enable bool, formats MigrateFormats,
                 cb SaveDataCallback) (MigrateResult, error)
```

In production the UI screens implement `SaveDataCallback`. In tests a small stub struct
returns predetermined answers, making every user-choice branch directly exercisable.

`MigrateFormats` is populated by reading three keys from
`/mnt/SDCARD/.userdata/shared/minuisettings.txt` immediately before calling `MigrateFile`:

| Key | Field | Default |
|-----|-------|---------|
| `saveFormat=<n>` | `SaveFormat` | `0` (MinUI) |
| `stateFormat=<n>` | `StateFormat` | `0` (MinUI) |
| `useExtractedFileName=<n>` | `UseExtractedFileName` | `false` (0) |

If the file is absent or a key is missing, the field takes its default value.

Migration is triggered only when the user explicitly toggles the per-game setting from
Manage Downloads. Changes to the global setting apply to future downloads only.

### Enabling unified naming (`enable=true`)

1. Derive virtual filename (same sanitisation as §3).
2. If virtual filename == current basename of `DestPath`: mark `UnifiedName=true`, done.
3. Derive old and new SRAM save paths via `SaveGamePath` (see §6). If the ROM is a `.zip`
   and `formats.UseExtractedFileName` is true, call `ZipInnerFilename` first and pass the
   result to `SaveGamePath`. If old path == new path (zip + `useExtractedFileName=1` +
   format 0 — the inner filename doesn't change when the zip is renamed), skip the save
   prompt entirely and proceed without touching the save.
4. If old save path ≠ new save path and a save exists at the old path, push
   `screen_save_migrate`; abort if user cancels.
5. Check for save-state files at old paths (see §6.5). If any exist, push
   `screen_state_migrate`; user may skip without aborting.
6. `os.Rename` ROM file.
7. `os.Rename` cover art in `.media/` if present.
8. Rename SRAM save if step 4 was triggered and user confirmed.
9. Rename each state file the user confirmed in step 5.
10. Update `DownloadedFile.DestPath`, set `UnifiedName=true`.
11. `inv.Save(path)`.

### Disabling unified naming (`enable=false`)

1. Target filename is `DownloadedFile.Filename` (stored upstream name).
2. If `UnifiedName` is already `false`: no-op.
3. Derive old and new save paths (same zip/`useExtractedFileName` logic as enabling, step 3).
   If old path == new path, skip save prompt.
4. If paths differ and a save exists at the current path, push `screen_save_migrate` → prompt.
5. Check for save-state files at current paths → prompt (skippable).
6. `os.Rename` ROM back to upstream name.
7. `os.Rename` cover art back.
8. Rename SRAM save if step 4 was triggered and user confirmed.
9. Rename each state file the user confirmed in step 5.
10. Update `DownloadedFile.DestPath`, set `UnifiedName=false`.
11. `inv.Save(path)`.

### Error handling

If any filesystem operation fails mid-migration, the function returns the error and leaves
the inventory unchanged. The error message names the specific file that needs attention. The
inventory is written only after all renames succeed.

A failed state file rename (after the ROM rename already succeeded) is non-fatal: it is
logged, the path is added to `MigrateResult.StatesSkipped`, and migration continues. State
files are not game data in the way SRAM saves are; a missed state means the user loses a
save point, not save data.

---

## 6. Save Game Handling (SRAM)

### Format settings

The user can configure the SRAM save format in NextUI via `saveFormat` in
`/mnt/SDCARD/.userdata/shared/minuisettings.txt`. The integer values map as follows:

| `saveFormat` | Name | Extension | Extension stripping |
|:---:|---|---|---|
| 0 | MinUI (default) | `.sav` | **No** — full ROM filename kept; e.g. `Game.gb.sav` |
| 1 | Retroarch SRM (compressed) | `.srm` | **Yes** — ROM extension stripped; e.g. `Game.srm` |
| 2 | Generic | `.sav` | **Yes** — ROM extension stripped; e.g. `Game.sav` |
| 3 | Retroarch SRM (uncompressed) | `.srm` | **Yes** — ROM extension stripped; e.g. `Game.srm` |

"Extension stripping" means the ROM's own extension (`.gb`, `.gbc`, …) is removed before
the save extension is appended, matching the behaviour of NextUI's `formatSavePath` helper.

### Path derivation

New helpers: `internal/roms/savegame.go`, `internal/roms/ziputil.go`

```go
// ZipInnerFilename returns the filename of the first recognized ROM file inside a zip
// archive (e.g. "Pokemon - Red Version (USA).gb"). Returns "" if the zip cannot be opened
// or contains no recognized ROM extension.
func ZipInnerFilename(zipPath string) string

// SaveGamePath derives the SRAM save path for a ROM.
// saveFormat: 0=MinUI, 1=Retroarch compressed, 2=Generic, 3=Retroarch uncompressed.
// innerFilename: pass the result of ZipInnerFilename when the ROM is a .zip and
// useExtractedFileName is true; pass "" otherwise. Only affects format 0 output.
// Returns "" for unrecognised ROM directories.
func SaveGamePath(romDestPath string, saveFormat int, innerFilename string) string
```

**`innerFilename` only changes the output for `saveFormat=0`.** For formats 1–3, both the
zip extension (`.zip`, 3 chars) and the inner extension (`.gb`/`.gbc`, 3–4 chars) fall
within NextUI's 3–5 char strip range, so the stripped stem is identical either way.

ROM directory → save directory mapping:

| ROM directory           | Save directory              | Save tag |
|-------------------------|-----------------------------|:--------:|
| `Game Boy (GB)/`        | `/mnt/SDCARD/Saves/GB/`     | `GB`     |
| `Game Boy Color (GBC)/` | `/mnt/SDCARD/Saves/GBC/`    | `GBC`    |
| `Game Boy Advance (GBA)/` | `/mnt/SDCARD/Saves/GBA/`  | `GBA`    |

Examples for `Doomslinger Dungeon.gb` (ROM in `Game Boy (GB)/`):

| saveFormat | innerFilename | Resulting save path |
|:---:|---|---|
| 0 (MinUI) | `""` | `/mnt/SDCARD/Saves/GB/Doomslinger Dungeon.gb.sav` |
| 1 or 3 (Retroarch) | `""` | `/mnt/SDCARD/Saves/GB/Doomslinger Dungeon.srm` |
| 2 (Generic) | `""` | `/mnt/SDCARD/Saves/GB/Doomslinger Dungeon.sav` |

Examples for `Pokemon - Red Version (USA, Europe).zip` (zip ROM in `Game Boy (GB)/`,
inner file `Pokemon - Red Version (USA, Europe).gb`):

| saveFormat | useExtractedFileName | innerFilename passed | Resulting save path |
|:---:|:---:|---|---|
| 0 (MinUI) | false | `""` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).zip.sav` |
| 0 (MinUI) | true | `"Pokemon - Red Version (USA, Europe).gb"` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).gb.sav` |
| 1 or 3 | false | `""` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).srm` |
| 1 or 3 | true | `"Pokemon - Red Version (USA, Europe).gb"` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).srm` *(identical)* |
| 2 | false | `""` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).sav` |
| 2 | true | `"Pokemon - Red Version (USA, Europe).gb"` | `/mnt/SDCARD/Saves/GB/Pokemon - Red Version (USA, Europe).sav` *(identical)* |

**The save-is-a-no-op rule:** when `saveFormat=0` and `useExtractedFileName=true` and the
ROM is a `.zip`, the save path is derived from the inner filename. Renaming the zip does not
rename the inner file, so the save path before and after the zip rename is the same.
`MigrateFile` detects this (old path == new path) and skips the save prompt and save rename.

When checking whether a save exists before migration, `SaveGamePath` is called with the
current ROM path, the active `saveFormat`, and the inner filename (if applicable). Both old
and new paths are derived before any rename takes place.

**Known limitation — format changes:** `SaveGamePath` only checks the path implied by the
*current* `saveFormat`. NextUI itself has no migration routine when the format setting
changes; it simply starts reading/writing the new path and leaves any pre-existing save at
the old path. If the user changed `saveFormat` after creating a save, that save lives at an
old-format path the Pak will not detect. The Pak's behaviour mirrors NextUI's: it does not
scan all four format paths, and will not prompt the user about saves orphaned by a format
change.

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

- **Rename save:** `os.Rename` old save → new save. On failure, abort the entire migration
  and return the error. The inventory is not updated.
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

## 6.5. Save State Handling

### Format settings

The user can configure the save-state format via `stateFormat` in
`/mnt/SDCARD/.userdata/shared/minuisettings.txt`:

| `stateFormat` | Name | Slot 0 | Slots 1–8 | Auto-resume |
|:---:|---|---|---|---|
| 0 | MinUI (default) | `<full>.st0` | `<full>.stN` | `<full>.st9` |
| 1 | Retroarch-ish compressed | `<stem>.state.1` | `<stem>.state.N` | `<stem>.state.auto` |
| 2 | Retroarch-ish uncompressed | `<stem>.state.1` | `<stem>.state.N` | `<stem>.state.auto` |
| 3 | Retroarch compressed | `<stem>.state` | `<stem>.stateN` | `<stem>.state.auto` |
| 4 | Retroarch uncompressed | `<stem>.state` | `<stem>.stateN` | `<stem>.state.auto` |

`<full>` = full ROM filename including extension (e.g. `Doomslinger Dungeon.gb`).
`<stem>` = ROM filename with extension stripped (e.g. `Doomslinger Dungeon`).

Formats 1/2 use `.state.<n>` (dot before the number) by design — this is a preserved legacy
typo in the NextUI source, kept for backward compatibility.

### Path derivation

New helper: `internal/roms/savestate.go`

```go
// SaveStatePaths returns all save-state paths that could exist for a ROM.
// Only paths that actually exist on disk should be presented to the user.
// stateFormat: 0–4 as above.
// innerFilename: same semantics as SaveGamePath — pass ZipInnerFilename result when ROM is
// a .zip and useExtractedFileName is true; pass "" otherwise.
// coreTag and coreName identify the emulator core directory
// (e.g. coreTag="GB", coreName="gambatte").
// Returns nil for unrecognised ROM directories or unknown core names.
func SaveStatePaths(romDestPath string, stateFormat int, innerFilename, coreTag, coreName string) []string
```

**`innerFilename` only changes the output for `stateFormat=0`** (MinUI), which uses the
full filename including extension (`<full>`). For formats 1–4, both `.zip` and `.gb`/`.gbc`
are stripped to the same stem, so the result is identical regardless. The same
save-is-a-no-op rule from §6 applies: if `stateFormat=0` and `useExtractedFileName=true`
and the ROM is a `.zip`, the state paths are based on the inner filename and do not change
when the zip is renamed — so state migration is also a no-op for those files.

State files live under `/mnt/SDCARD/.userdata/shared/<coreTag>-<coreName>/`, not under
`/Saves/`. The core-tag/core-name mapping for supported systems:

| ROM directory           | coreTag | coreName   |
|-------------------------|---------|------------|
| `Game Boy (GB)/`        | `GB`    | `gambatte` |
| `Game Boy Color (GBC)/` | `GBC`   | `gambatte` |
| `Game Boy Advance (GBA)/` | `GBA` | `gpsp`     |

Both GB and GBC share the `gambatte` libretro core; their state directories differ only in
the tag prefix (`GB-gambatte` vs `GBC-gambatte`). All values confirmed from NextUI source.

`SaveStatePaths` returns paths for all 10 slots (slots 0–9, where 9 = auto-resume). The
caller is responsible for filtering to only paths that exist on disk before prompting.

### Prompt — `screen_state_migrate`

Shown after `screen_save_migrate` (if triggered), only when at least one state file exists:

```
Save states detected

  N save state(s) found for this game.
  Rename them to match the new ROM name?
  If you skip, they will not load until
  renamed manually.

  [A] Rename states   [B] Skip
```

Unlike the SRAM save prompt, skipping here is non-fatal — states can be recreated by
playing. The migration continues regardless of the user's choice.

There is no overwrite guard for state files; a state at the new path is simply overwritten
on rename (the user's choice to rename implies intent to replace).

---

## 7. UI / UX

### `screen_settings` — global toggle

New row in the existing settings list:

```
Use game title as filename   [ON]
```

Changing this saves `config.UnifiedNaming` immediately. No migration is triggered.

### `screen_manage_downloads` — per-game toggle

**Current screen structure** (`internal/ui/screen_manage_downloads.go`):
`screen_manage_downloads` is only pushed when a game has **more than one downloaded file**.
It renders a vertical list of file rows (delete per file) plus a "Delete all" row. There is
no Y-button concept and no per-game settings in the current UI. Single-file games handle
deletion via a modal on `screen_detail` directly.

**Required change:** add a "Use game title as filename" toggle row to
`screen_manage_downloads`, and also expose it on `screen_detail` so single-file games can
reach it.

#### On `screen_manage_downloads` (multi-file games)

A new row is appended below the existing "Delete all" row, separated by a second divider:

```
  Game Boy ROM.gb  →  Doomslinger Dungeon.gb
  Old Game Jam Version.gb  →  Doomslinger Dungeon (2).gb
  ─────────────────────────────────────────
  Delete all
  ─────────────────────────────────────────
  Use game title as filename   [ON]
```

Pressing **A** on the toggle row invokes `MigrateFile` for every file in the entry (each
independently). If a save or state is detected for any file, the appropriate prompt is
pushed before that file's rename proceeds. A confirmation banner is shown on completion.

When `config.UnifiedNaming` is globally off, the toggle row is greyed out and
non-interactive — `UnifiedNamingDisabled` is only meaningful when the global setting is on.

#### On `screen_detail` (single-file and multi-file games)

A new action item is added to the detail screen's action list:

```
  [A] Download
  [A] Use game title as filename   [ON]   (shown only when a download exists)
  [B] Back
```

This ensures the per-game toggle is reachable regardless of file count. Activating it
follows the same `MigrateFile` flow as above.

The toggle row is hidden when no download exists for the game (nothing to rename yet).

### Modified existing screens

| Screen | Change |
|--------|--------|
| `screen_manage_downloads` | New toggle row: "Use game title as filename [ON/OFF]" |
| `screen_detail` | New action item: "Use game title as filename [ON/OFF]" (shown when download exists) |

### New screens

| Screen                  | Purpose                                                              |
|-------------------------|----------------------------------------------------------------------|
| `screen_rename_confirm` | Re-prompt before update download when unified naming active          |
| `screen_save_migrate`   | SRAM save detect / rename prompt during migration                    |
| `screen_state_migrate`  | Save-state detect / rename prompt during migration (skippable)       |

---

## 8. Testing

All new code is covered by unit tests. Tests are table-driven where multiple similar cases
exist. Two reference games are used as named fixtures; adding future games means adding a
fixture directory and extending the relevant tables.

### Test fixtures

```
testdata/
  doomslinger-dungeon/
    download_page.html      # captured itch.io download page (vague filenames)
    uploads.json            # parsed upload list returned by FetchUploads
  solastra/
    download_page.html      # regular game with versioned, extension-bearing filename
    uploads.json
```

**Doomslinger Dungeon** (`https://playinstinct.itch.io/doomslinger-dungeon`)
Upstream uploads: `"Game Boy ROM"` (NeedsFormat=true, .gb), `"Analogue Pocket ROM"`
(unsupported .pocket), `"Old Game Jam Version"` (NeedsFormat=true, .gb).
Represents the vague-filename corner case that motivated the feature.

**Solastra** (`https://vorvy.itch.io/solastra`)
Regular game with a properly-named upload (e.g. `"Solastra v1.2.gbc"`).
Represents the common case where the upstream name is already meaningful.

---

### A. Virtual filename sanitisation — `internal/roms/sanitise_test.go`

| # | Input title | Input ext | Expected output |
|---|-------------|-----------|-----------------|
| 1 | `"Doomslinger Dungeon"` | `.gb` | `"Doomslinger Dungeon.gb"` |
| 2 | `"Solastra"` | `.gbc` | `"Solastra.gbc"` |
| 3 | `"My Game: Subtitle"` | `.gb` | `"My Game Subtitle.gb"` (`:` stripped) |
| 4 | `"Game/Title"` | `.gb` | `"GameTitle.gb"` (`/` stripped) |
| 5 | `"  Spaced  Title  "` | `.gb` | `"Spaced Title.gb"` (trimmed + collapsed) |
| 6 | `"Game * Name"` | `.gb` | `"Game  Name.gb"` → collapsed → `"Game Name.gb"` |
| 7 | `"Game Boy ROM"` | `.gb` | `"Game Boy ROM.gb"` (title equals upstream — rename needed only if game title differs) |
| 8 | `""` | `.gb` | returns upstream filename unchanged (empty title fallback) |

---

### B. Save game path derivation — `internal/roms/savegame_test.go`

Tests are parameterised by `saveFormat`. The table below covers all four formats for the
two reference games; the format column uses the integer value.

| # | saveFormat | innerFilename | ROM path | Expected save path |
|---|:---:|---|----------|--------------------|
| 1 | 0 | `""` | `.../Roms/Game Boy (GB)/Doomslinger Dungeon.gb` | `.../Saves/GB/Doomslinger Dungeon.gb.sav` |
| 2 | 1 | `""` | `.../Roms/Game Boy (GB)/Doomslinger Dungeon.gb` | `.../Saves/GB/Doomslinger Dungeon.srm` |
| 3 | 2 | `""` | `.../Roms/Game Boy (GB)/Doomslinger Dungeon.gb` | `.../Saves/GB/Doomslinger Dungeon.sav` |
| 4 | 3 | `""` | `.../Roms/Game Boy (GB)/Doomslinger Dungeon.gb` | `.../Saves/GB/Doomslinger Dungeon.srm` |
| 5 | 0 | `""` | `.../Roms/Game Boy Color (GBC)/Solastra.gbc` | `.../Saves/GBC/Solastra.gbc.sav` |
| 6 | 1 | `""` | `.../Roms/Game Boy Color (GBC)/Solastra.gbc` | `.../Saves/GBC/Solastra.srm` |
| 7 | 2 | `""` | `.../Roms/Game Boy Color (GBC)/Solastra.gbc` | `.../Saves/GBC/Solastra.sav` |
| 8 | 0 | `""` | `.../Roms/Unknown Emulator/foo.rom` | `""` (unrecognised directory) |
| 9 | 0 | `""` | `.../Roms/Game Boy (GB)/My Game v1.2.gb` | `.../Saves/GB/My Game v1.2.gb.sav` |
| 10 | 2 | `""` | `.../Roms/Game Boy (GB)/My Game v1.2.gb` | `.../Saves/GB/My Game v1.2.sav` |
| 11 | 0 | `""` | `.../Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip` | `.../Saves/GB/Pokemon - Red Version (USA, Europe).zip.sav` |
| 12 | 0 | `"Pokemon - Red Version (USA, Europe).gb"` | `.../Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip` | `.../Saves/GB/Pokemon - Red Version (USA, Europe).gb.sav` |
| 13 | 1 | `""` | `.../Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip` | `.../Saves/GB/Pokemon - Red Version (USA, Europe).srm` |
| 14 | 1 | `"Pokemon - Red Version (USA, Europe).gb"` | `.../Roms/Game Boy (GB)/Pokemon - Red Version (USA, Europe).zip` | `.../Saves/GB/Pokemon - Red Version (USA, Europe).srm` *(same as #13)* |

---

### B2. Save state path derivation — `internal/roms/savestate_test.go`

Tests are parameterised by `stateFormat`. Each case asserts the full list of 10 slot paths
returned by `SaveStatePaths`. Only non-empty returns are shown in detail; format 0 uses
`<full>` (extension kept), formats 1–4 use `<stem>` (extension stripped).

`statesBase` = `/mnt/SDCARD/.userdata/shared/`

| # | stateFormat | innerFilename | ROM basename | Expected slot 0 path | Expected slot 5 path | Expected auto path |
|---|:---:|---|---|---|---|---|
| 1 | 0 | `""` | `Doomslinger Dungeon.gb` | `…/GB-<core>/Doomslinger Dungeon.gb.st0` | `…/GB-<core>/Doomslinger Dungeon.gb.st5` | `…/GB-<core>/Doomslinger Dungeon.gb.st9` |
| 2 | 1 | `""` | `Doomslinger Dungeon.gb` | `…/GB-<core>/Doomslinger Dungeon.state.1` | `…/GB-<core>/Doomslinger Dungeon.state.5` | `…/GB-<core>/Doomslinger Dungeon.state.auto` |
| 3 | 3 | `""` | `Doomslinger Dungeon.gb` | `…/GB-<core>/Doomslinger Dungeon.state` | `…/GB-<core>/Doomslinger Dungeon.state5` | `…/GB-<core>/Doomslinger Dungeon.state.auto` |
| 4 | 0 | `""` | `Solastra.gbc` | `…/GBC-<core>/Solastra.gbc.st0` | `…/GBC-<core>/Solastra.gbc.st5` | `…/GBC-<core>/Solastra.gbc.st9` |
| 5 | 0 | `""` | (unrecognised ROM dir) | `""` (nil return) | — | — |
| 6 | 0 | `""` | `Pokemon - Red Version (USA, Europe).zip` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).zip.st0` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).zip.st5` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).zip.st9` |
| 7 | 0 | `"Pokemon - Red Version (USA, Europe).gb"` | `Pokemon - Red Version (USA, Europe).zip` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).gb.st0` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).gb.st5` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).gb.st9` |
| 8 | 1 | `""` | `Pokemon - Red Version (USA, Europe).zip` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.1` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.5` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.auto` |
| 9 | 1 | `"Pokemon - Red Version (USA, Europe).gb"` | `Pokemon - Red Version (USA, Europe).zip` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.1` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.5` | `…/GB-<core>/Pokemon - Red Version (USA, Europe).state.auto` *(same as #8)* |

`<core>` expands to `gambatte` for GB/GBC and `gpsp` for GBA (see §6.5 core-name table).

---

### C. Migration — `internal/inventory/migrate_test.go`

Tests use a temp directory as the ROM root and a `stubCallback` that returns configurable
answers for each prompt. `MigrateFormats{0, 0}` (both MinUI defaults) is used unless a
test is specifically exercising format-aware behaviour.

#### Enabling unified naming

| # | Scenario | Save at old path? | Save at new path? | States at old path? | cb.AskRename | cb.AskOverwrite | cb.AskRenameStates | Expected result |
|---|----------|:-----------------:|:-----------------:|:-------------------:|:------------:|:---------------:|:------------------:|-----------------|
| 1 | Normal rename, no cover art, no save, no states | — | — | — | — | — | — | ROM renamed; `UnifiedName=true` |
| 2 | Normal rename, cover art present, no save | — | — | — | — | — | — | ROM + cover art renamed |
| 3 | Save exists, user renames it | yes | no | — | `true` | — | — | ROM + cover art + save renamed |
| 4 | Save exists, user skips it | yes | no | — | `false` | — | — | ROM + cover art renamed; save untouched |
| 5 | Save exists at new path, user overwrites | yes | yes | — | `true` | `true` | — | All renamed; old save at new path replaced |
| 6 | Save exists at new path, user cancels | yes | yes | — | `true` | `false` | — | Nothing renamed; inventory unchanged |
| 7 | States exist (2 slots), user renames | — | — | yes | — | — | `true` | ROM + cover art renamed; both state files renamed |
| 8 | States exist, user skips | — | — | yes | — | — | `false` | ROM + cover art renamed; state files untouched; `StatesSkipped` populated |
| 9 | Save + states exist, user renames all | yes | no | yes | `true` | — | `true` | ROM + save + states all renamed |
| 10 | Virtual name == current basename | — | — | — | — | — | — | No-op; `UnifiedName` set to true, no filesystem calls |
| 11 | ROM rename fails (read-only fs) | — | — | — | — | — | — | Error returned; inventory unchanged |
| 12 | Cover art rename fails after ROM rename succeeds | — | — | — | — | — | — | Non-fatal; ROM renamed, cover art not; inventory updated |
| 13 | State rename fails for one slot | — | — | yes | — | — | `true` | Non-fatal; that path in `StatesSkipped`, rest renamed |

> Note on cases 12–13: cover art and state files are display/checkpoint data. Their rename
> failure is logged and reflected in `MigrateResult` flags/slices but does not abort the
> ROM rename or leave the inventory inconsistent.

#### Disabling unified naming

| # | Scenario | Save at current path? | States at current path? | cb.AskRename | cb.AskRenameStates | Expected result |
|---|----------|:---------------------:|:-----------------------:|:------------:|:------------------:|-----------------|
| 14 | Normal revert, no save, no states | — | — | — | — | ROM + cover art renamed back to upstream name |
| 15 | Save exists, user renames | yes | — | `true` | — | ROM + cover art + save renamed back |
| 16 | Save exists, user skips | yes | — | `false` | — | ROM + cover art reverted; save untouched |
| 17 | States exist, user renames | — | yes | — | `true` | ROM + cover art + states reverted |
| 18 | `UnifiedName` already false | — | — | — | — | No-op |

#### Save-format-aware path derivation in migration

| # | saveFormat | useExtractedFileName | ROM basename | Expected old save path checked |
|---|:---:|:---:|---|---|
| 19 | 0 | false | `Doomslinger Dungeon.gb` | `Saves/GB/Doomslinger Dungeon.gb.sav` |
| 20 | 1 | false | `Doomslinger Dungeon.gb` | `Saves/GB/Doomslinger Dungeon.srm` |
| 21 | 2 | false | `Doomslinger Dungeon.gb` | `Saves/GB/Doomslinger Dungeon.sav` |
| 22 | 0 | false | `Pokemon - Red Version (USA, Europe).zip` | `Saves/GB/Pokemon - Red Version (USA, Europe).zip.sav` |
| 23 | 0 | true | `Pokemon - Red Version (USA, Europe).zip` (inner: `.gb`) | `Saves/GB/Pokemon - Red Version (USA, Europe).gb.sav`; old path == new path → **save migration skipped** |
| 24 | 1 | true | `Pokemon - Red Version (USA, Europe).zip` (inner: `.gb`) | `Saves/GB/Pokemon - Red Version (USA, Europe).srm` *(same as useExtractedFileName=false)* |

#### Name collision guard

| # | Scenario | Expected result |
|---|----------|-----------------|
| 22 | Virtual name `"Doomslinger Dungeon.gb"` not on disk | Renamed to `"Doomslinger Dungeon.gb"` |
| 23 | `"Doomslinger Dungeon.gb"` already exists (different file) | Renamed to `"Doomslinger Dungeon (2).gb"` |

---

### D. Update detection — `internal/inventory/inventory_test.go` additions

| # | Scenario | `DownloadedFile.Filename` | `DownloadedFile.DestPath` basename | Upstream files | Expected `HasPendingUpdates` |
|---|----------|--------------------------|-------------------------------------|----------------|------------------------------|
| 16 | Unified name active, same upstream file | `"Game Boy ROM.gb"` | `"Doomslinger Dungeon.gb"` | `["Game Boy ROM.gb"]` | `false` |
| 17 | Unified name active, new upstream version | `"Game Boy ROM.gb"` | `"Doomslinger Dungeon.gb"` | `["Game Boy ROM.gb", "Game Boy ROM v2.gb"]` | `true` |
| 18 | No unified name, upstream unchanged | `"Solastra v1.2.gbc"` | `"Solastra v1.2.gbc"` | `["Solastra v1.2.gbc"]` | `false` |
| 19 | No unified name, new upstream file | `"Solastra v1.2.gbc"` | `"Solastra v1.2.gbc"` | `["Solastra v1.2.gbc", "Solastra v1.3.gbc"]` | `true` |

---

### E. Settings backward compatibility — `internal/settings/settings_test.go` additions

| # | JSON input | Expected `UnifiedNaming` |
|---|------------|--------------------------|
| 20 | `{}` (old config, field absent) | `true` (default) |
| 21 | `{"unified_naming": false}` | `false` |
| 22 | `{"unified_naming": true}` | `true` |
| 23 | Round-trip: load false, save, reload | `false` |
| 24 | Round-trip: load true, save, reload | `true` |

---

### F. Game-specific download scenarios — `internal/itchio/download_test.go` additions

These tests use the fixtures in `testdata/doomslinger-dungeon/` and `testdata/solastra/`.

#### Doomslinger Dungeon (vague filenames)

| # | Scenario |
|---|----------|
| 25 | `FetchUploads` returns three uploads; `.pocket` upload has `NeedsFormat=true`; `.gb` uploads have `NeedsFormat=true` |
| 26 | After format pick `.gb`, unified naming ON → `DownloadedFile.Filename="Game Boy ROM.gb"`, `DestPath` ends with `"Doomslinger Dungeon.gb"`, `UnifiedName=true` |
| 27 | After format pick `.gb`, unified naming OFF (per-game disabled) → `DestPath` ends with `"Game Boy ROM.gb"`, `UnifiedName=false` |
| 28 | Download both `.gb` uploads (unified naming ON) → first gets `"Doomslinger Dungeon.gb"`, second gets `"Doomslinger Dungeon (2).gb"` |

#### Solastra (proper filename)

| # | Scenario |
|---|----------|
| 29 | `FetchUploads` returns upload with a proper filename and extension; `NeedsFormat=false` |
| 30 | Unified naming ON, upstream filename already differs from game title → renamed to `"Solastra.<ext>"` |
| 31 | Unified naming ON, upstream filename == sanitised game title → no rename, `UnifiedName=true` still recorded |
| 32 | Update: new upstream version detected; `HasPendingUpdates` returns true using stored `Filename` |

---

### G. CoverArtPath consistency — `internal/inventory/inventory_test.go` additions

| # | Scenario | Expected cover art path |
|---|----------|------------------------|
| 33 | `DestPath = ".../Game Boy (GB)/Doomslinger Dungeon.gb"` | `.../Game Boy (GB)/.media/Doomslinger Dungeon.png` |
| 34 | `DestPath = ".../Game Boy (GB)/Game Boy ROM.gb"` (unified off) | `.../Game Boy (GB)/.media/Game Boy ROM.png` |
| 35 | After migration back: `DestPath` reverts → cover art path reverts to match |

---

### Extensibility

To add a future game as a test case:
1. Add a directory under `testdata/<game-slug>/` with captured `download_page.html` and
   `uploads.json`.
2. Add rows to the relevant tables in §F referencing the new fixture.
3. No changes needed to the migration, sanitisation, or save-game test tables — those are
   game-agnostic.

---

## 10. Invariants & Edge Cases

- `DownloadedFile.Filename` is **write-once** at download time. Nothing modifies it after.
- `HasPendingUpdates`, `CoverArtPath`, and `VerifyAndClean` require no changes.
- A game with multiple downloaded files (e.g. both `.gb` and `.gbc`) migrates each file
  independently; each gets its own save-game and save-state check.
- If two files in the same game entry would produce the same virtual filename (same title,
  same extension), the collision guard appends ` (2)` on the second.
- The global setting change does **not** retroactively rename existing downloads. The note
  in the Settings screen communicates this.
- **Save detection mirrors NextUI's own behaviour.** The Pak probes only the path that
  matches the *current* `saveFormat` / `stateFormat`. If the user changed those settings
  after creating saves, pre-existing saves at old-format paths are invisible to both NextUI
  and the Pak. The Pak will not prompt about them and will not attempt to rename them.
  This is a documented limitation, not a bug.
- `useExtractedFileName` only affects `SaveGamePath` and `SaveStatePaths` output for
  `saveFormat=0` / `stateFormat=0` (MinUI). For all other formats, extension stripping
  produces the same stem whether the input is a `.zip` or its inner extension.
- When `saveFormat=0` + `useExtractedFileName=true` + ROM is a `.zip`: save and state paths
  are anchored to the inner filename, which does not change when the zip is renamed.
  `MigrateFile` detects old-path == new-path and skips the prompt and rename for those files.
- Save-format and state-format are read from `minuisettings.txt` once per migration call.
  If the user changes their format setting concurrently (unlikely on a handheld), the migration
  uses whichever value was read at start. No re-read mid-migration.
- State files are in `/mnt/SDCARD/.userdata/shared/<TAG>-<core>/`, **not** under `/Saves/`.
  SRAM saves and save states are always in separate directories and handled independently.
- `SaveStatePaths` returns all 10 slot paths regardless of whether they exist. The caller
  filters to existing files before deciding whether to prompt the user. A game with no state
  files triggers no state prompt.
- The `coreName` mapping (§6.5) must be confirmed for each supported system before
  implementing `SaveStatePaths`. An unrecognised system returns nil (no state paths), which
  is safe — the migration proceeds without touching states for that system.

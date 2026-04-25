# Extensionless ROM Format Picker

**Date:** 2026-04-25
**Status:** Approved

## Problem

Some itch.io developers upload GB/GBC ROMs without a file extension, or with version-number suffixes that look like extensions (e.g. "Glory Hunters 2.0", "Glory Hunters 1.3 (ROM)"). The current upload filter accepts only `.gb` and `.gbc`, so these files are silently dropped. The user sees "no downloadable files found" even though the game clearly has uploads.

Real-world case: `https://2think.itch.io/glory-hunters` — three uploads, none with a `.gb`/`.gbc` extension:
- "Glory Hunters 2.0" (ROM, no ext)
- "Glory Hunters 1.3 (ROM)" (ROM, no ext)
- "Digital Deluxe Content" (mixed — ROMs + artwork, likely a ZIP)

## Goal

When a game has no recognized `.gb`/`.gbc` uploads but does have files with unrecognized extensions, present those files to the user and let them choose:

1. Which file to download
2. Whether to treat it as **GB**, **GBC**, or **ZIP**

Append the chosen extension to the filename before saving so all downstream logic (destination routing, cover-art, `LastROMDirs` memory) works without modification.

## Non-Goals

- Do not show the format picker when the game already has `.gb`/`.gbc` uploads. It is the developer's responsibility to label files correctly.
- Do not unzip downloaded ZIP files.
- Do not support bulk download of multiple unknown-format files in one session.

## Design

### 1. `isSkippableExt` helper

**Location:** `internal/itchio/download.go`

A package-level `map[string]bool` listing extensions that are always silently skipped when scanning a game's upload list. Anything **not** in this map (including empty string, version-number suffixes like `.0`, and `.zip`) surfaces as `NeedsFormat: true`.

Skippable set:
- Archives: `.7z` `.tar` `.gz` `.rar` `.bz2`
- Images: `.png` `.jpg` `.jpeg` `.gif` `.bmp` `.webp`
- Audio: `.mp3` `.ogg` `.wav` `.flac` `.aac`
- Documents: `.pdf` `.txt` `.md` `.epub` `.mobi`
- Video: `.mp4` `.avi` `.mkv` `.mov`
- Executables: `.exe` `.dmg` `.apk`
- Other handheld ROM formats: `.pocket` `.nes` `.gba` `.nds` `.sfc` `.smc`

`.zip` is **not** skippable — ZIP uploads are presented to the user so they can decide where to save them.

### 2. `NeedsFormat bool` field

Added to both `itchio.Upload` and `roms.Upload`. When `true`, the file has no recognized ROM extension and the user must choose GB / GBC / ZIP before downloading.

### 3. Updated fetch functions

Both `FetchAuthUploads` (`download_auth.go`) and `ParseDownloadPage` (`game.go`) apply the same three-way classification per upload:

| Extension | Result |
|-----------|--------|
| `.gb` / `.gbc` | Normal upload, `NeedsFormat: false` |
| Skippable ext | Dropped, logged at Debug level |
| Anything else | `NeedsFormat: true` |

### 4. `roms.DestinationDir` — ZIP case

Add `.zip` → `/mnt/SDCARD/Roms/Game Boy Color (GBC)/`

This makes the GBC ROMs folder the auto-mode destination for ZIP saves, and the fallback start directory in the location picker when no `LastROMDirs[".zip"]` is remembered.

### 5. `screen_fetch_uploads.go` routing

The goroutine copies the `NeedsFormat` flag when building each `roms.Upload`. After a successful fetch, `nextScreen()` splits uploads into `known` (`.gb`/`.gbc`) and `unknown` (`NeedsFormat: true`):

| known | unknown | Action |
|-------|---------|--------|
| > 0   | any     | Existing flow (DownloadScreen or ROMPickerScreen on `known`) |
| 0     | > 0     | `FormatPickerScreen` |
| 0     | 0       | Error: "no downloadable files found for this game" |

### 6. `screen_format_picker.go` (new)

Displays the list of unknown-format uploads. Each row shows the filename left-aligned and a format badge right-aligned.

**Controls:**

| Button | Action |
|--------|--------|
| ↑ / ↓ | Navigate rows |
| ◄ / ► | Cycle format on selected row: GB → GBC → ZIP → GB |
| B | Confirm selection and proceed |
| A | Back |

**Default format per file:**
- Original extension already `.zip` → default to ZIP
- All other cases → default to GBC

**Confirm logic (`confirm()`):**

```
chosenExt = ".gb" | ".gbc" | ".zip"
originalExt = filepath.Ext(original filename)
if originalExt == chosenExt:
    upload.Filename = original filename   // already correct, no double-append
else:
    upload.Filename = original filename + chosenExt
```

Then:
- `ROMLocation == "ask"` → `NewLocationPickerScreen(...)`
- `ROMLocation == "auto"` → `NewDownloadScreen(..., dest = DestinationDir(chosenExt) + upload.Filename)`

The `LocationPickerScreen` derives its `LastROMDirs` key from `filepath.Ext(upload.Filename)`, so `.zip` saves automatically land in `LastROMDirs[".zip"]` and start in the GBC folder when no prior save is remembered.

## Files Changed

| File | Change |
|------|--------|
| `internal/itchio/game.go` | Add `NeedsFormat bool` to `Upload` struct |
| `internal/itchio/download.go` | Add `isSkippableExt`; update `ParseDownloadPage` three-way filter |
| `internal/itchio/download_auth.go` | Update `FetchAuthUploads` three-way filter |
| `internal/roms/roms.go` | Add `NeedsFormat bool` to `Upload`; add `.zip` case to `DestinationDir` |
| `internal/ui/screen_fetch_uploads.go` | Copy `NeedsFormat` flag; update `nextScreen()` routing |
| `internal/ui/screen_format_picker.go` | **New** — format picker screen |

## Testing

- Extend `TestFetchAuthUploads` with a mock that returns one `.gbc`, one unknown-ext file, and one `.pdf`. Assert: one known upload, one `NeedsFormat: true` upload, `.pdf` dropped.
- Add `TestParseDownloadPage_UnknownExt` with a download page HTML containing an unknown-ext upload. Assert `NeedsFormat: true`.
- Existing tests must continue to pass without modification.

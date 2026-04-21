# ROM Location Feature — Design Spec

**Date:** 2026-04-21
**Status:** Approved

## Overview

Add a `ROM Location` setting that lets power users choose where downloaded ROM files are saved, instead of always using the hardcoded default directory. When set to `ask`, a directory browser is shown before each download. The browser remembers the last confirmed path per file extension and validates it on open.

---

## 1. Settings Changes

### New fields on `Config` (`internal/settings/settings.go`)

```go
type Config struct {
    APIKey       string            `json:"api_key"`
    ROMSelection string            `json:"rom_selection"`
    ROMLocation  string            `json:"rom_location"`                    // "auto" | "ask"
    LastROMDirs  map[string]string `json:"last_rom_dirs,omitempty"`         // ".gbc" → "/path/to/dir/"
    Filter       ContentFilter     `json:"content_filter"`
}
```

- `ROMLocation` defaults to `"auto"` (added to `defaults()`).
- `LastROMDirs` is `nil` until the user confirms a location for the first time; omitted from JSON when empty.
- Keys are lowercase file extensions as returned by `strings.ToLower(filepath.Ext(filename))` — e.g. `".gbc"`, `".gb"`.

### Settings screen (`internal/ui/screen_settings.go`)

A new `sItemROMLocation` enum value is inserted after `sItemROMMode`, shifting subsequent item indices. The row displays `ROM Location: auto` or `ROM Location: ask`. Activating it toggles between the two values and saves config, identical to the existing `sItemROMMode` pattern.

---

## 2. Download Flow

```
Detail screen
    → FetchUploadsScreen
    → ROMPickerScreen          (only when ROMSelection == "ask")
    → LocationPickerScreen     (NEW — only when ROMLocation == "ask")
    → DownloadScreen
```

The `LocationPickerScreen` is injected at two points:

- **`screen_rom_picker.go`** — after the user selects a file, instead of calling `NewDownloadScreen` directly, check `cfg.ROMLocation`. If `"ask"`, call `NewLocationPickerScreen`; otherwise call `NewDownloadScreen` as today.
- **`screen_fetch_uploads.go`** — the single-upload auto-selection path applies the same check.

`DownloadScreen` is unchanged. It receives a fully resolved `dest` string (directory + filename) and does not need to know how it was chosen.

In `auto` mode the entire feature is a no-op. Existing behaviour is preserved exactly.

---

## 3. LocationPicker Screen (`internal/ui/screen_location_picker.go`)

### Struct

```go
type LocationPickerScreen struct {
    client   *itchio.Client
    cfg      *settings.Config
    cfgPath  string
    cache    *renderer.ImageCache
    game     itchio.Game
    detail   *itchio.GameDetail
    upload   roms.Upload
    prev     Screen

    currentDir string   // directory currently displayed
    entries    []string // sorted subdirectory names in currentDir
    cursor     int      // 0 = "Save here", 1 = "..", 2+ = entries
}
```

### Initialisation

1. Determine `ext = strings.ToLower(filepath.Ext(upload.Filename))`.
2. Look up `cfg.LastROMDirs[ext]`.
3. If a remembered path exists, call `os.Stat` on it.
   - If the path exists on disk: use it as `currentDir`.
   - If the path does not exist: delete `cfg.LastROMDirs[ext]`, call `cfg.Save(cfgPath)`, and use `roms.DestinationDir(ext)` as `currentDir`.
4. If no remembered path: use `roms.DestinationDir(ext)` as `currentDir`.
5. Read `currentDir` with `os.ReadDir`, filter to directories only, sort alphabetically → `entries`.
6. Set `cursor = 0` (pointing at "Save here").

### Layout

```
┌─ Header bar ──────────────────────────────────────────┐
│ <game title>                                          │
│ by <author>                          (small, dimmed)  │
├─ Path bar ────────────────────────────────────────────┤
│ /mnt/SDCARD/Roms/Game Boy Color (GBC)/                │
├─ Confirm row (always first) ──────────────────────────┤
│ [ ✓  Save here ]                                      │
├─ Directory list ───────────────────────────────────────┤
│   ↑  .. (go up)        ← dimmed + inactive at root   │
│ ▸ Action                                              │
│ ▸ RPG                  ← highlighted when selected    │
│   Sports                                              │
│   (no subfolders)      ← italic, dimmed, when empty   │
├─ Footer bar ──────────────────────────────────────────┤
│ B: confirm / enter dir  ·  A: go up  ·  Start: cancel │
│   (footer adapts — see button mapping below)          │
└───────────────────────────────────────────────────────┘
```

The path bar always shows the full `currentDir`. If the path is wider than the screen, it is truncated on the left (showing the rightmost portion) using the existing `truncateToWidth` helper.

The confirm row is drawn with a distinct background (dark green tint) separate from the directory list. When the cursor is on it, it receives the highlight colour. When the cursor is on a directory entry, the confirm row uses its base colour.

### Button mapping

| Button | Cursor on "Save here" | Cursor on ".." | Cursor on a directory |
|--------|-----------------------|----------------|-----------------------|
| D-pad ↑/↓ | Move cursor | Move cursor | Move cursor |
| B | Confirm (see below) | Go up one level | Enter that directory |
| A | Go up one level (if not at root) | Go up one level (if not at root) | Go up one level (if not at root) |
| Start | Cancel — return to `prev`, no download | Cancel | Cancel |

**Root clamp:** Navigation stops at `/mnt/SDCARD/`. At root, the `".."` row is rendered dimmed and does not respond to B. The `A` button hint is removed from the footer.

**Empty directory:** When `entries` is empty after reading, the list shows a single dimmed italic `(no subfolders)` placeholder. The cursor cannot land on it; it resets to 0 ("Save here").

**Footer hint adapts:**
- At root: `B: confirm / enter dir  ·  Start: cancel`
- Cursor on "Save here": `B: confirm  ·  A: go up  ·  Start: cancel`
- All other: `B: confirm / enter dir  ·  A: go up  ·  Start: cancel`

### On confirm

1. Save `currentDir` to `cfg.LastROMDirs[ext]` (creating the map if nil).
2. Call `cfg.Save(cfgPath)`.
3. Return `NewDownloadScreen(client, cfg, game, detail, upload, prev)` with `dest = currentDir + upload.Filename`.

`currentDir` must always end with a `/` (ensured when navigating and when loading from `LastROMDirs`), consistent with the trailing-slash convention used by `roms.DestinationDir`.

### Directory reading

Directories are read with `os.ReadDir(currentDir)`. Only entries where `entry.IsDir()` is true and the name does not start with `.` are included. Results are sorted case-insensitively. Read errors (permission denied, etc.) leave `entries` empty — the screen degrades gracefully to the empty-directory state.

---

## 4. Memory Behaviour

| Situation | Browser opens at | Effect on stored memory |
|-----------|-----------------|------------------------|
| First use — no entry for this extension | `roms.DestinationDir(ext)` | Nothing stored yet |
| Remembered path exists on disk | Remembered path | Unchanged until new confirmation |
| Remembered path no longer exists | `roms.DestinationDir(ext)` | Stale entry deleted; config saved immediately |
| User confirms a location | — | New path written to `LastROMDirs[ext]`; config saved |
| User cancels with Start | — | No change; no download |
| `.gb` vs `.gbc` (different extensions) | Each uses its own remembered path or default | Stored independently |

---

## 5. Files Touched

| File | Change |
|------|--------|
| `internal/settings/settings.go` | Add `ROMLocation string` and `LastROMDirs map[string]string`; default `ROMLocation = "auto"` |
| `internal/ui/screen_settings.go` | Add `sItemROMLocation`; render row; toggle on activate; update `sItemCount` |
| `internal/ui/screen_location_picker.go` | **New file.** Full directory browser screen |
| `internal/ui/screen_rom_picker.go` | Route to `LocationPickerScreen` instead of `DownloadScreen` when `cfg.ROMLocation == "ask"` |
| `internal/ui/screen_fetch_uploads.go` | Same routing change for the single-upload auto-selection path |

No changes to `internal/roms/roms.go`, `internal/ui/screen_download.go`, or any other file.

---

## 6. Out of Scope

- Creating new directories from within the browser (user is expected to set up their folder structure in advance).
- Text entry / on-screen keyboard.
- Navigating above `/mnt/SDCARD/`.
- Any change to auto mode behaviour.

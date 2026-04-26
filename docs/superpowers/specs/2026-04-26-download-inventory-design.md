# Download Inventory — Design Spec

**Date:** 2026-04-26  
**Status:** Approved

## Overview

Track games the user has downloaded so the UI can show which games are already on the device, allow per-file or full-game deletion, and clean up stale entries at startup.

---

## Data Model

**Package:** `internal/inventory`  
**File:** `internal/inventory/inventory.go`  
**Persistence:** `$HOME/inventory.json` (same directory as `config.json` and `games_cache.json`)

```
Inventory
└── Entries  map[string]*Entry   // keyed by game URL

Entry
├── GameURL      string          // primary key (game URL is unique and available in RSS feed)
├── Title        string          // display name
├── Author       string
├── CoverURL     string          // used to derive .media/ art path on delete
├── Files        []DownloadedFile
└── VerifiedAt   time.Time       // last time on-disk presence was confirmed

DownloadedFile
├── Filename     string
├── DestPath     string          // absolute path to the ROM on device
└── DownloadedAt time.Time
```

**Note:** The itch.io numeric GameID is not used as the primary key because it requires scraping the full game detail page and is not present in the RSS feed that drives the list screen. The game URL is unique per game and available without any additional network call.

### Key Methods

| Method | Behaviour |
|--------|-----------|
| `Load(path) (*Inventory, error)` | Reads JSON; returns empty inventory (no error) if file missing |
| `(*Inventory) Save(path) error` | Atomic write via tmp + rename, same pattern as `SaveGamesCache` |
| `(*Inventory) Add(url string, entry Entry, file DownloadedFile)` | Upserts entry, appends file; deduplicates by `DestPath` |
| `(*Inventory) Remove(url string)` | Deletes the full entry |
| `(*Inventory) Lookup(url string) (*Entry, bool)` | O(1) map lookup |
| `(*Inventory) IsPresent(url string) bool` | True if entry exists AND at least one `DestPath` exists on disk |
| `(*Inventory) VerifyAndClean(path string) int` | Walks all entries, removes missing files, removes entries with no files remaining, saves; returns count of removed entries |

---

## Startup Integration

**File:** `cmd/itchio-pak/main_sdl.go`

```go
inventoryPath := filepath.Join(filepath.Dir(cfgPath), "inventory.json")
inv, _ := inventory.Load(inventoryPath)
removed := inv.VerifyAndClean(inventoryPath)
// logger.Info:  "inventory: cleaned %d stale entries", removed
// logger.Debug: one line per removed file/entry
```

`*Inventory` and `inventoryPath` are then passed into `NewListScreen` alongside the existing `cfg` and `cachePath` arguments.

---

## Download Recording

**File:** `internal/ui/screen_download.go`

On successful download (after `DownloadCoverArt` succeeds), `DownloadScreen` calls:

```go
inv.Add(game.URL, inventory.Entry{
    GameURL:  game.URL,
    Title:    game.Title,
    Author:   game.Author,
    CoverURL: game.CoverURL,
}, inventory.DownloadedFile{
    Filename:     upload.Filename,
    DestPath:     dest,
    DownloadedAt: time.Now(),
})
inv.Save(inventoryPath)
```

`DownloadScreen` gains `inv *inventory.Inventory` and `inventoryPath string` constructor arguments. These thread through the screen chain: `NewListScreen` → `NewDetailScreen` → `NewFetchUploadsScreen` → `NewDownloadScreen`, consistent with how `cfgPath` already threads through.

---

## List Screen Visual Changes

**File:** `internal/ui/screen_list.go`

For each row, `inv.IsPresent(g.URL)` is checked (O(1) map lookup — no I/O per frame):

- **If present:** render the floppy disk icon `🖾` (U+1F4BE) in the price badge position, in soft cyan (`80, 200, 220`); render the game title in bold.
- **Otherwise:** unchanged (price badge as today).

### Floppy disk glyph

`sanitizeText` in `renderer/text.go` currently strips U+1F4BE (falls in `0x1F300–0x1F5FF`). A targeted exemption for exactly U+1F4BE is added to `isEmoji` so it passes through. If the bundled font cannot render it (tofu box on-device), fall back to `↓` (U+2193, safely below all stripped ranges).

### Bold title rendering

`renderer.Renderer` gains a `DrawBoldText(text string, x, y int32, r, g, b uint8)` method:

1. `font.SetStyle(ttf.STYLE_BOLD)`
2. Draw text
3. `font.SetStyle(ttf.STYLE_NORMAL)`

SDL_ttf synthesizes bold even for fonts without a dedicated bold variant.

---

## Detail Screen Changes

**File:** `internal/ui/screen_detail.go`

### Action area (when game is in inventory)

```
[ A: Download again ]   🖾 Already on device
  my-game.gb  →  /mnt/SDCARD/Roms/Game Boy (GB)/
```

The floppy + "Already on device" text is rendered in soft cyan. Each tracked file is listed below in small text.

### Delete button

A `[ X: Delete ]` button appears in the action area whenever the game is in the inventory (whether or not files are still present). On device, X maps to `sdl.CONTROLLER_BUTTON_X`.

### Modal extension

`detailModal` gains a `kind` field (`kindInfo` / `kindDeleteConfirm`) so `HandleEvent` can distinguish "dismiss on any button" from "A confirms, B cancels".

Footer hint for confirm modals: `"A: confirm  B: cancel"`.

---

## Delete Flow

### Single file

Pressing X → confirmation modal:

- Title: `"Delete downloaded file?"`
- Body: filename + destination path
- Footer: `"A: confirm  B: cancel"`

On confirm:
1. `os.Remove(destPath)` — log each at debug
2. Derive and delete cover art: `filepath.Join(filepath.Dir(destPath), ".media", baseName+artExt)`
3. `inv.Remove(gameURL)`, `inv.Save(inventoryPath)`
4. Dismiss modal; detail screen refreshes to standard `[ A: Download ]`

Log at info: `"inventory: deleted game=%q files=1"`

### Multiple files — ManageDownloadsScreen

**File:** `internal/ui/screen_manage_downloads.go`

Pressing X navigates to a new `ManageDownloadsScreen` (consistent with existing picker screen pattern). It receives `*inventory.Inventory`, `inventoryPath`, the game's `*inventory.Entry`, and `prev Screen`. No network dependency.

```
  Manage Downloads — Game Title
  ────────────────────────────────
  ▶  my-game-v1.gb    /mnt/SDCARD/Roms/Game Boy (GB)/
     my-game-v2.gbc   /mnt/SDCARD/Roms/Game Boy Color (GBC)/
  ────────────────────────────────
     Delete all
```

- D-pad navigates rows
- A on a file row → single-file confirmation modal (same flow as above, but leaves the entry in inventory if other files remain)
- A on "Delete all" → confirmation modal listing all files; on confirm deletes all files, cover art, and the full inventory entry
- After the last file is deleted the screen pops back to the detail screen, which now shows `[ A: Download ]`
- B exits back to the detail screen without changes

Footer: `"A: select/delete  B: back"`

---

## Cover Art Deletion

Cover art path is derived from the ROM `DestPath` using the same logic as `DownloadCoverArt` in reverse:

```go
dir     := filepath.Dir(destPath)
base    := strings.TrimSuffix(filepath.Base(destPath), filepath.Ext(destPath))
artExt  := filepath.Ext(parsed.Path of CoverURL)  // from Entry.CoverURL
artPath := filepath.Join(dir, ".media", base+artExt)
os.Remove(artPath)
```

If `CoverURL` is empty or parsing fails, fall back to checking `.media/<base>.*` with common extensions (`.png`, `.jpg`, `.jpeg`).

---

## Files Changed / Created

| File | Change |
|------|--------|
| `internal/inventory/inventory.go` | New package |
| `internal/inventory/inventory_test.go` | New unit tests |
| `internal/renderer/text.go` | Exempt U+1F4BE from `isEmoji` |
| `internal/renderer/renderer.go` | Add `DrawBoldText` method |
| `cmd/itchio-pak/main_sdl.go` | Load inventory, run VerifyAndClean, pass to ListScreen |
| `internal/ui/screen_list.go` | Floppy icon + bold title for present games |
| `internal/ui/screen_detail.go` | Download status display, X button, modal kind field, delete flow |
| `internal/ui/screen_manage_downloads.go` | New screen for multi-file deletion |
| `internal/ui/screen_download.go` | Record download to inventory on success |
| `internal/ui/screen_fetch_uploads.go` | Add `inv` and `inventoryPath` to constructor; pass through to `NewDownloadScreen` |
| `internal/ui/screen_rom_picker.go` | Thread `inv` and `inventoryPath` through to `NewDownloadScreen` |
| `internal/ui/screen_location_picker.go` | Thread `inv` and `inventoryPath` through to `NewDownloadScreen` |
| `internal/ui/screen_format_picker.go` | Thread `inv` and `inventoryPath` through to `NewDownloadScreen` |

---

## Logging Standards

| Level | Message |
|-------|---------|
| Info | `"inventory: cleaned %d stale entries"` |
| Debug | `"inventory: removing stale file=%s"`, `"inventory: removing empty entry game=%q"` |
| Info | `"inventory: recorded game=%q file=%s"` |
| Info | `"inventory: deleted game=%q files=%d"` |
| Debug | `"inventory: deleted file=%s"`, `"inventory: deleted cover-art=%s"` |

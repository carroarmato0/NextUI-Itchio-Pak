# Inventory Update Tracking — Design Spec

**Date:** 2026-04-28  
**Status:** Approved

## Overview

Extend the inventory system to detect when new ROM files become available upstream for downloaded games, repair missing cover art in the background, and surface update status in the UI. The feature runs asynchronously at startup and on manual trigger, never blocking the main UI loop.

---

## 1. Data Model

### 1.1 New type: `UpstreamFile`

```go
type UpstreamFile struct {
    Filename string    `json:"filename"`
    UploadID string    `json:"upload_id"`
    SeenAt   time.Time `json:"seen_at"` // when this file was last confirmed upstream
}
```

### 1.2 Extended `Entry` fields

Six new fields added to the existing `Entry` struct (all `omitempty`):

```go
IsFree             bool           `json:"is_free,omitempty"`
KnownUpstreamFiles []UpstreamFile `json:"known_upstream_files,omitempty"`
UpdateCheckedAt    time.Time      `json:"update_checked_at,omitempty"`
UpdateDismissedAt  time.Time      `json:"update_dismissed_at,omitempty"`
GameRemovedAt      time.Time      `json:"game_removed_at,omitempty"`
RemovalDismissedAt time.Time      `json:"removal_dismissed_at,omitempty"`
```

`IsFree` is set when the entry is first created in `inv.Add()` from `game.IsFree`. It controls whether file-diff is attempted during update checks (free games only — paid games have no lightweight public file-listing API).

All existing inventory data is forward-compatible; old entries simply have zero values for these fields.

### 1.3 New `*Inventory` methods

**`HasPendingUpdates(gameURL string) bool`**  
Returns true when any `UpstreamFile` in the entry satisfies both:
- `Filename` is not present in `Files[*].Filename` (not yet downloaded)
- `SeenAt` is after `UpdateDismissedAt` (not dismissed)

**`IsRemoved(gameURL string) bool`**  
Returns true when:
```
!GameRemovedAt.IsZero() &&
(RemovalDismissedAt.IsZero() || GameRemovedAt.After(RemovalDismissedAt))
```

**`DismissUpdate(gameURL string)`**  
Sets `UpdateDismissedAt = time.Now()`. Game drops back into normal `[DL]` sort position on next `rebuildView()`.

**`DismissRemoval(gameURL string)`**  
Sets `RemovalDismissedAt = time.Now()`. Badge hidden until `GameRemovedAt` is set fresh (only possible if the game reappears then goes 404 again).

### 1.4 Removal detection semantics

`GameRemovedAt` is written **only on the first 404 detection** (`GameRemovedAt.IsZero()` guard). Subsequent checks leave it unchanged if the game is still unreachable. This means `RemovalDismissedAt > GameRemovedAt` persists across checks, suppressing the badge without the warning re-appearing on every run.

When the game becomes reachable again, both `GameRemovedAt` and `RemovalDismissedAt` are cleared, returning the entry to a clean slate.

---

## 2. `UpdateService` (`internal/inventory/updater.go`)

### 2.1 Struct and API

```go
type UpdateService struct {
    inv           *Inventory
    inventoryPath string
    client        *itchio.Client
    triggerCh     chan struct{} // buffered(1): absorbs duplicate triggers
    stopCh        chan struct{}
}

func NewUpdateService(inv *Inventory, inventoryPath string, client *itchio.Client) *UpdateService
func (s *UpdateService) Start()        // launch goroutine; runs check immediately
func (s *UpdateService) Stop()         // signal goroutine to exit; idempotent
func (s *UpdateService) TriggerNow()   // non-blocking; no-op if already running
func (s *UpdateService) IsRunning() bool // true while runCheck() is executing; safe for UI polling
```

### 2.2 Worker loop

```
Start() → goroutine:
    runCheck()
    loop:
        select triggerCh → runCheck()
        select stopCh    → return
```

`TriggerNow()` sends on `triggerCh` without blocking. The buffered channel of size 1 absorbs rapid duplicate calls (e.g. user hammering the Settings button).

### 2.3 `runCheck()` — per inventory entry, sequentially

Two distinct code paths based on `entry.IsFree`.

**Free games** use `client.FetchUploads(gameURL)` as the single network call. It fetches the game page (naturally detects 404) and returns the upload list in one round-trip.

**Paid games** use `client.FetchGameDetail(gameURL)` for 404 detection only. File-diff is skipped — re-fetching a paid download key solely to list files is out of scope.

1. **Cover art repair**: for each `DownloadedFile`, `os.Stat(CoverArtPath(...))`. If missing and `entry.CoverURL != ""`, call `client.DownloadCoverArt(entry.CoverURL, file.DestPath)`. Log info on re-download, debug on hit.

2. **Removed game check + file diff (free games)**: call `client.FetchUploads(gameURL)`.
   - HTTP 404/410 error and `GameRemovedAt.IsZero()` → set `GameRemovedAt = now`, log warn. Skip file diff.
   - HTTP 404/410 error and `GameRemovedAt` already set → skip silently.
   - Other network/HTTP error → log warn, leave all fields unchanged, continue to next entry.
   - Success and `GameRemovedAt` non-zero → clear `GameRemovedAt` and `RemovalDismissedAt`, log info ("game reappeared").
   - Success → proceed to file diff (step 3).

3. **File diff (free games only)**: compare `FetchUploads` result against `KnownUpstreamFiles`.
   - New filenames (not in known set): append `UpstreamFile{Filename, UploadID, SeenAt: now}`.
   - Vanished filenames (in known set but not in scraped list): remove from `KnownUpstreamFiles`.
   - Set `UpdateCheckedAt = now`.

**Removed game check (paid games only)**: call `client.FetchGameDetail(gameURL)`. Apply the same 404/error/reappear logic as above. No file diff. Set `UpdateCheckedAt = now`.

4. **Save**: call `inv.Save(inventoryPath)` after each entry. Partial progress is preserved if the app exits mid-check.

5. **Completion**: after all entries, push `sdl.UserEvent` to wake the render loop for an immediate redraw.

### 2.4 Integration points

- **Startup**: `updateService.Start()` called in `main_sdl.go` after inventory is loaded, alongside `go inv.VerifyAndClean(...)`.
- **Post-cache-refresh**: `ListScreen.newCacheRefreshScreen` callback chain calls `updateService.TriggerNow()` after `rebuildView()` completes.
- **Settings trigger**: `sItemUpdateInventory` activate handler calls `updateService.TriggerNow()`.
- **Shutdown**: `updateService.Stop()` called before SDL teardown.

---

## 3. Sort Logic — `[DL]` Mode

`ApplySort` for `SortModeDL` is extended to float action-needed games to the top:

```
Group 1 — [UP]:  HasPendingUpdates == true
Group 2 — [!]:   IsRemoved == true
Group 3 — [DL]:  all other downloaded games (default/RSS order within group)
```

No new sort mode is added. The `[UPDATES]` sort mode proposed during brainstorming was dropped — the existing `[DL]` filter is the natural home for downloaded-game management.

Separators and section labels are rendered conditionally:
- "— updates available —" label shown only when Group 1 or Group 2 is non-empty.
- "— downloaded —" label shown only when Group 3 is non-empty AND at least one of Group 1/2 is also non-empty (i.e. there is something above it to separate from).

---

## 4. UI Changes

### 4.1 List screen row badges

| Condition | Badge | Colour |
|---|---|---|
| `HasPendingUpdates` | `[UP]` | amber `(240,160,40)` |
| `IsRemoved` | `[!]` | red `(200,60,60)` |
| `IsPresent` (downloaded, up to date) | `[DL]` | cyan `(80,200,220)` |
| Not downloaded | price / `Free` | yellow / green (unchanged) |

### 4.2 Cover art pill badge

Drawn after `DrawTextureAt` so it appears above animated GIF frames:

```
1. DrawRect(x+1, y+1, w, h, shadow colour)   // depth layer
2. DrawRect(x,   y,   w, h, pill colour)      // main pill
3. DrawSmallText(label, x+pad, y+pad, text colour)
```

| State | Pill colour | Shadow colour | Text | Text colour |
|---|---|---|---|---|
| `[UP]` | `(240,160,40)` | `(160,96,16)` | `UPDATE` | `(20,20,20)` |
| `[!]` | `(200,60,60)` | `(122,16,16)` | `REMOVED` | `(255,255,255)` |

Badge position: 5px from top-right corner of the cover art box.

### 4.3 Dismiss from list screen — X button

X button (`CONTROLLER_BUTTON_X`) is handled in `ListScreen.HandleEvent` only when the selected game has `[UP]` or `[!]` state. Action:
- `[UP]`: calls `inv.DismissUpdate(gameURL)`, saves inventory, calls `rebuildView()`.
- `[!]`: calls `inv.DismissRemoval(gameURL)`, saves inventory, calls `rebuildView()`.

Cursor follows the game to its new position in the `[DL]` group after `rebuildView()`.

**Footer hints (contextual — only shown for `[UP]` / `[!]` games):**

| Screen width | `[UP]` hint | `[!]` hint |
|---|---|---|
| Wide (> 640px) | `X:dismiss update` (amber) | `X:dismiss warning` (red) |
| Narrow (≤ 640px) | `X:dismiss` (amber) | `X:dismiss` (red) |

### 4.4 Settings screen

New item `sItemUpdateInventory` inserted between `sItemRefreshCache` and `sItemContentModeration`:

```
"Update Inventory"    [right-aligned: last: Xh ago / checking… / never]
```

- Timestamp derived from the most recent `UpdateCheckedAt` across all inventory entries. Format: `just now` · `5m ago` · `2h ago` · `3d ago` · `never`.
- While running: right side shows `checking…` in amber. The entry remains navigable; the user can leave settings freely.
- `sItemCount` incremented by 1.

### 4.5 Detail screen

No changes. The detail screen already shows download status via the inventory. Update state is surfaced in the list screen; navigating to the detail screen is not required to dismiss.

---

## 5. Logging

| Level | Event |
|---|---|
| `Info` | Cover art re-downloaded for a ROM; update check started/completed; game reappeared after removal; `TriggerNow` fired from settings |
| `Debug` | Cover art stat hit (present, skipping); per-entry scrape result (N files found); file diff (added/pruned filenames) |
| `Warn` | Transient network error for an entry (skipping); first 404 detection for a game |
| `Error` | `inv.Save` failure after entry; `DownloadCoverArt` failure |

---

## 6. False Positives

Zip files that are not ROM files will appear as pending updates when first detected. This is expected and accepted. The dismiss-per-game mechanism (`X:dismiss update`) gives the user a quick way to silence false positives. Future uploads to that game that appear after the dismissal will resurface automatically via the `SeenAt > UpdateDismissedAt` condition.

# Sort & Filter System Design

**Date:** 2026-04-27
**Status:** Approved

## Overview

Add a sort/filter system to the game list screen. The user cycles through modes with the SELECT button. The active mode is shown as a coloured badge in the top-right of the header. The selected mode persists between sessions via `config.json`.

---

## Sort Modes

Seven modes, cycled in order by SELECT (wraps from last back to `[RSS]`):

| Badge | Label | Type | Behaviour |
|-------|-------|------|-----------|
| `[RSS]` | Feed default | Sort | itch.io RSS order — no transform. This is the default/reset state. |
| `[A-Z]` | Alpha ascending | Sort | All games A→Z by title (case-insensitive stable sort). |
| `[Z-A]` | Alpha descending | Sort | All games Z→A by title (case-insensitive stable sort). |
| `[NEW]` | Newest first | Sort | By `PublishedAt` descending. Games with zero/missing date sort to the end. |
| `[DL]` | Downloaded only | Filter | Exclusive — shows only games present in the inventory. |
| `[FREE]` | Free only | Filter | Exclusive — shows only games where `IsFree == true`. |
| `[PAID]` | Paid only | Filter | Exclusive — shows only games where `IsFree == false`. |

**Cycle order:** `[RSS]` → `[A-Z]` → `[Z-A]` → `[NEW]` → `[DL]` → `[FREE]` → `[PAID]` → *(wrap to `[RSS]`)*

Sort is only available when the full game cache is loaded (`cacheReady == true`). In live-feed fallback mode the badge is hidden and SELECT is ignored.

---

## Badge Appearance

Coloured badge, right-aligned in the header bar. Matches the colour of the corresponding row badges already used in the list:

| Modes | Colour | RGB |
|-------|--------|-----|
| `[RSS]`, `[A-Z]`, `[Z-A]`, `[NEW]`, `[DL]` | Teal | `(80, 200, 220)` — same as the `[DL]` row badge |
| `[FREE]` | Green | `(80, 200, 80)` — same as the "Free" row badge |
| `[PAID]` | Gold | `(220, 180, 60)` — same as the price row badge |

The badge is not rendered when `cacheReady == false`.

---

## Empty State

When the active filter produces zero results, the list panel shows:

```
No games match this filter.
Press SELECT to change sort.
```

Centred in the list panel. The right panel (cover art) shows nothing. D-pad navigation, L/R page buttons, and A are disabled. SELECT still works to cycle to the next mode.

---

## Data Model Changes

### `internal/itchio/feed.go`

Add `PublishedAt time.Time` to `Game`:

```go
type Game struct {
    // existing fields ...
    PublishedAt time.Time `json:"published_at,omitempty"`
}
```

Add `PubDate string` to `rssItem` and parse it into `PublishedAt` when building `Game` objects. Accept RFC1123 and RFC1123Z formats (standard RSS date formats). A parse failure produces a zero `time.Time` (sorts to end in `[NEW]` mode).

Existing cache files serialised without `published_at` unmarshal to zero `time.Time` — handled gracefully.

### `internal/settings/settings.go`

Add `SortMode string` to `Config`:

```go
type Config struct {
    // existing fields ...
    SortMode string `json:"sort_mode,omitempty"`
}
```

`defaults()` leaves `SortMode` as `""` (empty string), which the UI maps to `[RSS]`. Old config files without the key unmarshal to `""` — backward compatible with no migration needed.

---

## Sort Logic — `internal/itchio/sort.go` (new file)

```go
type SortMode string

const (
    SortModeRSS  SortMode = ""
    SortModeAZ   SortMode = "az"
    SortModeZA   SortMode = "za"
    SortModeNew  SortMode = "new"
    SortModeDL   SortMode = "dl"
    SortModeFree SortMode = "free"
    SortModePaid SortMode = "paid"
)

// SortModes is the SELECT cycle order.
var SortModes = []SortMode{
    SortModeRSS, SortModeAZ, SortModeZA, SortModeNew,
    SortModeDL, SortModeFree, SortModePaid,
}

// SortModeBadge returns the display string for the header badge.
func SortModeBadge(m SortMode) string { ... } // e.g. "" → "[RSS]", "az" → "[A-Z]"

// ApplySort returns a new slice derived from games according to mode.
// downloaded is a set of game URLs present in the inventory.
// cachedGames is never mutated.
func ApplySort(games []Game, mode SortMode, downloaded map[string]bool) []Game
```

`ApplySort` always returns a new slice. For sort modes it copies then sorts; for filter modes it copies only matching entries.

---

## `ListScreen` Changes

### New fields

```go
sortMode  itchio.SortMode
viewGames []itchio.Game  // sorted/filtered view; paging operates on this
```

### `rebuildView()` (new method)

1. Build `downloaded map[string]bool` by iterating `inv.Entries` and including URLs where `inv.IsPresent(url) == true`.
2. Call `itchio.ApplySort(s.cachedGames, s.sortMode, downloaded)` → store in `s.viewGames`.
3. Recalculate `s.totalGames = len(s.viewGames)` and `s.totalPages`.
4. Reset `s.page = 1`.
5. Call `s.loadPage(1, "")` so `s.games` (the current page slice) is refreshed from `viewGames`.

Called whenever:
- `cachedGames` is set (on init, after `buildCache`, after `refreshCacheIfStale`)
- `sortMode` changes (SELECT press)

### `loadPage` update

When `cacheReady == true`, use `pageSlice(s.viewGames, page)` instead of `pageSlice(s.cachedGames, page)`.

### SELECT handler

```go
case sdl.CONTROLLER_BUTTON_BACK:
    if !s.cacheReady { return s }
    s.sortMode = itchio.NextSortMode(s.sortMode)
    s.rebuildView()
    s.cfg.SortMode = string(s.sortMode)
    go s.cfg.Save(s.cfgPath)
    return s
```

`NextSortMode` advances to the next entry in `SortModes`, wrapping around.

### Init

Load `s.sortMode = itchio.SortMode(cfg.SortMode)` in `NewListScreen`. After loading the cache, call `rebuildView()` (which will apply the persisted sort immediately).

### Header draw

After the title text, spacer, then conditionally render the badge when `cacheReady`:

```go
if s.cacheReady {
    badge := itchio.SortModeBadge(s.sortMode)
    bw, _ := r.TextSize(badge)
    bx := r.W - bw - 12
    // pick colour by mode
    r.DrawText(badge, bx, headerTextY, badgeR, badgeG, badgeB)
}
```

### Empty state draw

In `Draw`, after checking `s.loading` and `s.err`, add:

```go
if len(s.viewGames) == 0 && s.cacheReady {
    r.DrawTextCentered("No games match this filter.", 0, r.H/2 - fontH, leftW, 140, 140, 140)
    r.DrawTextCentered("Press SELECT to change sort.", 0, r.H/2 + 4, leftW, 80, 160, 180)
    // render footer + badge, skip list and cover art
    return
}
```

---

## Footer Hint Update

Add `SELECT:sort` to the footer hint string when `cacheReady == true`:

```
Page 1/14 · 500 games  |  A:select  L/R:page  SELECT:sort  B:exit  Start:settings
```

---

## Button Mapping

SELECT maps to `sdl.CONTROLLER_BUTTON_BACK` (SDL2 standard back/select button, index 4). Not currently used anywhere in the codebase — no conflicts.

---

## Testing

`ApplySort` is a pure function — unit tests in `internal/itchio/sort_test.go` cover:
- Each mode with a small fixed slice
- `[DL]` with empty `downloaded` map → returns empty slice
- `[NEW]` with mixed zero/non-zero `PublishedAt` values
- Cycle order via `NextSortMode`

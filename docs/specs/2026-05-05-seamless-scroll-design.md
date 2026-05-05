# Seamless Scroll — Game List Screen

## Overview

Replace the page-based navigation model on the game list screen with a single
continuous scrollable list. Internal pagination is retained (the itch.io RSS API
returns 36 items per page), but the user never experiences a page boundary.
L1/R1 gain press-and-hold acceleration identical to the existing D-pad behaviour.

## Goals

- Scrolling through any number of games feels like one flat list — no visible
  page transitions.
- L1/R1 single press jumps one visible screen-height worth of items; held
  accelerates using the same ease-out curve as D-pad.
- The "Page X/Y" indicator remains in the footer as a position reference.
- No behaviour changes for the detail screen, settings, sort/filter, or inventory.

## Non-Goals

- Changing the itch.io API client or pagination constants (`PerPage = 36`).
- Adding infinite scroll or lazy loading beyond what the progressive callback
  already provides.
- Fixing the pre-existing lack of a mutex around `viewGames` mutations from
  goroutines (out of scope; behaviour is unchanged from today).

---

## State Model Changes (`ListScreen`)

### Fields removed

| Field | Reason |
|---|---|
| `games []itchio.Game` | Renderer iterates `viewGames` directly |
| `page int` | Derived from cursor; never stored |
| `jumpToEnd bool` | Only needed for backward page transitions |

### Fields changed

| Field | Before | After |
|---|---|---|
| `cursor int` | Page-relative index (0..PerPage-1) | Absolute index into `viewGames` |

### Fields added

| Field | Type | Purpose |
|---|---|---|
| `heldShoulderDir` | `int` | -1 = L1 held, +1 = R1 held, 0 = none |
| `heldShoulderSince` | `time.Time` | When shoulder button was first pressed |
| `lastShoulderRepeat` | `time.Time` | Last time shoulder auto-repeat fired |
| `lastVisibleRows` | `int` | Cached from most recent Draw; used by HandleEvent for jump size |

### Computed values (no longer stored)

```go
page       = s.cursor/itchio.PerPage + 1   // for footer display only
totalPages = s.totalPages                   // unchanged
```

---

## Navigation

### `moveCursor(dir int)`

```
dir > 0:  if cursor < len(viewGames)-1  →  cursor++
dir < 0:  if cursor > 0                 →  cursor--
```

No page loading, no boundary transitions, no `jumpToEnd`. Scroll state resets
(`titleScrollX`, `tagScrollY`, `lastCursorMove`, `warmedGameURL`) are unchanged.

### `jumpCursor(n int)` — new helper

```
step    = n  (if abs(n) == 0, treat as ±1)
newPos  = clamp(cursor + step, 0, len(viewGames)-1)
cursor  = newPos
```

Resets the same scroll state as `moveCursor`. Called by L1/R1 with
`n = ±lastVisibleRows`.

### `pageSlice()` — deleted

No longer used once `s.games` is removed.

---

## L1/R1 Held Acceleration

### New methods

**`startShoulderHold(dir int)`**
- Calls `jumpCursor(dir * lastVisibleRows)` immediately (first press action).
- Sets `heldShoulderDir`, `heldShoulderSince`, `lastShoulderRepeat`.

**`stopShoulderHold(dir int)`**
- Clears `heldShoulderDir` if it matches `dir`.

### `processAutoRepeat()` extension

Adds a second block after the existing D-pad block:

```
if heldShoulderDir != 0:
    elapsed = now - heldShoulderSince
    if elapsed >= repeatDelay and now - lastShoulderRepeat >= currentRepeatInterval(elapsed - repeatDelay):
        jumpCursor(heldShoulderDir * lastVisibleRows)
        lastShoulderRepeat = now
```

Uses the identical `currentRepeatInterval` ease-out curve as D-pad — same
`repeatDelay`, `accelStart`, `accelMin`, `accelRamp` constants.

### `NeedsRedraw()` update

Add `heldShoulderDir != 0` to the existing check so held shoulder buttons
trigger redraws at the same cadence as held D-pad:

```go
func (s *ListScreen) NeedsRedraw() bool {
    if s.heldDir != 0 || s.heldShoulderDir != 0 {
        return true
    }
    ...
}
```

### Footer hint label

`"L1/R1: Page"` → `"L1/R1: Jump"`

---

## Rendering (`Draw`)

### Loop source

```go
// before
for i, g := range s.games { ... }

// after
for i, g := range s.viewGames { ... }
```

### `startIdx` computation

Unchanged formula, now reads absolute `s.cursor`:

```go
if s.cursor >= int(visibleRows) {
    startIdx = s.cursor - int(visibleRows) + 1
}
```

`lastVisibleRows = int(visibleRows)` is written at the top of each `Draw` call.

### `warmPreloadWindow` simplification

```go
// before
absIdx := (s.page-1)*itchio.PerPage + s.cursor

// after
absIdx := s.cursor
```

### DL-mode separator

`dlSepAfterUpdates` scans `s.viewGames` (previously `s.games`). The separator
draw logic `sepRowIdx = dlSepAfterUpdates - startIdx` is unchanged.

### Footer page indicator

```go
page := s.cursor/itchio.PerPage + 1
pageInfo := fmt.Sprintf("Page %d/%d", page, s.totalPages)
```

---

## Progressive Loading (`buildCache`)

### `FetchAllGames` callback signature change

```go
// before
progress func(fetched int)

// after
progress func(partial []Game)
```

All three call sites update accordingly:
- `screen_list.go` — uses `partial` for progressive `rebuildView` (see below).
- `screen_cache_refresh.go` — `atomic.StoreInt64(&s.fetched, int64(len(partial)))`.
- `feed_test.go` — two callers pass `nil`; one passes `func(fetched int)` → update to `func(partial []Game)`.

### `buildCache` progressive callback

```go
s.client.FetchAllGames(ctx, func(partial []Game) {
    logger.Debug("cache: fetched %d games so far", len(partial))
    s.cachedGames = partial
    s.cacheReady = true
    s.rebuildView()
})
```

`rebuildView` preserves the absolute cursor via `selectedURL` lookup, so the
user's position is stable as new pages arrive. On non-RSS sort modes the list
re-sorts on each partial update; this is acceptable since it only occurs on a
fresh first run before the user has changed the sort mode.

---

## `rebuildView` Simplification

Cursor preservation collapses to a direct assignment:

```go
// before
page   = i/itchio.PerPage + 1
cursor = i % itchio.PerPage
s.page   = page
s.games  = pageSlice(s.viewGames, page)
s.cursor = cursor

// after
s.cursor = i
```

The "nearest position" fallback similarly becomes `s.cursor = selectedViewIdx`
with a bounds clamp, and the `s.games = pageSlice(...)` call is removed.

---

## `loadPage` — Live-Fetch Path Only

With cache-ready navigation no longer calling `loadPage`, the cache-ready branch
of that function becomes dead code and is removed. What remains is the live-fetch
path (first run only):

```go
func (s *ListScreen) loadPage(page int, query string) {
    s.loading = true
    s.err = nil
    games, err := s.client.FetchGames(page, query)
    s.viewGames = games   // was s.games
    s.err = err
    s.cursor = 0          // was placeCursor()
    // reset scroll state
    s.loading = false
}
```

`placeCursor()` is deleted (its only caller was `loadPage`).

The simplified `loadPage` resets all scroll state explicitly:

```go
s.cursor = 0
s.titleScrollX = 0
s.titleScrollAt = time.Now()
s.tagScrollY = 0
s.tagScrollAt = time.Now()
s.lastCursorMove = time.Now()
s.warmedGameURL = ""
```

---

## `.gitignore`

Add `.superpowers/` if not already present, to avoid committing brainstorm
session files.

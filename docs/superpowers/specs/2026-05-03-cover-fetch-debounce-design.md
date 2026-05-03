# Cover Fetch Debounce + Preload Window — Design Spec

**Date:** 2026-05-03
**Status:** Approved

## Problem

`cache.Get` is called every Draw frame for the currently-selected game's cover URL.
During fast scrolling the cursor moves to a new game every ~30 ms, so every game
scrolled past launches a background goroutine queued on the semaphore. Those
goroutines linger long after the user has moved on, generating a large number of
HTTP requests for images that will never be displayed.

## Goal

1. Suppress spurious fetches for games scrolled past quickly.
2. Lazily preload cover art for the ±5 games around the current selection so that
   images are already cached when the user navigates to a neighbour naturally.

## Solution Overview

Split fetch initiation from rendering by adding two focused methods to
`ImageCache`, then have the list screen drive fetch timing explicitly via a
cursor-settle delay.

---

## Image Cache API (`internal/renderer/image_cache.go`)

### `Peek(r *Renderer, url string) *sdl.Texture`

Identical to `Get` except it never starts a background fetch. Returns the cached
texture (advancing GIF frames as needed) or `nil` if not cached. Read-only path
used by the list screen right panel for rendering.

### `Warm(url string)`

Schedules a background fetch for `url` if it is not already cached or in-flight.
Returns immediately with no texture. Write-only path used by the list screen to
initiate fetches at the right moment.

### `Get` — unchanged

All existing callers (detail screen, cover-art download, etc.) keep their current
behaviour. No modifications required.

---

## List Screen Changes (`internal/ui/screen_list.go`)

### New constant

```go
coverSettleDelay = 100 * time.Millisecond
```

- Below `accelStart` (180 ms, slowest repeat interval) → settle fires between
  natural key taps, so normal browsing feels instant.
- Well above `accelMin` (30 ms, fastest repeat interval) → fast scrolling keeps
  resetting the timer before it fires, suppressing all intermediate fetches.

### New fields on `ListScreen`

```go
lastCursorMove time.Time // reset whenever the cursor index changes
warmedGameURL  string    // URL of the game that last triggered warming
```

### Cursor tracking

`lastCursorMove = time.Now()` and `warmedGameURL = ""` are set in:

- `moveCursor()` — every D-pad step
- `placeCursor()` — after every page load

Clearing `warmedGameURL` ensures the next settle re-warms even if the cursor
lands on the same index on a different page.

### `warmPreloadWindow()` method

```
absIdx = (s.page − 1) × PerPage + s.cursor
for i in [absIdx−5 .. absIdx+5], clamped to [0, len(s.viewGames)):
    if s.viewGames[i].CoverURL != "":
        s.cache.Warm(s.viewGames[i].CoverURL)
s.warmedGameURL = s.games[s.cursor].CoverURL
```

Warms at most 11 URLs (current game + 5 on each side). Handles page boundaries
correctly by indexing into `viewGames` rather than the page slice.

### Settle check in `Draw()`

Added before the right-panel render block:

```go
if time.Since(s.lastCursorMove) >= coverSettleDelay &&
   s.cursor < len(s.games) &&
   s.games[s.cursor].CoverURL != s.warmedGameURL {
    s.warmPreloadWindow()
}
```

### Right-panel cover render

The single `cache.Get` call for the cover art becomes `cache.Peek`. The
`cache.Failed` check and "Loading…" / "No Image" fallbacks are unchanged.

---

## What Does Not Change

- `Get` and all its callers outside `screen_list.go`
- LRU eviction logic, semaphore, `maxConcurrentFetches = 2`
- `Failed` map and transient-vs-permanent error handling
- No new goroutines or locks introduced
- Detail screen, download screen, and all other screens are unaffected

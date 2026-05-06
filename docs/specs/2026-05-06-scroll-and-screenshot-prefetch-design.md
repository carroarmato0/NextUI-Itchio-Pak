# Design: L1/R1 Smooth Scroll + Detail Screen Screenshot Pre-fetch

**Date:** 2026-05-06
**Scope:** Two independent UX improvements — list screen scroll feel, detail screen image loading.

---

## Feature 1: L1/R1 Smooth Scroll

### Problem

L1/R1 shoulder buttons jump the cursor by `lastVisibleRows` rows per repeat (~10 rows at
150 ms minimum). Each jump teleports `startIdx` discontinuously, making the list content
appear to flash rather than scroll. D-pad single-step scrolling at the same acceleration
curve feels smooth; L1/R1 does not.

### Design

**File:** `internal/ui/screen_list.go`

Two constant changes:

| Constant | Before | After | Reason |
|----------|--------|-------|--------|
| `shoulderAccelMin` | `150ms` | `15ms` | One row per frame at 60fps — fastest physically smooth scroll |
| Jump expression in `processAutoRepeat` | `s.heldShoulderDir * s.lastVisibleRows` | `s.heldShoulderDir` | Single-step, same as D-pad |

The existing acceleration ramp (`currentRepeatInterval`, ease-out cubic from `accelStart`
180ms → new floor 15ms) applies unchanged. At full speed: ~67 rows/sec, roughly 2× D-pad
max (33 rows/sec). The user can still hold L1/R1 to scroll faster than D-pad; the motion
is now continuous rather than discontinuous.

No new state, no animation logic, no other files changed.

---

## Feature 2: Detail Screen Screenshot Pre-fetch

### Problem

When opening a game's detail screen, screenshots are fetched on-demand via `cache.Get()`
only when that screenshot index is displayed. Navigating to the next screenshot shows
"Loading..." until the fetch completes. The cover art (index 0) is already in the cache
from the list screen warm window, but all other screenshots are cold.

### Design

**File:** `internal/ui/screen_detail.go`

Inside the existing fetch goroutine in `NewDetailScreen`, immediately after `ScreenshotURLs`
is assembled (cover prepended at index 0), call `cache.Warm(url)` for every URL in the
slice before setting `s.loading = false`.

```go
// After: d.ScreenshotURLs = append([]string{game.CoverURL}, d.ScreenshotURLs...)
for _, u := range d.ScreenshotURLs {
    cache.Warm(u)
}
```

**Why this location:**
- `ScreenshotURLs` is fully assembled here — all URLs are known.
- The goroutine is already background; `Warm` is goroutine-safe and non-blocking.
- Pre-fetching starts at the earliest possible moment, before `s.loading` flips false.
- Mirrors the existing `warmPreloadWindow` pattern in the list screen exactly.

**Throttling:** `ImageCache` limits concurrent fetches to `maxConcurrentFetches = 2`.
A typical game has 3–8 screenshots; they queue and download sequentially in background.
LRU eviction handles the case where the user navigates away before all fetches complete.

**Cover art:** index 0 is `game.CoverURL`, which is already cached from the list screen
warm window. `Warm` is idempotent — calling it on a cached URL is a no-op.

No new state, no new fields, no interface changes.

---

## Files Changed

| File | Change |
|------|--------|
| `internal/ui/screen_list.go` | `shoulderAccelMin` 150ms → 15ms; jump size `* lastVisibleRows` → `* 1` |
| `internal/ui/screen_detail.go` | `cache.Warm` loop after ScreenshotURLs assembled in fetch goroutine |

---

## Out of Scope

- Proactive screenshot pre-fetch from the list screen (before detail opens)
- Pagination or page-size configuration for L1/R1
- Progressive screenshot warm (only current ± 1) — full warm is simpler and the cache throttles automatically

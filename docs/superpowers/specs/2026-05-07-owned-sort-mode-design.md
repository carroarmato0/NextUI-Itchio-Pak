# Design: OWNED Sort Mode + Owned Badge

**Date:** 2026-05-07  
**Status:** Approved

---

## Overview

Add a new `OWNED` sort mode that shows only games the authenticated user has purchased on itch.io. Simultaneously introduce an `OWNED` badge (teal-green) shown on every paid game the user owns but has not yet downloaded, across **all** sort modes — making ownership status visible throughout the app.

---

## Goals

- Surface the user's purchased itch.io library as a filterable view.
- Make ownership state visible in every sort mode via a consistent badge.
- Require no extra user interaction beyond the existing "Test API Key" flow.
- Survive app restarts via an on-disk cache.

---

## Non-Goals

- Fetching owned games on a background schedule (only refreshed on explicit key re-test).
- Showing games owned via bundles differently from directly-purchased games (both appear identically).
- Persisting download-key IDs anywhere (security).

---

## Architecture

### New file: `internal/itchio/owned_cache.go`

```
OwnedCache {
    SavedAt time.Time
    URLs    []string   // canonical itch.io game URLs
}

SaveOwnedCache(path string, urls []string) error
LoadOwnedCache(path string) ([]string, error)   // returns nil slice on missing file (not an error)
```

Atomic write (tmp → rename), same pattern as `cache.go`.  
Path: `$SHARED_USERDATA_PATH/Itchio/owned_cache.json`  
(resolved in `main_sdl.go` as `filepath.Join(filepath.Dir(cfgPath), "owned_cache.json")`)

---

### `internal/itchio/sort.go` changes

1. Add constant: `SortModeOwned SortMode = "owned"`
2. Append to `SortModes` slice: `..., SortModePaid, SortModeOwned`
3. `SortModeBadge`: return `"OWNED"` for `SortModeOwned`
4. `ApplySort` signature: add `owned map[string]bool` parameter (nil-safe; existing callers pass nil)
5. New `SortModeOwned` case in `ApplySort`:
   - Filter: keep only games where `owned[g.URL]` is true
   - Sort: pending-updates first, then A-Z by sort key
   - No separator (that is a rendering concern, not a sort concern)

Existing callers of `ApplySort` (only `screen_list.go`) are updated to pass the owned map.

---

### `internal/ui/screen_list.go` changes

**New fields on `ListScreen`:**
```go
ownedURLs      map[string]bool
ownedCachePath string
```

**`NewListScreen`:**
- Accept `ownedCachePath string` as new parameter
- Load owned cache from disk; populate `s.ownedURLs` (empty map if file absent)
- Add `ownedUpdateCh chan map[string]bool` field (capacity 1, like `cacheUpdateCh`)
- Build `onOwnedReady func([]itchio.OwnedGame)` closure (runs on background goroutine):
  - Converts `[]OwnedGame` → `map[string]bool`
  - Calls `itchio.SaveOwnedCache(s.ownedCachePath, urls)`
  - Sends map into `s.ownedUpdateCh` (non-blocking send, old value discarded if unread)
  - Pushes `sdl.UserEvent` to wake the SDL loop
- In `Draw` / SDL event handler: drain `ownedUpdateCh`, update `s.ownedURLs`, call `s.rebuildView()`
- Pass `onOwnedReady` to `NewSettingsScreen` calls

**`rebuildView`:**
- Pass `s.ownedURLs` as the new `owned` argument to `itchio.ApplySort`

**Sort cycle gating:**
Replace direct `itchio.NextSortMode` call with a local helper:
```go
func (s *ListScreen) nextSortMode() itchio.SortMode {
    m := itchio.NextSortMode(s.sortMode)
    if m == itchio.SortModeOwned && len(s.ownedURLs) == 0 {
        m = itchio.NextSortMode(m) // skip OWNED if no owned data
    }
    return m
}
```

**Badge rendering (`Draw`):**
Add `isOwned := s.ownedURLs[g.URL]` before the badge switch.  
Insert new case between `isPresent` and `g.IsFree`:
```go
case isOwned:
    badgeLabel = "OWNED"
    badgeR, badgeG, badgeB = 60, 200, 120
```
This case only fires when the game is not already downloaded/updated/removed (those cases are above it in the switch).

**No separator in OWNED mode:**
The `dlSepAfterUpdates` block is gated on `s.sortMode == itchio.SortModeDL` — no change needed.

---

### `internal/ui/screen_settings.go` changes

- Add `onOwnedReady func([]itchio.OwnedGame)` field
- Add to `NewSettingsScreen` signature
- Pass `onOwnedReady` to `NewKeyTestScreen` calls (two call sites)

---

### `internal/ui/screen_apikey_check.go` changes

- Add `onOwnedReady func([]itchio.OwnedGame)` field
- Add to `NewKeyTestScreen` signature
- In the success branch of the goroutine: call `s.onOwnedReady(owned)` before pushing the SDL event
- `onOwnedReady` may be nil — guard with `if s.onOwnedReady != nil`

---

### `cmd/itchio-pak/main_sdl.go` changes

- Add `ownedCachePath := filepath.Join(filepath.Dir(cfgPath), "owned_cache.json")`
- Pass `ownedCachePath` to `ui.NewListScreen`

---

## Badge colour summary

| Label   | R   | G   | B   | Meaning                          |
|---------|-----|-----|-----|----------------------------------|
| UP      | 240 | 160 | 40  | Downloaded, update available     |
| !       | 200 | 60  | 60  | Downloaded, removed from store   |
| DL      | 80  | 200 | 220 | Downloaded                       |
| OWNED   | 60  | 200 | 120 | Purchased, not yet downloaded    |
| Free    | 80  | 200 | 80  | Free game, not downloaded        |
| $X.XX   | 220 | 180 | 60  | Paid, not owned, not downloaded  |

Note: `OWNED` supersedes `Free` and `$X.XX` in the badge switch because it appears earlier in the case order.

---

## Data flow diagram

```
ValidateAPIKey (background goroutine)
    → on success → call onOwnedReady([]OwnedGame)
        → SaveOwnedCache to disk (still on background goroutine)
        → convert URLs → map[string]bool
        → send map into ListScreen.ownedUpdateCh (cap 1, like cacheUpdateCh)
        → sdl.PushEvent(UserEvent) to wake the SDL loop
            → SDL main goroutine reads ownedUpdateCh in Draw/UserEvent handler
                → s.ownedURLs = received map
                → s.rebuildView()
```

This mirrors the `cacheUpdateCh` pattern already used for game-list updates, ensuring `ownedURLs` is only mutated on the SDL goroutine.

---

## Testing

- `internal/itchio/sort_test.go`: add `TestApplySort_Owned` covering:
  - Only owned games appear
  - Pending-updates sorted to top
  - Remaining games A-Z
  - nil `owned` map → empty result
- `internal/itchio/owned_cache_test.go`: round-trip save/load; missing file returns nil slice without error
- No UI tests (SDL-only code excluded from CI via build tag)

---

## Edge cases

| Scenario | Behaviour |
|----------|-----------|
| No API key / key not yet tested | `ownedURLs` empty; OWNED skipped in sort cycle |
| User in OWNED mode, key later invalidated | Mode stays active until next sort cycle; owned cache remains on disk |
| Owned game not in RSS feed | Filtered out silently (not in `cachedGames`) |
| User buys new game between sessions | Not shown until user re-tests the API key |
| `onOwnedReady` called before cache is ready | `rebuildView` is a no-op when `!s.cacheReady`; owned URLs stored and applied when cache arrives |

# Cache Refresh Screen Design

## Goal

When the user selects "Refresh Game List" in Settings, navigate to a dedicated blocking progress screen that shows live fetch progress, a completion message, and an error state — instead of returning silently to the settings list with no visible feedback.

## Architecture

### New screen: `CacheRefreshScreen`

Follows the same state-machine pattern as `FetchUploadsScreen`:

- **States:** `refreshing` | `done` | `refreshError`
- **Fields:**
  - `fetched int64` — atomic counter updated by the `FetchAllGames` progress callback; read in `Draw` via `atomic.LoadInt64`
  - `total int` — set when the goroutine completes (total games saved)
  - `err error` — set on failure
  - `state refreshCacheState` — drives Draw and HandleEvent behaviour
  - `prev Screen` — the Settings screen to return to when done or on error
- **Goroutine:** calls `client.FetchAllGames` with a progress callback, then `itchio.SaveGamesCache`. On completion (success or failure) pushes `sdl.UserEvent` to wake the SDL event loop immediately.
- **`onCacheUpdated func([]itchio.Game)`** callback: called on success so `ListScreen` can update its in-memory state (`cachedGames`, `cacheReady`, `totalGames`, `totalPages`) without the refresh screen needing to know about `ListScreen` internals.

### Callback signature change in `SettingsScreen`

`onRefreshGames` changes from `func()` to `func(prev Screen) Screen`.

- `activate()` case for `sItemRefreshCache` now **returns** the result of `onRefreshGames(s)` instead of returning `s`. This is consistent with how `sItemContentModeration` navigates to a new screen.
- When `onRefreshGames` is `nil` (e.g. `screen_detail.go` call sites), the case falls through and returns `s` unchanged.

### `ListScreen` changes

`triggerCacheRefresh()` is replaced by `newCacheRefreshScreen(prev Screen) Screen`.

This factory method creates a `CacheRefreshScreen` with:
- `s.client` — the shared itch.io HTTP client
- `s.cachePath` — path to `games_cache.json`
- `prev` — the Settings screen (passed through from `onRefreshGames`)
- `onCacheUpdated` callback — updates `s.cachedGames`, `s.cacheReady`, `s.totalGames`, `s.totalPages` on success

The silent background `buildCache()` goroutine used for first-launch and auto-stale refresh is **not changed**.

## UI

### While fetching (`refreshing` state)

```
┌─────────────────────────────────────┐
│  Refreshing Game List               │  ← header bar (same style as other screens)
├─────────────────────────────────────┤
│                                     │
│                                     │
│       Fetching games...             │  ← centred, normal text colour
│       843 fetched                   │  ← centred, updated live via atomic read
│                                     │
│                                     │
├─────────────────────────────────────┤
│  Please wait...                     │  ← footer bar
└─────────────────────────────────────┘
```

- All button input ignored during `refreshing` state.

### On success (`done` state)

```
│       Done!                         │  ← green (80, 200, 80)
│       1823 games cached.            │  ← normal text colour
```

Footer: `"B: back to settings"`

### On error (`refreshError` state)

```
│       Refresh failed:               │  ← red (200, 60, 60)
│       <error message>               │  ← wrapped, lighter red
```

Footer: `"B: back to settings"`

### Input when done or error

B or A → navigate to `prev` (Settings screen).

## Files Changed

| File | Change |
|------|--------|
| `internal/ui/screen_cache_refresh.go` | **New.** `CacheRefreshScreen` with states, goroutine, Draw, HandleEvent |
| `internal/ui/screen_settings.go` | `onRefreshGames func()` → `func(Screen) Screen`; `activate()` returns result of callback |
| `internal/ui/screen_list.go` | Replace `triggerCacheRefresh()` with `newCacheRefreshScreen(Screen) Screen`; update two `NewSettingsScreen` call sites |
| `internal/ui/screen_detail.go` | Two `NewSettingsScreen` call sites: continue passing `nil` — Go accepts `nil` for `func(Screen) Screen`, and the nil-guard in `activate()` already handles it |

## Error Handling

- `FetchAllGames` returns partial results + error on a mid-fetch page failure. `CacheRefreshScreen` treats any non-nil error as `refreshError` and shows the error message. Partial results are discarded (not saved to cache).
- `SaveGamesCache` failure → `refreshError` state, error message shown.
- The `onCacheUpdated` callback is only called on full success (both fetch and save succeeded).

## Invariants

- `buildCache()` on `ListScreen` (silent background goroutine) is unchanged and independent of this screen.
- `CacheRefreshScreen` has no import of or reference to `ListScreen` — communication is one-way via the `onCacheUpdated` callback.
- The atomic `fetched` field is the only field written from a goroutine; all other state transitions happen via the goroutine completing and the SDL event loop reading updated fields on the next `Draw` tick after the `UserEvent` wakes it.

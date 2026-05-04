# Event-Driven Rendering

**Date:** 2026-05-04
**Status:** Approved

## Problem

The main render loop calls `Draw()` → `SDL_RenderPresent()` unconditionally every 16ms (~60fps),
writing directly to the raw framebuffer even when the screen content has not changed. On devices
without a display compositor or vsync, this causes visible tearing and flickering — most noticeable
during the screenshot capture workflow and whenever background goroutines complete and trigger
a re-render mid-settle.

A secondary issue: long game titles in the list screen begin their horizontal scroll animation
immediately when the cursor moves, including during fast D-pad navigation. The desired behaviour
is that scrolling only starts after the user has been stationary on a game for ~1 second.

## Goals

- Eliminate unnecessary `SDL_RenderPresent` calls when screen content has not changed.
- CPU-idle when the user is not interacting and no animation is running.
- Maintain 60fps rendering during: held-button auto-repeat, scroll animations, and active
  progress screens (download, fetch, cache refresh).
- Title scroll begins only after the cursor has been stationary for `scrollDelay` (1 second).
  This is already enforced by the existing `titleScrollAt` reset-on-move logic; the new
  `NeedsRedraw()` implementation preserves and makes it explicit.

## Architecture

### 1. Screen interface — `NeedsRedraw() bool`

`internal/ui/screen.go` gains one method on the `Screen` interface:

```go
// NeedsRedraw returns true when the screen has active time-based state
// (auto-repeat, scroll animation, progress indicator) that requires the
// render loop to keep calling Draw() even without incoming SDL events.
NeedsRedraw() bool
```

### 2. Main render loop — `main_sdl.go`

Replace the current `PollEvent` + `sdl.Delay(16)` pattern with `WaitEventTimeout`:

```
loop:
  newImages := cache.ProcessPending(r)   // bool: true if any textures were uploaded

  gotEvent := false
  if e := sdl.WaitEventTimeout(16); e != nil {
      gotEvent = true
      for e != nil {
          // existing intercept logic (SDL_QUIT, power events, inventory update)
          current = current.HandleEvent(e)
          e = sdl.PollEvent()   // drain remaining queued events
      }
  }

  if current != nil && (gotEvent || newImages || current.NeedsRedraw()) {
      current.Draw(r)
  }
  // sdl.Delay(16) is removed
```

`WaitEventTimeout(16)` blocks until an SDL event arrives or 16ms elapses. When the screen
is truly idle (`NeedsRedraw()` false, no events, no new images) the loop sleeps continuously
and burns no CPU. The 16ms timeout fires only when a screen has active animation, capping
render rate at 60fps.

### 3. `ProcessPending` — `internal/renderer/image_cache.go`

Change signature from `func (c *ImageCache) ProcessPending(r *Renderer)` to
`func (c *ImageCache) ProcessPending(r *Renderer) bool`.

Returns `true` if at least one texture was uploaded from a background goroutine this call.
This lets the main loop trigger a `Draw()` immediately when new cover art arrives, without
waiting for the next user event or animation tick.

### 4. `NeedsRedraw()` implementations

Each screen falls into one of four buckets:

| Bucket | `NeedsRedraw()` returns | Screens |
|--------|------------------------|---------|
| Auto-repeat only | `s.heldDir != 0` | `SettingsScreen`, `ContentModerationScreen`, `TagFilterScreen` |
| Auto-repeat + scroll | `s.heldDir != 0 \|\| scrollActive()` | `ListScreen`, `DetailScreen` |
| Always animating | `true` | `DownloadScreen`, `FetchUploadsScreen`, `CacheRefreshScreen` |
| Fully static | `false` | `AboutScreen`, `ManageDownloadsScreen`, `MigrateFlowScreen`, `RomPickerScreen`, `LocationPickerScreen`, `FormatPickerScreen`, `PurchasePickerScreen`, `APIKeyCheckScreen` |

**`ListScreen.NeedsRedraw()`** (scroll timing detail):

```go
func (s *ListScreen) NeedsRedraw() bool {
    if s.heldDir != 0 {
        return true
    }
    // Wake up 500ms before scrollDelay expires so the first animation frame
    // is not missed. scrollDelay = 1s, so rendering resumes at t=500ms and
    // the visible scroll starts at t=1000ms.
    return !s.titleScrollAt.IsZero() &&
        time.Since(s.titleScrollAt) > scrollDelay/2
}
```

`DetailScreen.NeedsRedraw()` mirrors this using `pathScrollAt`.

**"Always animating" screens** return `true` because their progress callbacks do not push
SDL events on each increment — only on completion. The 16ms `WaitEventTimeout` timeout
caps their render rate at 60fps without additional changes.

**Static screens** have no time-based state; they change only in response to button presses
(which are SDL events), so `false` is correct.

## Title Scroll Behaviour (existing logic, made explicit)

`titleScrollAt` is already reset to `time.Now()` on every cursor move, including each
auto-repeat tick. `Draw()` only advances `titleScrollX` once
`time.Since(titleScrollAt) > scrollDelay` (1 second). During fast D-pad navigation the
timer resets continuously, so the scroll never starts. No logic change is required; the
`NeedsRedraw()` 500ms head-start window ensures the animation loop resumes in time to
catch the scroll start.

Timeline after last cursor move:
- **0–500ms**: `NeedsRedraw()` false → loop sleeps, no renders
- **500ms**: `NeedsRedraw()` true → `Draw()` resumes at 60fps
- **1000ms**: `scrollDelay` expires inside `Draw()` → title scrolls visually

## Files Changed

| File | Change |
|------|--------|
| `internal/ui/screen.go` | Add `NeedsRedraw() bool` to `Screen` interface |
| `internal/renderer/image_cache.go` | `ProcessPending` returns `bool` |
| `cmd/itchio-pak/main_sdl.go` | `WaitEventTimeout` loop, conditional `Draw()` |
| `internal/ui/screen_list.go` | `NeedsRedraw()` with scroll timing |
| `internal/ui/screen_detail.go` | `NeedsRedraw()` with path scroll timing |
| `internal/ui/screen_settings.go` | `NeedsRedraw()` — `heldDir != 0` |
| `internal/ui/screen_content_moderation.go` | `NeedsRedraw()` — `heldDir != 0` |
| `internal/ui/screen_tag_filter.go` | `NeedsRedraw()` — `heldDir != 0` |
| `internal/ui/screen_download.go` | `NeedsRedraw()` — `true` |
| `internal/ui/screen_fetch_uploads.go` | `NeedsRedraw()` — `true` |
| `internal/ui/screen_cache_refresh.go` | `NeedsRedraw()` — `true` |
| `internal/ui/screen_about.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_manage_downloads.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_migrate_flow.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_rom_picker.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_location_picker.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_format_picker.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_purchase_picker.go` | `NeedsRedraw()` — `false` |
| `internal/ui/screen_apikey_check.go` | `NeedsRedraw()` — `false` |

## Testing

The rendering path is excluded from CI (`//go:build !headless`). Verification is done via
the device screenshot workflow:

1. `./scripts/dev-screenshot.sh --screen settings` — confirm no flicker during settle period
2. `./scripts/dev-screenshot.sh --screen list` — confirm cover art loads and renders
3. Manual device test: hold D-pad on list screen, release, confirm title scroll starts ~1s later
4. Manual device test: trigger a download, confirm progress bar animates smoothly

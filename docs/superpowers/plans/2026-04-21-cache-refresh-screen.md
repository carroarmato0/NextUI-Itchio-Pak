# Cache Refresh Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a dedicated blocking progress screen when the user triggers "Refresh Game List" in Settings, with a live fetched-games counter, a success state, and an error state.

**Architecture:** A new `CacheRefreshScreen` owns the full fetch-and-save cycle (via `FetchAllGames` + `SaveGamesCache`) and notifies `ListScreen` on completion via an `onCacheUpdated` callback. `SettingsScreen`'s `onRefreshGames` callback changes from `func()` to `func(Screen) Screen` so `activate()` can return the new screen to navigate to. The silent background `buildCache()` goroutine on `ListScreen` (used for first-launch and auto-stale refresh) is left unchanged.

**Tech Stack:** Go 1.22+, SDL2 via `go-sdl2`, existing `itchio.Client`, existing renderer drawing primitives (`DrawHeaderBar`, `DrawFooterBar`, `DrawTextCentered`, `DrawWrappedText`).

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/ui/screen_cache_refresh.go` | **Create** | New screen: states, goroutine, Draw, HandleEvent |
| `internal/ui/screen_settings.go` | **Modify** | `onRefreshGames` type → `func(Screen) Screen`; `activate()` returns the new screen |
| `internal/ui/screen_list.go` | **Modify** | Replace `triggerCacheRefresh()` with `newCacheRefreshScreen(Screen) Screen`; update two call sites |

`internal/ui/screen_detail.go` — **no change**. Its two `NewSettingsScreen` call sites already pass `nil`; Go accepts `nil` for any function type so the signature change is backward-compatible.

---

### Task 1: `CacheRefreshScreen` (`internal/ui/screen_cache_refresh.go`)

**Files:**
- Create: `internal/ui/screen_cache_refresh.go`

This file has `//go:build !headless` so it cannot be unit-tested. Verification is a clean headless build.

- [ ] **Step 1: Create the file**

```go
//go:build !headless

package ui

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
	"github.com/veandco/go-sdl2/sdl"
)

type refreshCacheState int

const (
	refreshCacheLoading refreshCacheState = iota
	refreshCacheDone
	refreshCacheError
)

// CacheRefreshScreen is a blocking progress screen shown while the full game
// list is being re-fetched. It pushes a UserEvent when the goroutine finishes
// so the SDL event loop wakes up immediately.
type CacheRefreshScreen struct {
	client         *itchio.Client
	cachePath      string
	prev           Screen
	onCacheUpdated func([]itchio.Game)

	state   refreshCacheState
	fetched int64 // written atomically from goroutine, read in Draw
	total   int   // number of games saved (success only)
	err     error // set on failure
}

// NewCacheRefreshScreen creates the screen and immediately starts the
// background fetch. onCacheUpdated is called on success so the caller can
// update its in-memory state; it may be nil.
func NewCacheRefreshScreen(
	client *itchio.Client,
	cachePath string,
	prev Screen,
	onCacheUpdated func([]itchio.Game),
) *CacheRefreshScreen {
	s := &CacheRefreshScreen{
		client:         client,
		cachePath:      cachePath,
		prev:           prev,
		onCacheUpdated: onCacheUpdated,
		state:          refreshCacheLoading,
	}
	go func() {
		games, err := client.FetchAllGames(context.Background(), func(fetched int) {
			atomic.StoreInt64(&s.fetched, int64(fetched))
		})
		if err != nil {
			logger.Error("cache refresh: failed after %d games: %v", len(games), err)
			s.err = err
			s.state = refreshCacheError
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			return
		}
		if err := itchio.SaveGamesCache(cachePath, games); err != nil {
			logger.Error("cache refresh: save failed: %v", err)
			s.err = err
			s.state = refreshCacheError
			sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
			return
		}
		logger.Info("cache refresh: saved %d games", len(games))
		s.total = len(games)
		if onCacheUpdated != nil {
			onCacheUpdated(games)
		}
		s.state = refreshCacheDone
		sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT})
	}()
	return s
}

func (s *CacheRefreshScreen) Draw(r *renderer.Renderer) {
	r.Clear(colorBG, colorBG, colorBG)

	headerH := int32(72)
	footerH := int32(40)
	textY := r.DrawHeaderBar(headerH)
	r.DrawText("Refreshing Game List", 20, textY, colorText, colorText, colorText)

	_, fontH := r.TextSize("Ag")
	contentTop := headerH + 4
	contentH := r.H - headerH - footerH
	mid := contentTop + contentH/2

	switch s.state {
	case refreshCacheLoading:
		fetched := atomic.LoadInt64(&s.fetched)
		r.DrawTextCentered("Fetching games...", 0, mid-fontH-4, r.W, colorText, colorText, colorText)
		r.DrawTextCentered(fmt.Sprintf("%d fetched", fetched), 0, mid+4, r.W, colorText, colorText, colorText)

	case refreshCacheDone:
		r.DrawTextCentered("Done!", 0, mid-fontH-4, r.W, 80, 200, 80)
		r.DrawTextCentered(fmt.Sprintf("%d games cached.", s.total), 0, mid+4, r.W, colorText, colorText, colorText)

	case refreshCacheError:
		r.DrawTextCentered("Refresh failed:", 0, mid-fontH-8, r.W, 200, 60, 60)
		r.DrawWrappedText(s.err.Error(), 20, mid, r.W-40, fontH+4, 200, 100, 100)
	}

	ftrY := r.DrawFooterBar(footerH)
	switch s.state {
	case refreshCacheLoading:
		r.DrawSmallText("Please wait...", 10, ftrY, 140, 140, 140)
	default:
		r.DrawSmallText("B: back to settings", 10, ftrY, 140, 140, 140)
	}
	r.Present()
}

func (s *CacheRefreshScreen) HandleEvent(e sdl.Event) Screen {
	switch ev := e.(type) {
	case *sdl.KeyboardEvent:
		if ev.Type != sdl.KEYDOWN {
			return s
		}
		if s.state != refreshCacheLoading {
			switch ev.Keysym.Sym {
			case sdl.K_ESCAPE, sdl.K_RETURN:
				return s.prev
			}
		}
	case *sdl.ControllerButtonEvent:
		if ev.Type != sdl.CONTROLLERBUTTONDOWN {
			return s
		}
		if s.state != refreshCacheLoading {
			switch ev.Button {
			case sdl.CONTROLLER_BUTTON_A, sdl.CONTROLLER_BUTTON_B:
				return s.prev
			}
		}
	case *sdl.QuitEvent:
		return nil
	}
	return s
}
```

- [ ] **Step 2: Verify build**

```bash
cd /home/carroarmato0/Applications/Development/NextUI/Paks/Itch-io
go build -tags headless ./...
```

Expected: clean build (no errors). The new file compiles because the headless tag excludes it from the build — if there are import errors they still surface.

Actually the `!headless` tag means the file IS included in a non-headless build but excluded from headless. To catch compilation errors with our setup, run without the tag (this will fail on SDL2 link but type-checks):

```bash
go vet -tags headless ./...
```

Expected: no vet errors.

- [ ] **Step 3: Commit**

```bash
git add internal/ui/screen_cache_refresh.go
git commit -m "feat(ui): add CacheRefreshScreen with live progress counter"
```

---

### Task 2: Update `SettingsScreen` callback signature (`internal/ui/screen_settings.go`)

**Files:**
- Modify: `internal/ui/screen_settings.go`

Two changes: the `onRefreshGames` field type, and the `activate()` return for `sItemRefreshCache`.

- [ ] **Step 1: Change `onRefreshGames` field type in `SettingsScreen` struct**

Find:
```go
	onRefreshGames  func() // nil if not available
```

Replace with:
```go
	onRefreshGames func(Screen) Screen // nil if not available
```

- [ ] **Step 2: Update `NewSettingsScreen` signature**

Find:
```go
func NewSettingsScreen(cfg *settings.Config, cfgPath string, prev Screen, onRefreshGames func()) *SettingsScreen {
```

Replace with:
```go
func NewSettingsScreen(cfg *settings.Config, cfgPath string, prev Screen, onRefreshGames func(Screen) Screen) *SettingsScreen {
```

The body is unchanged (`return &SettingsScreen{cfg: cfg, cfgPath: cfgPath, prev: prev, onRefreshGames: onRefreshGames}`).

- [ ] **Step 3: Update `activate()` to return the new screen**

Find:
```go
	case sItemRefreshCache:
		if s.onRefreshGames != nil {
			s.onRefreshGames()
		}
```

Replace with:
```go
	case sItemRefreshCache:
		if s.onRefreshGames != nil {
			return s.onRefreshGames(s)
		}
```

- [ ] **Step 4: Verify build**

```bash
go build -tags headless ./...
```

Expected: clean build. (Note: `screen_list.go` still references `s.triggerCacheRefresh` which has type `func()` — this will cause a compile error. That is expected and will be fixed in Task 3.)

If the build fails only on the `triggerCacheRefresh` type mismatch, that is expected — proceed to Task 3 immediately.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/screen_settings.go
git commit -m "feat(ui): change onRefreshGames callback to func(Screen) Screen"
```

---

### Task 3: Replace `triggerCacheRefresh` with `newCacheRefreshScreen` (`internal/ui/screen_list.go`)

**Files:**
- Modify: `internal/ui/screen_list.go`

Two changes: replace the `triggerCacheRefresh` method, and update the two `NewSettingsScreen` call sites.

- [ ] **Step 1: Replace `triggerCacheRefresh` method**

Find the existing method (around line 535):
```go
// triggerCacheRefresh is the callback handed to SettingsScreen for the
// "Refresh Game List" menu item.
func (s *ListScreen) triggerCacheRefresh() {
	logger.Info("cache: manual refresh triggered from settings")
	go s.buildCache()
}
```

Replace with:
```go
// newCacheRefreshScreen returns a CacheRefreshScreen that runs a full cache
// rebuild and notifies this ListScreen on completion via onCacheUpdated.
// It is passed to SettingsScreen as the onRefreshGames callback.
func (s *ListScreen) newCacheRefreshScreen(prev Screen) Screen {
	return NewCacheRefreshScreen(s.client, s.cachePath, prev, func(games []itchio.Game) {
		s.cachedGames = games
		s.cacheReady = true
		s.totalGames = len(games)
		s.totalPages = (len(games) + itchio.PerPage - 1) / itchio.PerPage
	})
}
```

- [ ] **Step 2: Update both `NewSettingsScreen` call sites**

There are two call sites in `HandleEvent` (one for keyboard `sdl.K_s`, one for controller `sdl.CONTROLLER_BUTTON_START`). Find both:

```go
return NewSettingsScreen(s.cfg, s.cfgPath, s, s.triggerCacheRefresh)
```

Replace both with:

```go
return NewSettingsScreen(s.cfg, s.cfgPath, s, s.newCacheRefreshScreen)
```

- [ ] **Step 3: Verify build and tests**

```bash
go build -tags headless ./...
go test ./...
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/screen_list.go
git commit -m "feat(ui): wire CacheRefreshScreen into ListScreen via newCacheRefreshScreen"
```

---

## Self-Review

**Spec coverage:**

| Requirement | Task |
|---|---|
| New `CacheRefreshScreen` with `refreshing`/`done`/`error` states | Task 1 |
| Live `fetched` counter via `atomic.StoreInt64` / `atomic.LoadInt64` | Task 1 |
| SDL `UserEvent` push on goroutine completion | Task 1 |
| "Done! N games cached." in green on success | Task 1 (`Draw`, `refreshCacheDone` case) |
| Error message in red on failure | Task 1 (`Draw`, `refreshCacheError` case) |
| "Please wait..." footer during fetch; "B: back" when done | Task 1 |
| All input blocked during `refreshCacheLoading` | Task 1 (`HandleEvent`) |
| B/A navigates to `prev` when done or error | Task 1 (`HandleEvent`) |
| `onRefreshGames` changed to `func(Screen) Screen` | Task 2 |
| `activate()` returns the new screen | Task 2 |
| `triggerCacheRefresh()` replaced by `newCacheRefreshScreen(Screen) Screen` | Task 3 |
| Both `NewSettingsScreen` call sites updated | Task 3 |
| `screen_detail.go` unchanged (`nil` stays valid) | — (no task needed) |
| Silent `buildCache()` goroutine unchanged | — (untouched) |
| `onCacheUpdated` updates `ListScreen` fields on success | Task 3 |

**Placeholder scan:** No TBDs or incomplete steps.

**Type consistency:**
- `CacheRefreshScreen` — defined in Task 1, returned in Task 3 via `NewCacheRefreshScreen`
- `NewCacheRefreshScreen(client, cachePath, prev, onCacheUpdated)` — defined in Task 1, called in Task 3
- `onRefreshGames func(Screen) Screen` — defined in Task 2, satisfied by `s.newCacheRefreshScreen` in Task 3 (method value `func(Screen) Screen` ✓)
- `colorBG`, `colorText` — pre-existing constants in the `ui` package, used correctly in Task 1
- `refreshCacheLoading`, `refreshCacheDone`, `refreshCacheError` — defined and used within Task 1 only

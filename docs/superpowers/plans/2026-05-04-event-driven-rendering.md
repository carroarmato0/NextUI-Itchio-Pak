# Event-Driven Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the unconditional 60fps render loop with event-driven rendering that only calls `Draw()` when screen content has changed, eliminating raw-framebuffer flicker on device.

**Architecture:** Add `NeedsRedraw() bool` to the `Screen` interface; change `ProcessPending` to return `bool`; replace `PollEvent + Delay(16)` with `WaitEventTimeout(16)` guarded by `gotEvent || newImages || current.NeedsRedraw()`. Each screen returns true only when it has active time-based state (held button, scroll animation, progress indicator).

**Tech Stack:** Go 1.22+, SDL2 via `go-sdl2`, `//go:build !headless` on all renderer/UI files (no headless unit tests for SDL code — verification is by `build.sh native` + device screenshot).

---

## File Map

| File | Change |
|------|--------|
| `internal/renderer/image_cache.go` | `ProcessPending` returns `bool` |
| `internal/ui/screen.go` | Add `NeedsRedraw() bool` to `Screen` interface |
| `internal/ui/screen_list.go` | Extract `scrollDelay` const; add `NeedsRedraw()` |
| `internal/ui/screen_detail.go` | Extract `pathScrollDelay` const; add `NeedsRedraw()` |
| `internal/ui/screen_settings.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_content_moderation.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_tag_filter.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_download.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_fetch_uploads.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_cache_refresh.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_about.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_manage_downloads.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_migrate_flow.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_rom_picker.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_location_picker.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_format_picker.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_purchase_picker.go` | Add `NeedsRedraw()` |
| `internal/ui/screen_apikey_check.go` | Add `NeedsRedraw()` |
| `cmd/itchio-pak/main_sdl.go` | Replace poll+delay loop with `WaitEventTimeout` |

---

## Task 1: `ProcessPending` returns `bool`

**Files:**
- Modify: `internal/renderer/image_cache.go:191-200`

This file has `//go:build !headless` — no headless unit test is possible. Verification is the native build in Task 4.

- [ ] **Step 1: Change `ProcessPending` to return `bool`**

Open `internal/renderer/image_cache.go`. Replace the function:

```go
func (c *ImageCache) ProcessPending(r *Renderer) {
	for {
		select {
		case raw := <-c.readyCh:
			c.uploadTexture(r, raw)
		default:
			return
		}
	}
}
```

with:

```go
func (c *ImageCache) ProcessPending(r *Renderer) bool {
	uploaded := false
	for {
		select {
		case raw := <-c.readyCh:
			c.uploadTexture(r, raw)
			uploaded = true
		default:
			return uploaded
		}
	}
}
```

- [ ] **Step 2: Fix the call site in `main_sdl.go`**

Open `cmd/itchio-pak/main_sdl.go`. Find the line:

```go
cache.ProcessPending(r)
```

Change it to (temporary — will be replaced properly in Task 3):

```go
_ = cache.ProcessPending(r)
```

- [ ] **Step 3: Run the headless test suite to confirm nothing broke**

```bash
./scripts/test.sh ./...
```

Expected: all packages `ok`. The renderer package is excluded from headless CI so no new failures are expected.

- [ ] **Step 4: Commit**

```bash
git add internal/renderer/image_cache.go cmd/itchio-pak/main_sdl.go
git commit -m "feat: ProcessPending returns bool indicating whether images were uploaded"
```

---

## Task 2: `NeedsRedraw()` on Screen interface + all implementations

**Files:**
- Modify: `internal/ui/screen.go`
- Modify: `internal/ui/screen_list.go`
- Modify: `internal/ui/screen_detail.go`
- Modify: 8 remaining screen files (steps below show each)

All files have `//go:build !headless`. The SDL build (`build.sh native`) is the compiler gate.

### Step group A — interface + scroll-constant extraction

- [ ] **Step 1: Add `NeedsRedraw()` to the Screen interface**

Open `internal/ui/screen.go`. Replace:

```go
// Screen is implemented by every UI screen.
// Draw renders the current frame.
// HandleEvent processes one SDL event and returns the next screen.
// Returning nil exits the application. Returning self means no transition.
type Screen interface {
	Draw(r *renderer.Renderer)
	HandleEvent(e sdl.Event) Screen
}
```

with:

```go
// Screen is implemented by every UI screen.
// Draw renders the current frame.
// HandleEvent processes one SDL event and returns the next screen.
// Returning nil exits the application. Returning self means no transition.
// NeedsRedraw returns true when the screen has active time-based state
// (auto-repeat, scroll animation, progress indicator) that requires the
// render loop to keep calling Draw() without an incoming SDL event.
type Screen interface {
	Draw(r *renderer.Renderer)
	HandleEvent(e sdl.Event) Screen
	NeedsRedraw() bool
}
```

- [ ] **Step 2: Extract `scrollDelay` to file scope in `screen_list.go`**

Open `internal/ui/screen_list.go`. Find the constants inside `Draw()`:

```go
const scrollDelay = time.Second
const scrollSpeed = int32(50)
```

These are local to `Draw()`. Add them as file-level constants just before the `ListScreen` struct definition (or near the top of the file after imports). Search for the first `const` block or `type ListScreen struct` and insert above it:

```go
const (
	scrollDelay = time.Second
	scrollSpeed = int32(50)
)
```

Then remove the duplicate `const` declarations from inside `Draw()`. There are two copies inside `Draw()` — search for `const scrollDelay` and `const scrollSpeed` within the function body and delete both lines.

- [ ] **Step 3: Add `NeedsRedraw()` to `ListScreen`**

In `internal/ui/screen_list.go`, add after the `moveCursor` or `processAutoRepeat` method (anywhere before `Draw`):

```go
func (s *ListScreen) NeedsRedraw() bool {
	if s.heldDir != 0 {
		return true
	}
	// Resume rendering 500ms before scrollDelay expires so the first
	// animation frame is not missed when the cursor has been stationary.
	return !s.titleScrollAt.IsZero() &&
		time.Since(s.titleScrollAt) > scrollDelay/2
}
```

- [ ] **Step 4: Extract `pathScrollDelay` to file scope in `screen_detail.go`**

Open `internal/ui/screen_detail.go`. Find inside `Draw()`:

```go
const pathScrollDelay = time.Second
const pathScrollSpeed = int32(50)
```

Add file-level constants before the `DetailScreen` struct definition:

```go
const (
	pathScrollDelay = time.Second
	pathScrollSpeed = int32(50)
)
```

Then remove the duplicate `const` lines from inside `Draw()`.

- [ ] **Step 5: Add `NeedsRedraw()` to `DetailScreen`**

In `internal/ui/screen_detail.go`, add after the `processAutoScroll` method:

```go
func (s *DetailScreen) NeedsRedraw() bool {
	if s.heldDir != 0 {
		return true
	}
	// Resume rendering 500ms before pathScrollDelay expires so the first
	// animation frame is not missed when the cursor has been stationary.
	return !s.pathScrollAt.IsZero() &&
		time.Since(s.pathScrollAt) > pathScrollDelay/2
}
```

### Step group B — auto-repeat screens

- [ ] **Step 6: Add `NeedsRedraw()` to `SettingsScreen`**

In `internal/ui/screen_settings.go`, add after the `moveCursor` method:

```go
func (s *SettingsScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
```

- [ ] **Step 7: Add `NeedsRedraw()` to `ContentModerationScreen`**

Open `internal/ui/screen_content_moderation.go`. Add after whatever method precedes `Draw`:

```go
func (s *ContentModerationScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
```

- [ ] **Step 8: Add `NeedsRedraw()` to `TagFilterScreen`**

Open `internal/ui/screen_tag_filter.go`. Add after whatever method precedes `Draw`:

```go
func (s *TagFilterScreen) NeedsRedraw() bool {
	return s.heldDir != 0
}
```

### Step group C — always-animating progress screens

- [ ] **Step 9: Add `NeedsRedraw()` to `DownloadScreen`**

Open `internal/ui/screen_download.go`. Add before or after `Draw`:

```go
func (s *DownloadScreen) NeedsRedraw() bool {
	return true
}
```

- [ ] **Step 10: Add `NeedsRedraw()` to `FetchUploadsScreen`**

Open `internal/ui/screen_fetch_uploads.go`. Add before or after `Draw`:

```go
func (s *FetchUploadsScreen) NeedsRedraw() bool {
	return true
}
```

- [ ] **Step 11: Add `NeedsRedraw()` to `CacheRefreshScreen`**

Open `internal/ui/screen_cache_refresh.go`. Add before or after `Draw`:

```go
func (s *CacheRefreshScreen) NeedsRedraw() bool {
	return true
}
```

### Step group D — static screens

Each of the following screens has no time-based state. Add the method returning `false` to each.

- [ ] **Step 12: `AboutScreen`** — `internal/ui/screen_about.go`

```go
func (s *AboutScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 13: `ManageDownloadsScreen`** — `internal/ui/screen_manage_downloads.go`

```go
func (s *ManageDownloadsScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 14: `MigrateFlowScreen`** — `internal/ui/screen_migrate_flow.go`

```go
func (s *MigrateFlowScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 15: `RomPickerScreen`** — `internal/ui/screen_rom_picker.go`

```go
func (s *RomPickerScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 16: `LocationPickerScreen`** — `internal/ui/screen_location_picker.go`

```go
func (s *LocationPickerScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 17: `FormatPickerScreen`** — `internal/ui/screen_format_picker.go`

```go
func (s *FormatPickerScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 18: `PurchasePickerScreen`** — `internal/ui/screen_purchase_picker.go`

```go
func (s *PurchasePickerScreen) NeedsRedraw() bool { return false }
```

- [ ] **Step 19: `APIKeyCheckScreen`** — `internal/ui/screen_apikey_check.go`

```go
func (s *APIKeyCheckScreen) NeedsRedraw() bool { return false }
```

### Step group E — build + commit

- [ ] **Step 20: Run the headless tests**

```bash
./scripts/test.sh ./...
```

Expected: all packages `ok`.

- [ ] **Step 21: Run the native SDL build to confirm all interface implementations compile**

```bash
./scripts/build.sh native
```

Expected output ends with: `Built: bin/native/itchio-pak`

If the build fails with `does not implement Screen (missing NeedsRedraw method)`, find the missing screen and add the method.

- [ ] **Step 22: Commit**

```bash
git add internal/ui/screen.go \
        internal/ui/screen_list.go \
        internal/ui/screen_detail.go \
        internal/ui/screen_settings.go \
        internal/ui/screen_content_moderation.go \
        internal/ui/screen_tag_filter.go \
        internal/ui/screen_download.go \
        internal/ui/screen_fetch_uploads.go \
        internal/ui/screen_cache_refresh.go \
        internal/ui/screen_about.go \
        internal/ui/screen_manage_downloads.go \
        internal/ui/screen_migrate_flow.go \
        internal/ui/screen_rom_picker.go \
        internal/ui/screen_location_picker.go \
        internal/ui/screen_format_picker.go \
        internal/ui/screen_purchase_picker.go \
        internal/ui/screen_apikey_check.go
git commit -m "feat: add NeedsRedraw() to Screen interface; implement on all screens"
```

---

## Task 3: Rewrite the main render loop

**Files:**
- Modify: `cmd/itchio-pak/main_sdl.go:161-253`

- [ ] **Step 1: Replace the loop body**

Open `cmd/itchio-pak/main_sdl.go`. Find the entire loop from `loop:` through `sdl.Delay(16) // ~60 fps` and replace it with:

```go
loop:
	for current != nil {
		// Upload any images that background goroutines finished fetching.
		// Returns true if at least one texture was uploaded this call.
		newImages := cache.ProcessPending(r)

		// Block until an SDL event arrives or 16ms elapses.
		// When the screen is idle (NeedsRedraw false, no events, no new images)
		// this keeps the loop sleeping at near-zero CPU.
		gotEvent := false
		e := sdl.WaitEventTimeout(16)
		for e != nil {
			gotEvent = true
			if pendingQuit {
				e = sdl.PollEvent()
				continue // drain input while waiting for tasks
			}
			// Intercept SDL_QUIT (SIGTERM from NextUI) before screens see it.
			if _, ok := e.(*sdl.QuitEvent); ok {
				current = nil
				break loop
			}
			// Intercept UserEvents before screens see them.
			if uev, ok := e.(*sdl.UserEvent); ok {
				switch uev.Code {
				case userEventInventoryUpdate:
					// Update-svc finished a check; rebuild the list view so
					// new [UP]/[!] badges and DL-sort order are immediately visible.
					listScreen.ScheduleRebuild()
					// Fall through — do NOT continue. FetchUploadsScreen also uses
					// UserEvent code 0 for its goroutine-done signal, so the event
					// must still reach current.HandleEvent(e).
				case userEventPowerSleep:
					logger.Info("power: sleep requested, waiting for tasks")
					pendingQuit = true
					pendingAction = power.ActionSleep
					updateSvc.Stop()
					e = sdl.PollEvent()
					continue
				case userEventPowerShutdown:
					logger.Info("power: shutdown requested, waiting for tasks")
					pendingQuit = true
					pendingAction = power.ActionShutdown
					updateSvc.Stop()
					e = sdl.PollEvent()
					continue
				}
			}
			current = current.HandleEvent(e)
			if current == nil {
				break loop
			}
			e = sdl.PollEvent()
		}
		if current == nil {
			break loop
		}
		if pendingQuit {
			var busy bool
			if bc, ok := current.(ui.BusyChecker); ok {
				busy = bc.IsBusy()
			}
			if !busy && !updateSvc.IsRunning() {
				if pendingAction == power.ActionShutdown {
					logger.Info("power: all tasks done, writing /tmp/poweroff")
					if err := os.WriteFile("/tmp/poweroff", []byte{}, 0644); err != nil {
						logger.Error("power: /tmp/poweroff: %v", err)
					}
					break loop // exit cleanly; NextUI detects /tmp/poweroff and shuts down
				}
				suspendPath := filepath.Join(os.Getenv("SYSTEM_PATH"), "bin", "suspend")
				if _, err := os.Stat(suspendPath); err != nil {
					logger.Warn("power: suspend script not found at %s, exiting instead", suspendPath)
					current = nil
				} else {
					logger.Info("power: all tasks done, calling %s", suspendPath)
					if err := exec.Command(suspendPath).Run(); err != nil {
						logger.Error("power: suspend: %v", err)
					}
					logger.Info("power: resumed from sleep")
					powerMgr.PostWake()
					// Flush any power UserEvents the goroutine queued while
					// processing the wake-up key press. They arrived before
					// suspend.Run() returned, so PostWake() alone is too late.
					for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
						if uev, ok := e.(*sdl.UserEvent); ok &&
							(uev.Code == userEventPowerSleep || uev.Code == userEventPowerShutdown) {
							logger.Info("power: discarding buffered wake-up event")
							continue
						}
						current = current.HandleEvent(e)
						if current == nil {
							break loop
						}
					}
					pendingQuit = false
				}
			} else {
				drawPowerPendingOverlay(r, pendingAction)
			}
		} else if gotEvent || newImages || current.NeedsRedraw() {
			current.Draw(r)
		}
	}
```

- [ ] **Step 2: Run headless tests**

```bash
./scripts/test.sh ./...
```

Expected: all packages `ok`.

- [ ] **Step 3: Run native SDL build**

```bash
./scripts/build.sh native
```

Expected: `Built: bin/native/itchio-pak`

- [ ] **Step 4: Commit**

```bash
git add cmd/itchio-pak/main_sdl.go
git commit -m "feat: event-driven render loop — only Draw() on event, new image, or NeedsRedraw()"
```

---

## Task 4: Cross-compile and device verification

**Files:** none (verification only)

- [ ] **Step 1: Cross-compile for device**

```bash
./scripts/build.sh tg5040
```

Expected: `Built: bin/tg5040/itchio-pak`

- [ ] **Step 2: Capture settings screenshot — confirm no flicker during settle**

```bash
./scripts/dev-screenshot.sh --screen settings --out /tmp/itchio-screenshots/settings-event-driven.png
```

Expected: exit code 0, screenshot saved. The terminal output should NOT show exit code 143 (already fixed). On the device, the screen should appear without intermittent flickering during the 4-second settle window.

- [ ] **Step 3: Capture list screenshot — confirm cover art renders**

```bash
./scripts/dev-screenshot.sh --screen list --out /tmp/itchio-screenshots/list-event-driven.png
```

Expected: screenshot shows the game list with cover art loaded on the right panel.

- [ ] **Step 4: Manual device test — title scroll timing**

Deploy to device:

```bash
./scripts/deploy.sh
```

On the device: open the Itch.io pak, navigate to the game list, hold the D-pad down for fast scrolling, then release. Confirm:
- No title scroll animation starts while holding the button
- After releasing, the title of the selected game begins scrolling approximately 1 second later

- [ ] **Step 5: Manual device test — download progress**

Trigger a game download. Confirm the progress indicator animates smoothly throughout the download.

- [ ] **Step 6: Final commit (if any last-minute fixes were made)**

If no fixes were needed, skip. Otherwise:

```bash
git add -p
git commit -m "fix: <description of any issues found during device verification>"
```

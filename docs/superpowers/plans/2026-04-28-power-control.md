# Power Control Integration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add power button support — short press sleeps the device, 2-second hold shuts it down, with graceful task draining before either action and full resume-where-you-left-off behaviour after sleep.

**Architecture:** A new `internal/power` package reads `KEY_POWER` events from the first `/dev/input/event*` device that advertises that capability (platform-agnostic). Short/long press push SDL UserEvents into the main loop, which defers the action until active tasks finish (showing a full-screen overlay), then calls `$SYSTEM_PATH/bin/suspend` (sleep, blocks until wake) or writes `/tmp/poweroff` (shutdown). The app process stays alive through sleep, resuming the event loop on wake.

**Tech Stack:** `github.com/holoplot/go-evdev` (pure Go, no CGo), existing SDL2 `UserEvent` mechanism, `os/exec` for the system suspend call.

---

## File map

| File | Change | Responsibility |
|------|--------|----------------|
| `go.mod` / `go.sum` | Modify | Add `github.com/holoplot/go-evdev` |
| `internal/power/power.go` | **Create** | Scan for power device, read KEY_POWER events, fire sleep/shutdown callback |
| `internal/power/power_test.go` | **Create** | Test device-scan error path (headless-compatible) |
| `internal/ui/screen.go` | Modify | Add `BusyChecker` interface |
| `internal/ui/screen_download.go` | Modify | Add `IsBusy()` — true while download goroutine is running |
| `internal/ui/screen_cache_refresh.go` | Modify | Add `IsBusy()` — true while full-fetch goroutine is running |
| `internal/ui/screen_list.go` | Modify | Add `cacheBuilding atomic.Bool` field; add `IsBusy()` |
| `cmd/itchio-pak/main_sdl.go` | Modify | Power manager wiring, `pendingQuit` loop, overlay draw, sleep/shutdown execution |
| 15 `internal/ui/screen_*.go` | Modify | Remove unreachable `*sdl.QuitEvent` handlers |

> **Note on testing:** `scripts/test.sh` runs `go test -race -tags headless ./...`. Files with `//go:build !headless` (all UI screens) are excluded. Unit tests are provided where headless-compatible; UI IsBusy() methods are verified on device in the final verification section.

---

## Task 1 — Add `go-evdev` dependency

**Files:** `go.mod`, `go.sum`

- [ ] **Add the dependency**

  Run from the repo root (not inside the container):
  ```bash
  go get github.com/holoplot/go-evdev@latest
  ```
  Expected: `go.mod` gains a `require` line for `github.com/holoplot/go-evdev`, `go.sum` updated.

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Commit**

  ```bash
  git add go.mod go.sum
  git commit -m "chore: add github.com/holoplot/go-evdev dependency"
  ```

---

## Task 2 — Create `internal/power` package

**Files:** `internal/power/power.go`, `internal/power/power_test.go`

Scans `/dev/input/event*` for a device advertising `EV_KEY` + `KEY_POWER` capability, reads events in a background goroutine, measures press duration, and fires `notify(ActionSleep)` or `notify(ActionShutdown)`. Designed identically to `UpdateService`: caller injects a `notify` callback that pushes an SDL UserEvent.

- [ ] **Write the failing test**

  Create `internal/power/power_test.go`:
  ```go
  package power

  import "testing"

  func TestFindPowerDeviceWithPattern_NoMatch(t *testing.T) {
      _, err := findPowerDeviceWithPattern("/nonexistent/does-not-exist/event*")
      if err == nil {
          t.Error("expected error when no devices match pattern")
      }
  }
  ```

- [ ] **Run — expect compile error (package missing)**

  ```bash
  go test -race -tags headless ./internal/power/... 2>&1 | head -5
  ```
  Expected: `no Go files` or `cannot find package`.

- [ ] **Implement `internal/power/power.go`**

  ```go
  package power

  import (
  	"fmt"
  	"path/filepath"
  	"time"

  	evdev "github.com/holoplot/go-evdev"

  	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
  )

  const (
  	holdThreshold = 2 * time.Second
  	cooldown      = 1 * time.Second
  )

  // Action is the power button action to perform.
  type Action int

  const (
  	ActionSleep    Action = iota
  	ActionShutdown Action = iota
  )

  // Manager watches the power button input device and invokes notify when a
  // sleep or shutdown action is detected. notify is called from a background
  // goroutine and must be safe to call concurrently.
  type Manager struct {
  	notify func(Action)
  }

  // NewManager creates a Manager. notify must not be nil.
  func NewManager(notify func(Action)) *Manager {
  	return &Manager{notify: notify}
  }

  // Start launches the background goroutine. Returns immediately.
  // If no power button device is found, it logs a warning and returns.
  func (m *Manager) Start() {
  	go m.run()
  }

  func (m *Manager) run() {
  	dev, err := findPowerDeviceWithPattern("/dev/input/event*")
  	if err != nil {
  		logger.Warn("power: no power button device found: %v", err)
  		return
  	}
  	defer dev.Close()
  	logger.Info("power: monitoring for KEY_POWER events")

  	var pressTime time.Time
  	var cooldownUntil time.Time

  	for {
  		event, err := dev.ReadOne()
  		if err != nil {
  			logger.Error("power: read error: %v", err)
  			return
  		}
  		if time.Now().Before(cooldownUntil) {
  			continue
  		}
  		if event.Type != evdev.EV_KEY || event.Code != evdev.KEY_POWER {
  			continue
  		}
  		switch event.Value {
  		case 1: // key down
  			pressTime = time.Now()
  		case 2: // key held
  			if !pressTime.IsZero() && time.Since(pressTime) >= holdThreshold {
  				logger.Info("power: long press detected — shutdown")
  				m.notify(ActionShutdown)
  				pressTime = time.Time{}
  				cooldownUntil = time.Now().Add(cooldown)
  			}
  		case 0: // key up
  			if !pressTime.IsZero() {
  				if time.Since(pressTime) < holdThreshold {
  					logger.Info("power: short press detected — sleep")
  					m.notify(ActionSleep)
  				}
  				pressTime = time.Time{}
  				cooldownUntil = time.Now().Add(cooldown)
  			}
  		}
  	}
  }

  // findPowerDeviceWithPattern scans devices matching pattern for one with
  // KEY_POWER capability. Package-private; tests call it directly.
  func findPowerDeviceWithPattern(pattern string) (*evdev.InputDevice, error) {
  	paths, err := filepath.Glob(pattern)
  	if err != nil || len(paths) == 0 {
  		return nil, fmt.Errorf("no devices found matching %q", pattern)
  	}
  	for _, path := range paths {
  		dev, err := evdev.Open(path)
  		if err != nil {
  			continue
  		}
  		if deviceHasPowerKey(dev) {
  			return dev, nil
  		}
  		dev.Close()
  	}
  	return nil, fmt.Errorf("no device with KEY_POWER found")
  }

  func deviceHasPowerKey(dev *evdev.InputDevice) bool {
  	for _, t := range dev.CapableTypes() {
  		if t != evdev.EV_KEY {
  			continue
  		}
  		for _, code := range dev.CapableEvents(evdev.EV_KEY) {
  			if code == evdev.KEY_POWER {
  				return true
  			}
  		}
  	}
  	return false
  }
  ```

- [ ] **Run tests — expect pass**

  ```bash
  go test -race -tags headless ./internal/power/...
  ```
  Expected:
  ```
  ok      github.com/carroarmato0/nextui-itchio-pak/internal/power
  ```

- [ ] **Verify full build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Commit**

  ```bash
  git add internal/power/
  git commit -m "feat(power): add power button manager with sleep/shutdown detection"
  ```

---

## Task 3 — Add `BusyChecker` interface

**Files:** `internal/ui/screen.go`

- [ ] **Add the interface**

  Open `internal/ui/screen.go`. After the `Screen` interface closing brace, add:
  ```go
  // BusyChecker is implemented by screens that must block safe power-off while
  // an operation is in progress. The main event loop uses a type assertion —
  // this is intentionally not part of the Screen interface.
  type BusyChecker interface {
  	IsBusy() bool
  }
  ```

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Commit**

  ```bash
  git add internal/ui/screen.go
  git commit -m "feat(ui): add BusyChecker interface for power-safe shutdown"
  ```

---

## Task 4 — `DownloadScreen.IsBusy()`

**Files:** `internal/ui/screen_download.go`

`IsBusy()` returns true while the download goroutine is running (state `dlDownloading`). Once it succeeds or errors, state transitions to `dlDone`/`dlError` and `IsBusy()` returns false.

- [ ] **Add the method**

  In `internal/ui/screen_download.go`, add after the `HandleEvent` method:
  ```go
  // IsBusy implements BusyChecker. Returns true while a download is in flight.
  func (s *DownloadScreen) IsBusy() bool {
  	return s.state == dlDownloading
  }
  ```

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Commit**

  ```bash
  git add internal/ui/screen_download.go
  git commit -m "feat(ui): DownloadScreen implements BusyChecker"
  ```

---

## Task 5 — `CacheRefreshScreen.IsBusy()`

**Files:** `internal/ui/screen_cache_refresh.go`

- [ ] **Add the method**

  In `internal/ui/screen_cache_refresh.go`, add after the `HandleEvent` method:
  ```go
  // IsBusy implements BusyChecker. Returns true while the cache fetch is running.
  func (s *CacheRefreshScreen) IsBusy() bool {
  	return refreshCacheState(atomic.LoadInt32((*int32)(&s.state))) == refreshCacheLoading
  }
  ```

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Commit**

  ```bash
  git add internal/ui/screen_cache_refresh.go
  git commit -m "feat(ui): CacheRefreshScreen implements BusyChecker"
  ```

---

## Task 6 — `ListScreen.IsBusy()` via `cacheBuilding`

**Files:** `internal/ui/screen_list.go`

`ListScreen` spawns a `buildCache()` goroutine that fetches the full game list from the network and writes it to the SD card. `IsBusy()` is true while this is active.

- [ ] **Add `cacheBuilding` field to the `ListScreen` struct**

  In `internal/ui/screen_list.go`, locate the `ListScreen` struct (around line 37). Add the field after `updateSvc`:
  ```go
  	updateSvc     UpdateServicer

  	// cacheBuilding is set while buildCache / refreshCacheIfStale runs.
  	cacheBuilding atomic.Bool
  ```
  `atomic.Bool` is in the standard library (Go 1.19+). No new import needed.

- [ ] **Set the flag in `buildCache()`**

  Locate `func (s *ListScreen) buildCache()` (around line 873). Add the flag at the top:
  ```go
  func (s *ListScreen) buildCache() {
  	s.cacheBuilding.Store(true)
  	defer s.cacheBuilding.Store(false)
  	logger.Info("cache: starting background full fetch")
  	// ... rest of existing body unchanged
  ```

- [ ] **Add `IsBusy()` method**

  Add after `rebuildView`:
  ```go
  // IsBusy implements BusyChecker. Returns true while the background game-list
  // fetch/write goroutine is running.
  func (s *ListScreen) IsBusy() bool {
  	return s.cacheBuilding.Load()
  }
  ```

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Run existing tests to check nothing broke**

  ```bash
  scripts/test.sh
  ```
  Expected: all existing tests pass.

- [ ] **Commit**

  ```bash
  git add internal/ui/screen_list.go
  git commit -m "feat(ui): ListScreen implements BusyChecker via cacheBuilding flag"
  ```

---

## Task 7 — Wire power manager into `main_sdl.go`

**Files:** `cmd/itchio-pak/main_sdl.go`

This task adds: UserEvent constants, power manager startup, modified event loop, per-frame pending-quit check with inline sleep/shutdown execution, and the overlay draw function.

- [ ] **Add imports**

  In `cmd/itchio-pak/main_sdl.go`, add to the import block:
  ```go
  "os/exec"
  "path/filepath"

  "github.com/carroarmato0/nextui-itchio-pak/internal/power"
  ```
  (`"os"` is already present.)

- [ ] **Add UserEvent constants**

  Just before `func runSDL()`, add:
  ```go
  const (
  	userEventInventoryUpdate = int32(0) // UpdateService finished a check
  	userEventPowerSleep      = int32(1) // power: short press
  	userEventPowerShutdown   = int32(2) // power: long press
  )
  ```

- [ ] **Start the power manager**

  Inside `runSDL()`, after `updateSvc.Start(nil)`, add:
  ```go
  powerMgr := power.NewManager(func(action power.Action) {
  	code := userEventPowerSleep
  	if action == power.ActionShutdown {
  		code = userEventPowerShutdown
  	}
  	sdl.PushEvent(&sdl.UserEvent{Type: sdl.USEREVENT, Code: code})
  })
  powerMgr.Start()
  ```

- [ ] **Add pendingQuit state variables**

  After `var current ui.Screen = ui.NewListScreen(...)`, add:
  ```go
  var pendingQuit bool
  var pendingAction power.Action
  ```

- [ ] **Replace the event polling block**

  The current inner event loop is:
  ```go
  for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
      if pressedScancodes != nil {
          // my355 dedup ...
      }
      current = current.HandleEvent(e)
      if current == nil {
          break
      }
  }
  ```

  Replace it with:
  ```go
  for e := sdl.PollEvent(); e != nil; e = sdl.PollEvent() {
      if pendingQuit {
          continue // drain input while waiting for tasks
      }
      if pressedScancodes != nil {
          if kev, ok := e.(*sdl.KeyboardEvent); ok {
              sc := kev.Keysym.Scancode
              if kev.Type == sdl.KEYDOWN {
                  if pressedScancodes[sc] {
                      continue
                  }
                  pressedScancodes[sc] = true
              } else if kev.Type == sdl.KEYUP {
                  delete(pressedScancodes, sc)
              }
          }
      }
      // Intercept SDL_QUIT (SIGTERM from NextUI) before screens see it.
      if _, ok := e.(*sdl.QuitEvent); ok {
          current = nil
          break
      }
      // Intercept power UserEvents before screens see them.
      if uev, ok := e.(*sdl.UserEvent); ok {
          switch uev.Code {
          case userEventPowerSleep:
              logger.Info("power: sleep requested, waiting for tasks")
              pendingQuit = true
              pendingAction = power.ActionSleep
              updateSvc.Stop()
              continue
          case userEventPowerShutdown:
              logger.Info("power: shutdown requested, waiting for tasks")
              pendingQuit = true
              pendingAction = power.ActionShutdown
              updateSvc.Stop()
              continue
          }
      }
      current = current.HandleEvent(e)
      if current == nil {
          break
      }
  }
  ```

- [ ] **Replace the draw block**

  The current draw block at the bottom of the outer loop is:
  ```go
  if current != nil {
      current.Draw(r)
  }
  sdl.Delay(16) // ~60 fps
  ```

  Replace it with:
  ```go
  if current != nil {
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
                  for {
                      sdl.Delay(1000) // wait for system to kill us
                  }
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
                  pendingQuit = false
                  current.Draw(r) // first frame after wake
              }
          } else {
              drawPowerPendingOverlay(r, pendingAction)
          }
      } else {
          current.Draw(r)
      }
  }
  sdl.Delay(16) // ~60 fps
  ```

- [ ] **Add `drawPowerPendingOverlay`**

  Add this function after `runSDL()` in `main_sdl.go`:
  ```go
  func drawPowerPendingOverlay(r *renderer.Renderer, action power.Action) {
      r.Clear(20, 20, 20)
      subtitle := "Finishing up before sleep…"
      if action == power.ActionShutdown {
          subtitle = "Finishing up before shutdown…"
      }
      _, mainH := r.TextSize("Ag")
      mid := r.H / 2
      r.DrawTextCentered("Please wait", 0, mid-mainH-6, r.W, 220, 220, 220)
      r.DrawSmallTextCentered(subtitle, 0, mid+6, r.W, 120, 120, 120)
      r.Present()
  }
  ```

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Run tests**

  ```bash
  scripts/test.sh
  ```
  Expected: all existing tests pass.

- [ ] **Commit**

  ```bash
  git add cmd/itchio-pak/main_sdl.go
  git commit -m "feat: wire power manager — sleep/shutdown with task draining and overlay"
  ```

---

## Task 8 — Remove dead `QuitEvent` handlers

**Files:** 15 `internal/ui/screen_*.go` files

Now that `main_sdl.go` intercepts `*sdl.QuitEvent` before forwarding to screens, all per-screen handlers are unreachable dead code. Remove each two-line block:
```go
case *sdl.QuitEvent:
    return nil
```

Exact locations (line numbers are approximate — find by search):

| File | Occurrences |
|------|-------------|
| `screen_about.go` | 1 |
| `screen_apikey_check.go` | 1 |
| `screen_cache_refresh.go` | 1 |
| `screen_content_moderation.go` | 1 |
| `screen_detail.go` | **2** |
| `screen_download.go` | 1 |
| `screen_fetch_uploads.go` | 1 |
| `screen_format_picker.go` | 1 |
| `screen_list.go` | 1 |
| `screen_location_picker.go` | 1 |
| `screen_manage_downloads.go` | **2** |
| `screen_purchase_picker.go` | 1 |
| `screen_rom_picker.go` | 1 |
| `screen_settings.go` | 1 |
| `screen_tag_filter.go` | 1 |

- [ ] **Remove all 17 occurrences**

  For each file, delete both lines of every `case *sdl.QuitEvent: return nil` block. The surrounding `switch` structure remains valid.

  Verify none remain:
  ```bash
  grep -rn "QuitEvent" internal/ui/
  ```
  Expected: no output.

- [ ] **Verify the build**

  ```bash
  go build -tags headless ./...
  ```
  Expected: no errors.

- [ ] **Run all tests**

  ```bash
  scripts/test.sh
  ```
  Expected: all existing tests pass.

- [ ] **Commit**

  ```bash
  git add internal/ui/
  git commit -m "refactor(ui): remove unreachable QuitEvent handlers (main loop now intercepts)"
  ```

---

## Verification (on device via ADB)

Use `scripts/deploy.sh` to push the binary, then connect via `adb shell` and tail the log:
```bash
adb shell tail -f /mnt/SDCARD/.userdata/tg5040/logs/itchio-pak.log
```

| Scenario | Steps | Expected |
|----------|-------|----------|
| Sleep, no tasks | Open app, wait for load, short-press power | App sleeps immediately; wakes at same screen |
| Sleep during download | Start ROM download, short-press power | Overlay shown; download completes; device sleeps; app resumes with download done |
| Sleep during cache build | First launch (cache not built), short-press power | Overlay shown until build finishes; device sleeps; resumes at list screen |
| Shutdown | Hold power 2+ seconds | Overlay shown; tasks drain; device shuts down |
| SIGTERM (go back to launcher) | Press B to exit | App exits cleanly, no overlay |
| No `$SYSTEM_PATH` (dev machine) | Run binary without SYSTEM_PATH set | Warning logged, app exits cleanly instead of sleeping |
